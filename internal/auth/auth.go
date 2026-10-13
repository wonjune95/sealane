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
}

// User returns a stable display identity for audit logs.
func (i Identity) User() string { return i.Name }

// CanWrite reports whether the role may mutate resources.
func (i Identity) CanWrite() bool { return i.Role == "admin" || i.Role == "editor" }

var validRoles = map[string]bool{"admin": true, "editor": true, "viewer": true}

// ValidRole reports whether r is one of the app roles.
func ValidRole(r string) bool { return validRoles[r] }

// Store reads accounts from dir on every call — logins are rare and a mounted
// ConfigMap updates in place, so no caching/watching is needed.
type Store struct {
	dir string

	pwMu    sync.Mutex
	pwCache map[string]bool // "user:hash:plain" → match result (for the initial-password check in capabilities)
}

func NewStore(dir string) *Store { return &Store{dir: dir, pwCache: map[string]bool{}} }

// PasswordIs reports whether the account's current password equals plain.
// It runs on every capabilities call, so results are cached per hash (a new
// password means a new hash and thus a new key — stale entries are harmless and
// grow only with the number of users).
func (s *Store) PasswordIs(username, plain string) bool {
	if username == "" || strings.ContainsAny(username, "/\\") || strings.HasPrefix(username, ".") {
		return false
	}
	b, err := os.ReadFile(filepath.Join(s.dir, username))
	if err != nil {
		return false
	}
	_, hash, ok := strings.Cut(strings.TrimSpace(string(b)), ":")
	if !ok {
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
func (s *Store) Authenticate(username, password string) (string, error) {
	// usernames become file names; reject path tricks before touching the fs.
	if username == "" || strings.ContainsAny(username, "/\\") || strings.HasPrefix(username, ".") {
		return "", ErrBadCredentials
	}
	b, err := os.ReadFile(filepath.Join(s.dir, username))
	if err != nil {
		// burn comparable time so missing vs wrong-password is indistinguishable
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0xF3F0mQ5eGvHq0FhqYb6R1r1uW"), []byte(password))
		return "", ErrBadCredentials
	}
	role, hash, ok := strings.Cut(strings.TrimSpace(string(b)), ":")
	if !ok || !validRoles[role] {
		return "", fmt.Errorf("account %q is misconfigured (expected role:bcryptHash)", username)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", ErrBadCredentials
	}
	return role, nil
}

// Sessions signs and verifies session cookie values: "user|role|expiry|provider|sig".
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
	payload := fmt.Sprintf("%s|%s|%d|%s",
		base64.RawURLEncoding.EncodeToString([]byte(id.Name)), id.Role, time.Now().Add(ttl).Unix(), provider)
	return payload + "|" + s.sig(payload)
}

var ErrSessionInvalid = errors.New("login required")

func (s *Sessions) Parse(value string) (Identity, error) {
	parts, err := s.verify(value, 4)
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
	return Identity{Name: string(name), Role: parts[1], Provider: parts[3]}, nil
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
	if err != nil || time.Now().Unix() > exp {
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
