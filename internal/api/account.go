package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"golang.org/x/crypto/bcrypt"

	"github.com/wonjune95/sealane/internal/auth"
)

// EnsureInitialAdmin bootstraps an "admin" account when no account has the admin role (a fresh
// install, or a ConfigMap emptied by hand). Like Argo CD's argocd-initial-admin-secret, the password
// is random and written to the initial-admin Secret first, so nobody can sign in with a known
// default before the operator does:
//
//	kubectl -n <ns> get secret <release>-initial-admin -o jsonpath='{.data.password}' | base64 -d
//
// It reports whether it settled (an admin exists or was made, or there is nothing it may do);
// a transient failure returns false, and KeepEnsuringInitialAdmin retries.
func (s *Server) EnsureInitialAdmin(ctx context.Context) (settled bool) {
	defer func() { s.adminPending.Store(!settled) }()
	if s.dev || s.accountsCM.Name == "" {
		return true
	}
	client, err := s.factory.Base()
	if err != nil {
		slog.Warn("initial admin: k8s client unavailable", "err", err)
		return false
	}
	cms := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace)
	cm, err := cms.Get(ctx, s.accountsCM.Name, metav1.GetOptions{})
	cmMissing := apierrors.IsNotFound(err)
	switch {
	case err == nil && auth.HasAdmin(cm.Data):
		return true // an admin exists — nothing to do
	case err == nil && cm.Data["admin"] != "":
		slog.Warn("initial admin: the accounts ConfigMap has an \"admin\" key without the admin role; not touching it")
		return true
	case err != nil && !cmMissing:
		slog.Warn("initial admin: read accounts configmap failed", "err", err)
		return false
	}
	if s.initialAdminSecret == "" {
		slog.Warn("initial admin: INITIAL_ADMIN_SECRET is not set, so there is nowhere to put a password; add an admin to the accounts ConfigMap by hand")
		return true
	}
	// Secret first: an account whose password is stored nowhere would lock everyone out. Replicas
	// starting together all end up with the one password that won the Secret write.
	password, err := loadOrCreateSecretKey(ctx, client.Kube, s.accountsCM.Namespace, s.initialAdminSecret, "password", randomPassword)
	if err != nil {
		slog.Warn("initial admin: write the initial-admin Secret failed", "secret", s.initialAdminSecret, "err", err)
		return false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Warn("initial admin: hash failed", "err", err)
		return false
	}
	entry := "admin:" + string(hash)
	if cmMissing {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: s.accountsCM.Name, Namespace: s.accountsCM.Namespace},
			Data:       map[string]string{"admin": entry},
		}, metav1.CreateOptions{})
	}
	if !cmMissing || apierrors.IsAlreadyExists(err) { // another replica created it first: same password
		err = s.updateAccount(ctx, "admin", func(string, bool) (string, error) { return entry, nil })
	}
	s.accounts.Invalidate()
	if err != nil {
		slog.Warn("initial admin: write accounts configmap failed", "err", err)
		return false
	}
	slog.Info("initial admin account created; its password is in the initial-admin Secret",
		"secret", s.accountsCM.Namespace+"/"+s.initialAdminSecret, "key", "password")
	return true
}

// InitialAdminPending reports that the first admin account could not be made yet; the pod stays
// not ready meanwhile, so a fresh install that nobody could sign in to does not look healthy.
func (s *Server) InitialAdminPending() bool { return s.adminPending.Load() }

// KeepEnsuringInitialAdmin runs EnsureInitialAdmin until it settles: a transient API error at
// startup must not leave a fresh install without any account until someone restarts the pod.
func (s *Server) KeepEnsuringInitialAdmin(ctx context.Context, every time.Duration) {
	for !s.EnsureInitialAdmin(ctx) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// SetInitialAdminSecret names the Secret that receives a generated initial admin password.
func (s *Server) SetInitialAdminSecret(name string) { s.initialAdminSecret = name }

func randomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no entropy — nothing sane to do
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// AccountsCMRef locates the accounts ConfigMap so password changes can be
// written back (the mounted dir is read-only from the pod's perspective).
type AccountsCMRef struct{ Namespace, Name string }

// handlePasswordChange lets any logged-in user (viewers included) change their own
// password: verify the current one against the stored entry and write the new hash in one
// read-modify-write (updateAccount).
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if s.dev {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "dev-mode-no-accounts", "no accounts in dev mode")
		return
	}
	id, ok := s.session(w, r, true)
	if !ok {
		return
	}
	if id.Provider == auth.ProviderOIDC {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "sso-no-password", "password change is not available for single sign-on accounts")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "invalid-body", "invalid request body")
		return
	}
	if len(in.NewPassword) < 8 {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "new-password-too-short", "new password must be at least 8 characters")
		return
	}
	if s.accountsCM.Name == "" {
		writeCoded(w, http.StatusInternalServerError, "Internal", "accounts-not-configured", "accounts ConfigMap location is not configured (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	// The current password is checked against the entry as stored at the moment of the write, and
	// the account keeps the role it has then.
	var role string
	err = s.updateAccount(r.Context(), id.Name, func(old string, exists bool) (string, error) {
		if !exists {
			return "", errAccountGone
		}
		var stored string
		role, stored, _ = strings.Cut(strings.TrimSpace(old), ":")
		if !auth.ValidRole(role) || bcrypt.CompareHashAndPassword([]byte(stored), []byte(in.CurrentPassword)) != nil {
			return "", errPasswordIncorrect
		}
		return role + ":" + string(hash), nil
	})
	switch {
	case errors.Is(err, errPasswordIncorrect):
		writeCoded(w, http.StatusUnauthorized, "Unauthorized", "password-incorrect", "current password is incorrect")
		return
	case errors.Is(err, errAccountGone):
		clearCookie(w, r, sessionCookie, "/")
		writeCoded(w, http.StatusUnauthorized, "Unauthorized", "session-revoked", "your session ended because the account changed; sign in again")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update accounts ConfigMap: "+err.Error())
		return
	}
	// Older sessions of this account carry the old password version and are refused from now on;
	// this one continues with a fresh cookie.
	id.Role, id.PV = role, auth.PasswordVersion(string(hash))
	setSessionCookie(w, r, s.sessions.Sign(id, sessionTTL))
	slog.Info("password changed", "user", id.Name)
	w.WriteHeader(http.StatusNoContent)
}

// true if a local account still uses the old fixed initial password "admin" (installs from before
// 1.19). withAuth then allows nothing but changing it, and the UI shows the change screen.
func (s *Server) mustChangePassword(user string) bool {
	if s.dev || user == "" {
		return false
	}
	return s.accounts.PasswordIs(user, "admin")
}

// errPasswordRequired: a role-only change for an account that does not exist.
var errPasswordRequired = errors.New("a new account requires a password")

// errAccountGone: the account was deleted while a change to it was being made.
var errAccountGone = errors.New("account no longer exists")

// errPasswordIncorrect: the current password does not match the stored hash.
var errPasswordIncorrect = errors.New("current password is incorrect")

// updateAccount changes one account in the ConfigMap with a read-modify-write on its
// resourceVersion: mutate gets the entry as stored now ("" and false when there is none) and
// returns the new entry, or "" to delete it. A concurrent change makes the write conflict, and
// mutate runs again on the new data, so a password change can never revive an account an admin
// deleted, or undo a role change, a moment earlier.
func (s *Server) updateAccount(ctx context.Context, user string, mutate func(old string, exists bool) (string, error)) error {
	client, err := s.factory.Base()
	if err != nil {
		return err
	}
	cms := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm, err := cms.Get(ctx, s.accountsCM.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		old, exists := cm.Data[user]
		entry, err := mutate(old, exists)
		if err != nil {
			return err
		}
		if entry == "" && !exists {
			return nil // nothing to delete
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if entry == "" {
			delete(cm.Data, user)
		} else {
			cm.Data[user] = entry
		}
		_, err = cms.Update(ctx, cm, metav1.UpdateOptions{}) // carries the resourceVersion read above
		return err
	})
	s.accounts.Invalidate() // this replica sees the change (and revokes sessions) at once
	if err != nil && !errors.Is(err, errAccountGone) && !errors.Is(err, errPasswordIncorrect) && !errors.Is(err, errPasswordRequired) {
		return fmt.Errorf("%s/%s: %w", s.accountsCM.Namespace, s.accountsCM.Name, err)
	}
	return err
}

// --- Account management (admin only): exposes the ConfigMap via the API for editing in the UI ---

// Usernames that satisfy both the ConfigMap key rule (-._a-zA-Z0-9) and the Store's file-name rule.
var validUsername = regexp.MustCompile(`^[a-zA-Z0-9][-._a-zA-Z0-9]{0,63}$`)

// adminOnly checks dev mode, session and admin role (shared by account and cluster management).
// On failure the response is already written, so callers just return when ok=false.
func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	if s.dev {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "dev-mode-no-accounts", "no accounts in dev mode")
		return auth.Identity{}, false
	}
	id, ok := s.session(w, r, false)
	if !ok {
		return auth.Identity{}, false
	}
	if id.Role != "admin" {
		writeCoded(w, http.StatusForbidden, "Forbidden", "admin-required", "admin role required")
		return auth.Identity{}, false
	}
	return id, true
}

// adminGuard = adminOnly + a check that the accounts ConfigMap is configured.
func (s *Server) adminGuard(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, ok := s.adminOnly(w, r)
	if !ok {
		return auth.Identity{}, false
	}
	if s.accountsCM.Name == "" {
		writeCoded(w, http.StatusInternalServerError, "Internal", "accounts-not-configured", "accounts ConfigMap location is not configured (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
		return auth.Identity{}, false
	}
	return id, true
}

func (s *Server) handleAccountsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminGuard(w, r); !ok {
		return
	}
	client, err := s.factory.Base()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	cm, err := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace).Get(r.Context(), s.accountsCM.Name, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to read accounts ConfigMap: "+err.Error())
		return
	}
	type account struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	list := make([]account, 0, len(cm.Data))
	for name, entry := range cm.Data {
		role, _, _ := strings.Cut(entry, ":")
		list = append(list, account{Name: name, Role: role})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	writeJSON(w, http.StatusOK, list)
}

// handleAccountUpsert creates or updates an account. An empty password
// keeps the existing hash and changes only the role. Changing your own role is
// blocked to prevent lockout.
func (s *Server) handleAccountUpsert(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminGuard(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !validUsername.MatchString(name) {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "username-invalid", "username must start with a letter or digit and contain only letters, digits, '-', '.', '_' (max 64 characters)")
		return
	}
	var in struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "invalid-body", "invalid request body")
		return
	}
	if !auth.ValidRole(in.Role) {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "role-invalid", "role must be one of admin, editor, viewer")
		return
	}
	if name == id.Name && in.Role != id.Role {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "own-role", "you cannot change your own role")
		return
	}
	var newHash string
	if in.Password != "" {
		if len(in.Password) < 8 {
			writeCoded(w, http.StatusBadRequest, "BadRequest", "password-too-short", "password must be at least 8 characters")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", err.Error())
			return
		}
		newHash = string(hash)
	}
	err := s.updateAccount(r.Context(), name, func(old string, exists bool) (string, error) {
		if newHash != "" {
			return in.Role + ":" + newHash, nil
		}
		// Role-only change: keep the hash stored now.
		_, hash, found := strings.Cut(strings.TrimSpace(old), ":")
		if !exists || !found {
			return "", errPasswordRequired
		}
		return in.Role + ":" + hash, nil
	})
	if errors.Is(err, errPasswordRequired) {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "password-required", "a new account requires a password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update accounts ConfigMap: "+err.Error())
		return
	}
	slog.Info("account upserted", "by", id.Name, "user", name, "role", in.Role)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminGuard(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if name == "admin" {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "admin-undeletable", "the admin account cannot be deleted")
		return
	}
	if name == id.Name {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "own-account", "you cannot delete your own account")
		return
	}
	if err := s.updateAccount(r.Context(), name, func(string, bool) (string, error) { return "", nil }); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update accounts ConfigMap: "+err.Error())
		return
	}
	slog.Info("account deleted", "by", id.Name, "user", name)
	w.WriteHeader(http.StatusNoContent)
}
