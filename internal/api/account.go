package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"golang.org/x/crypto/bcrypt"

	"github.com/wonjune95/sealane/internal/auth"
)

// EnsureInitialAdmin creates the initial admin/admin account when the accounts
// ConfigMap has no admin (or the ConfigMap itself is missing). Accounts are added
// and changed by editing this ConfigMap. Failure should not block startup, so it
// is only logged as a warning.
func (s *Server) EnsureInitialAdmin(ctx context.Context) {
	if s.dev || s.accountsCM.Name == "" {
		return
	}
	client, err := s.factory.Base()
	if err != nil {
		slog.Warn("initial admin: k8s client unavailable", "err", err)
		return
	}
	cms := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace)
	cm, err := cms.Get(ctx, s.accountsCM.Name, metav1.GetOptions{})
	cmMissing := apierrors.IsNotFound(err)
	switch {
	case err == nil && cm.Data["admin"] != "":
		return // admin exists — nothing to do
	case err != nil && !cmMissing:
		slog.Warn("initial admin: read accounts configmap failed", "err", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		slog.Warn("initial admin: hash failed", "err", err)
		return
	}
	entry := "admin:" + string(hash)
	if cmMissing {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: s.accountsCM.Name, Namespace: s.accountsCM.Namespace},
			Data:       map[string]string{"admin": entry},
		}, metav1.CreateOptions{})
	} else {
		err = s.patchAccount(ctx, "admin", entry)
	}
	if err != nil {
		slog.Warn("initial admin: write accounts configmap failed", "err", err)
		return
	}
	slog.Info("initial admin account created (admin/admin) — change the password after first login")
}

// AccountsCMRef locates the accounts ConfigMap so password changes can be
// written back (the mounted dir is read-only from the pod's perspective).
type AccountsCMRef struct{ Namespace, Name string }

// handlePasswordChange lets any logged-in user (viewers included) change their own
// password: verify the current one, bcrypt the new one, patch the ConfigMap key.
// The mounted dir syncs within ~1min, so the old password works until then.
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if s.dev {
		writeError(w, http.StatusBadRequest, "BadRequest", "no accounts in dev mode")
		return
	}
	id, err := s.sessionIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
		return
	}
	if id.Provider == auth.ProviderOIDC {
		writeError(w, http.StatusBadRequest, "BadRequest", "password change is not available for single sign-on accounts")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "invalid request body")
		return
	}
	if len(in.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "BadRequest", "new password must be at least 8 characters")
		return
	}
	role, err := s.accounts.Authenticate(id.Name, in.CurrentPassword)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "current password is incorrect")
		return
	}
	if s.accountsCM.Name == "" {
		writeError(w, http.StatusInternalServerError, "Internal", "accounts ConfigMap location is not configured (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	if err := s.patchAccount(r.Context(), id.Name, role+":"+string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update accounts ConfigMap: "+err.Error())
		return
	}
	s.markPasswordChanged(id.Name)
	slog.Info("password changed", "user", id.Name)
	w.WriteHeader(http.StatusNoContent)
}

// true if the account still uses the initial password ("admin") — the UI pins the
// user to the change-password screen. A user who just changed it is released
// immediately in memory, without waiting for the mount sync (~1 min).
func (s *Server) mustChangePassword(user string) bool {
	if s.dev || user == "" {
		return false
	}
	s.pwChangedMu.Lock()
	changed := s.pwChanged[user]
	s.pwChangedMu.Unlock()
	return !changed && s.accounts.PasswordIs(user, "admin")
}

func (s *Server) markPasswordChanged(user string) {
	s.pwChangedMu.Lock()
	if s.pwChanged == nil {
		s.pwChanged = map[string]bool{}
	}
	s.pwChanged[user] = true
	s.pwChangedMu.Unlock()
}

// patchAccount merge-patches one ConfigMap key; entry nil deletes the key.
func (s *Server) patchAccount(ctx context.Context, user string, entry any) error {
	client, err := s.factory.Base()
	if err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{"data": map[string]any{user: entry}})
	if err != nil {
		return err
	}
	_, err = client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace).
		Patch(ctx, s.accountsCM.Name, types.MergePatchType, b, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("%s/%s: %w", s.accountsCM.Namespace, s.accountsCM.Name, err)
	}
	return nil
}

// --- Account management (admin only): exposes the ConfigMap via the API for editing in the UI ---

// Usernames that satisfy both the ConfigMap key rule (-._a-zA-Z0-9) and the Store's file-name rule.
var validUsername = regexp.MustCompile(`^[a-zA-Z0-9][-._a-zA-Z0-9]{0,63}$`)

// adminOnly checks dev mode, session and admin role (shared by account and cluster management).
// On failure the response is already written, so callers just return when ok=false.
func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	if s.dev {
		writeError(w, http.StatusBadRequest, "BadRequest", "no accounts in dev mode")
		return auth.Identity{}, false
	}
	id, err := s.sessionIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
		return auth.Identity{}, false
	}
	if id.Role != "admin" {
		writeError(w, http.StatusForbidden, "Forbidden", "admin role required")
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
		writeError(w, http.StatusInternalServerError, "Internal", "accounts ConfigMap location is not configured (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
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
		writeError(w, http.StatusBadRequest, "BadRequest", "username must start with a letter or digit and contain only letters, digits, '-', '.', '_' (max 64 characters)")
		return
	}
	var in struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "invalid request body")
		return
	}
	if !auth.ValidRole(in.Role) {
		writeError(w, http.StatusBadRequest, "BadRequest", "role must be one of admin, editor, viewer")
		return
	}
	if name == id.Name && in.Role != id.Role {
		writeError(w, http.StatusBadRequest, "BadRequest", "you cannot change your own role")
		return
	}
	var entry string
	if in.Password != "" {
		if len(in.Password) < 8 {
			writeError(w, http.StatusBadRequest, "BadRequest", "password must be at least 8 characters")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", err.Error())
			return
		}
		entry = in.Role + ":" + string(hash)
	} else {
		// Role-only change: read and keep the existing hash.
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
		_, hash, found := strings.Cut(cm.Data[name], ":")
		if !found {
			writeError(w, http.StatusBadRequest, "BadRequest", "a new account requires a password")
			return
		}
		entry = in.Role + ":" + hash
	}
	if err := s.patchAccount(r.Context(), name, entry); err != nil {
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
		writeError(w, http.StatusBadRequest, "BadRequest", "the admin account cannot be deleted")
		return
	}
	if name == id.Name {
		writeError(w, http.StatusBadRequest, "BadRequest", "you cannot delete your own account")
		return
	}
	if err := s.patchAccount(r.Context(), name, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update accounts ConfigMap: "+err.Error())
		return
	}
	slog.Info("account deleted", "by", id.Name, "user", name)
	w.WriteHeader(http.StatusNoContent)
}
