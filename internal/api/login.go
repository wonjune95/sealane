package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wonjune95/sealane/internal/auth"
)

const (
	sessionCookie = "sealane_session"
	sessionTTL    = 12 * time.Hour
)

// SecureCookies marks every cookie Secure regardless of the request (main sets it when
// OIDC_REDIRECT_URL is https or SECURE_COOKIES=true).
var SecureCookies bool

// secureRequest reports whether cookies for r must be Secure: always when configured, and for
// any request that reached Sealane over TLS or through a proxy that terminated TLS.
func secureRequest(r *http.Request) bool {
	return SecureCookies || r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// handleLogin verifies a local account (ConfigMap-backed) and sets the session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.dev {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "dev-mode-no-login", "login is not required in dev mode")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeCoded(w, http.StatusBadRequest, "BadRequest", "invalid-body", "invalid request body")
		return
	}
	keys := []string{"u:" + in.Username, "ip:" + clientIP(r)}
	for _, k := range keys {
		if d := s.loginThrottle.blocked(k); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeCoded(w, http.StatusTooManyRequests, "TooManyAttempts", "too-many-attempts", "too many failed sign-in attempts; try again later")
			return
		}
	}
	role, pv, err := s.accounts.Authenticate(in.Username, in.Password)
	if err != nil {
		slog.Warn("login failed", "user", in.Username)
		status, code := http.StatusUnauthorized, "invalid-credentials"
		if !errors.Is(err, auth.ErrBadCredentials) {
			status, code = http.StatusInternalServerError, "" // misconfigured account entry
		} else {
			for _, k := range keys {
				s.loginThrottle.fail(k)
			}
		}
		writeCoded(w, status, "Unauthorized", code, err.Error())
		return
	}
	for _, k := range keys {
		s.loginThrottle.reset(k)
	}
	id := auth.Identity{Name: in.Username, Role: role, Provider: auth.ProviderLocal, PV: pv}
	setSessionCookie(w, r, s.sessions.Sign(id, sessionTTL))
	slog.Info("login", "user", in.Username, "role", role, "provider", auth.ProviderLocal)
	writeJSON(w, http.StatusOK, map[string]string{"user": id.Name, "role": id.Role})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r, sessionCookie, "/")
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()), Secure: secureRequest(r),
	})
}

func clearCookie(w http.ResponseWriter, r *http.Request, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Secure: secureRequest(r)})
}
