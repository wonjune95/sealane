// Package auth implements app-local accounts: users live in a mounted
// ConfigMap (one file per user, content "role:bcryptHash"), login issues an
// HMAC-signed session cookie, and the app's role — not per-user Kubernetes RBAC —
// authorizes writes. All Kubernetes calls run as the pod's ServiceAccount.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Provider values carried in the session: who authenticated the user.
const (
	ProviderLocal = "local"
	ProviderOIDC  = "oidc"
)

// Identity is the logged-in user attached to each request.
type Identity struct {
	Name     string
	Role     string // admin | editor | viewer
	Provider string // ProviderLocal | ProviderOIDC
	// Groups are the IdP groups at sign-in, for the login log only; never stored in the session.
	Groups []string `json:"-"`
	// PV is a local account's password version at sign-in (PasswordVersion of its hash). A session
	// whose PV no longer matches the account was issued before a password change and is refused.
	PV string `json:"-"`
}

// PasswordVersion is a short, non-reversible fingerprint of a stored bcrypt hash.
func PasswordVersion(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(sum[:9])
}

// User returns a stable display identity for audit logs.
func (i Identity) User() string { return i.Name }

// CanWrite reports whether the role may mutate resources.
func (i Identity) CanWrite() bool { return i.Role == "admin" || i.Role == "editor" }

var validRoles = map[string]bool{"admin": true, "editor": true, "viewer": true}

// ValidRole reports whether r is one of the app roles.
func ValidRole(r string) bool { return validRoles[r] }

// Store reads accounts ("role:bcryptHash" per user). With a source (the accounts ConfigMap read
// through the API, see SetSource) changes are seen within sourceTTL on every replica, which is what
// revoking a deleted or demoted account's sessions relies on; otherwise, or when the source fails,
// it reads the mounted ConfigMap directory, which kubelet refreshes about once a minute.
type Store struct {
	dir string

	srcMu   sync.Mutex
	source  func() (map[string]string, error)
	srcData map[string]string
	srcAt   time.Time

	pwMu    sync.Mutex
	pwCache map[string]bool // "user:hash:plain" → match result (for the initial-password check in capabilities)
}

const sourceTTL = 3 * time.Second

func NewStore(dir string) *Store { return &Store{dir: dir, pwCache: map[string]bool{}} }

// SetSource reads accounts from fn (cached for sourceTTL) instead of the mounted directory.
func (s *Store) SetSource(fn func() (map[string]string, error)) {
	s.srcMu.Lock()
	s.source, s.srcData = fn, nil
	s.srcMu.Unlock()
}

// Invalidate drops the cached accounts, so this replica sees a change it just made at once.
func (s *Store) Invalidate() {
	s.srcMu.Lock()
	s.srcData = nil
	s.srcMu.Unlock()
}

func validUsername(u string) bool {
	return u != "" && !strings.ContainsAny(u, "/\\") && !strings.HasPrefix(u, ".")
}

// raw returns the stored "role:hash" entry of an account.
func (s *Store) raw(username string) (string, bool) {
	if !validUsername(username) { // usernames are file names; reject path tricks before touching the fs
		return "", false
	}
	s.srcMu.Lock()
	if s.source != nil && (s.srcData == nil || time.Since(s.srcAt) > sourceTTL) {
		if data, err := s.source(); err == nil {
			s.srcData, s.srcAt = data, time.Now()
		}
	}
	data := s.srcData
	s.srcMu.Unlock()
	if data != nil {
		v, ok := data[username]
		return strings.TrimSpace(v), ok
	}
	b, err := os.ReadFile(filepath.Join(s.dir, username))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// Lookup returns an account's role and password version (ok=false: no such account or a broken entry).
func (s *Store) Lookup(username string) (role, pv string, ok bool) {
	entry, found := s.raw(username)
	role, hash, cut := strings.Cut(entry, ":")
	if !found || !cut || !validRoles[role] {
		return "", "", false
	}
	return role, PasswordVersion(hash), true
}

// HasAdmin reports whether any account entry has the admin role.
func HasAdmin(entries map[string]string) bool {
	for _, v := range entries {
		if role, _, _ := strings.Cut(strings.TrimSpace(v), ":"); role == "admin" {
			return true
		}
	}
	return false
}

// PasswordIs reports whether the account's current password equals plain.
// It runs on every capabilities call, so results are cached per hash (a new
// password means a new hash and thus a new key — stale entries are harmless and
// grow only with the number of users).
func (s *Store) PasswordIs(username, plain string) bool {
	entry, found := s.raw(username)
	_, hash, ok := strings.Cut(entry, ":")
	if !found || !ok {
		return false
	}
	key := username + ":" + hash + ":" + plain
	s.pwMu.Lock()
	defer s.pwMu.Unlock()
	if v, cached := s.pwCache[key]; cached {
		return v
	}
	v := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
	s.pwCache[key] = v
	return v
}

var ErrBadCredentials = errors.New("invalid username or password")

// Authenticate verifies username/password and returns the account's role.
// Authenticate verifies username/password and returns the account's role and password version.
func (s *Store) Authenticate(username, password string) (role, pv string, err error) {
	entry, found := s.raw(username)
	if !found {
		// burn comparable time so missing vs wrong-password is indistinguishable
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0xF3F0mQ5eGvHq0FhqYb6R1r1uW"), []byte(password))
		return "", "", ErrBadCredentials
	}
	role, hash, ok := strings.Cut(entry, ":")
	if !ok || !validRoles[role] {
		return "", "", fmt.Errorf("account %q is misconfigured (expected role:bcryptHash)", username)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", ErrBadCredentials
	}
	return role, PasswordVersion(hash), nil
}

// Sessions signs and verifies session cookie values: "user|role|expiry|provider|pv|sig".
type Sessions struct{ secret []byte }

// NewSessions uses SESSION_SECRET when provided; otherwise a random per-boot key.
// ponytail: random key = everyone re-logs-in on pod restart (matches the in-memory
// audit history trade-off); set SESSION_SECRET if that ever hurts.
func NewSessions(secret string) *Sessions {
	if secret != "" {
		return &Sessions{secret: []byte(secret)}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no entropy — nothing sane to do
	}
	return &Sessions{secret: b}
}

func (s *Sessions) Sign(id Identity, ttl time.Duration) string {
	provider := id.Provider
	if provider == "" {
		provider = ProviderLocal
	}
	payload := fmt.Sprintf("%s|%s|%d|%s|%s",
		base64.RawURLEncoding.EncodeToString([]byte(id.Name)), id.Role, time.Now().Add(ttl).Unix(), provider, id.PV)
	return payload + "|" + s.sig(payload)
}

var ErrSessionInvalid = errors.New("login required")

func (s *Sessions) Parse(value string) (Identity, error) {
	parts, err := s.verify(value, 5)
	if err != nil {
		return Identity{}, ErrSessionInvalid
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return Identity{}, ErrSessionInvalid
	}
	name, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !validRoles[parts[1]] || (parts[3] != ProviderLocal && parts[3] != ProviderOIDC) {
		return Identity{}, ErrSessionInvalid
	}
	return Identity{Name: string(name), Role: parts[1], Provider: parts[3], PV: parts[4]}, nil
}

// SignState protects the OIDC login state (state, nonce, PKCE verifier) in a short-lived
// cookie with the same HMAC key, so the flow stays stateless across replicas.
func (s *Sessions) SignState(state, nonce, verifier string, ttl time.Duration) string {
	payload := fmt.Sprintf("%s|%s|%s|%d", state, nonce, verifier, time.Now().Add(ttl).Unix())
	return payload + "|" + s.sig(payload)
}

var ErrStateInvalid = errors.New("login state invalid or expired")

func (s *Sessions) ParseState(value string) (state, nonce, verifier string, err error) {
	parts, err := s.verify(value, 4)
	if err != nil {
		return "", "", "", ErrStateInvalid
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || time.Now().Unix() > exp || parts[0] == "" {
		return "", "", "", ErrStateInvalid
	}
	return parts[0], parts[1], parts[2], nil
}

// verify checks the trailing HMAC and splits the payload into exactly n fields.
func (s *Sessions) verify(value string, n int) ([]string, error) {
	i := strings.LastIndexByte(value, '|')
	if i < 0 {
		return nil, ErrSessionInvalid
	}
	payload, sig := value[:i], value[i+1:]
	if !hmac.Equal([]byte(s.sig(payload)), []byte(sig)) {
		return nil, ErrSessionInvalid
	}
	parts := strings.Split(payload, "|")
	if len(parts) != n {
		return nil, ErrSessionInvalid
	}
	return parts, nil
}

// MapRole picks the highest app role whose group list intersects the user's groups;
// defaultRole applies when nothing matches ("" = refuse the login).
func MapRole(groups []string, roles map[string][]string, defaultRole string) (string, bool) {
	has := map[string]bool{}
	for _, g := range groups {
		has[g] = true
	}
	for _, role := range []string{"admin", "editor", "viewer"} {
		for _, g := range roles[role] {
			if has[g] {
				return role, true
			}
		}
	}
	if defaultRole != "" && validRoles[defaultRole] {
		return defaultRole, true
	}
	return "", false
}

func (s *Sessions) sig(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
