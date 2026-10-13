package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/wonjune95/sealane/internal/auth"
)

const (
	oidcStateCookie = "sealane_oidc"
	oidcStatePath   = "/api/auth/oidc"
	oidcStateTTL    = 10 * time.Minute
)

// SetOIDC enables single sign-on; nil (the default) leaves only local accounts.
func (s *Server) SetOIDC(o *auth.OIDC) { s.oidc = o }

// handleAuthMethods tells the login page which methods exist. Unauthenticated by design.
func (s *Server) handleAuthMethods(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"local": !s.dev}
	if s.oidc != nil {
		out["oidc"] = map[string]any{"enabled": true, "displayName": s.oidc.DisplayName()}
	}
	writeJSON(w, http.StatusOK, out)
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no entropy — nothing sane to do
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// handleOIDCLogin starts the authorization-code flow: state, nonce and the PKCE verifier
// travel in a signed cookie so any replica can finish the flow.
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		writeError(w, http.StatusNotFound, "NotFound", "single sign-on is not configured")
		return
	}
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	target, err := s.oidc.AuthCodeURL(r.Context(), state, nonce, verifier)
	if err != nil {
		slog.Warn("oidc login", "err", err)
		http.Redirect(w, r, "/?oidc_error=idp", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: s.sessions.SignState(state, nonce, verifier, oidcStateTTL),
		Path: oidcStatePath, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(oidcStateTTL.Seconds()),
		Secure: SecureCookies,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

// handleOIDCCallback finishes the flow. Failures never leak IdP details to the browser:
// they are logged and the login page shows a generic message keyed by oidc_error.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(code string, err error) {
		slog.Warn("oidc callback failed", "reason", code, "err", err)
		clearCookie(w, oidcStateCookie, oidcStatePath)
		http.Redirect(w, r, "/?oidc_error="+code, http.StatusFound)
	}
	if s.oidc == nil {
		fail("idp", errors.New("single sign-on is not configured"))
		return
	}
	q := r.URL.Query()
	c, err := r.Cookie(oidcStateCookie)
	if err != nil {
		fail("state", err)
		return
	}
	// A state failure keeps the cookie: a forged callback link must not abort a real sign-in in progress.
	state, nonce, verifier, err := s.sessions.ParseState(c.Value)
	if err != nil || state != q.Get("state") {
		slog.Warn("oidc callback failed", "reason", "state", "err", err)
		http.Redirect(w, r, "/?oidc_error=state", http.StatusFound)
		return
	}
	// Only after the state check: a crafted ?error= link without our state is a state failure, not an IdP one.
	if e := q.Get("error"); e != "" {
		fail("idp", errors.New(e+": "+q.Get("error_description")))
		return
	}
	id, err := s.oidc.Exchange(r.Context(), q.Get("code"), verifier, nonce)
	if errors.Is(err, auth.ErrNoRole) {
		fail("access_denied", err)
		return
	}
	if err != nil {
		fail("idp", err)
		return
	}
	clearCookie(w, oidcStateCookie, oidcStatePath)
	setSessionCookie(w, s.sessions.Sign(id, sessionTTL))
	slog.Info("login", "user", id.Name, "role", id.Role, "provider", id.Provider)
	http.Redirect(w, r, "/", http.StatusFound)
}
