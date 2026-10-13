package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/wonjune95/sealane/internal/auth"
)

const (
	sessionCookie = "sealane_session"
	sessionTTL    = 12 * time.Hour
)

// handleLogin verifies a local account (ConfigMap-backed) and sets the session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.dev {
		writeError(w, http.StatusBadRequest, "BadRequest", "login is not required in dev mode")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "invalid request body")
		return
	}
	role, err := s.accounts.Authenticate(in.Username, in.Password)
	if err != nil {
		slog.Warn("login failed", "user", in.Username)
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrBadCredentials) {
			status = http.StatusInternalServerError // misconfigured account entry
		}
		writeError(w, status, "Unauthorized", err.Error())
		return
	}
	id := auth.Identity{Name: in.Username, Role: role, Provider: auth.ProviderLocal}
	setSessionCookie(w, s.sessions.Sign(id, sessionTTL))
	slog.Info("login", "user", in.Username, "role", role)
	writeJSON(w, http.StatusOK, map[string]string{"user": id.Name, "role": id.Role})
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	clearCookie(w, sessionCookie, "/")
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
}

func clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
