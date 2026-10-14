package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"

	kubefake "k8s.io/client-go/kubernetes/fake"
)

// fakeIdP is a minimal OpenID provider: discovery, JWKS and a token endpoint that mints
// RS256 id_tokens with whatever nonce/groups the test sets. aud, exp and signKey
// override the valid defaults ("sealane", +1h, the published key) for negative tests.
// omitGroupsClaim drops groups from the id_token; /userinfo then serves userinfoGroups
// (nil = HTTP 500) to the "at" bearer token and counts calls in userinfoCalls.
type fakeIdP struct {
	srv          *httptest.Server
	key          *rsa.PrivateKey
	nonce        string
	groups       []string
	email        string
	lastVerifier string
	aud          string
	exp          time.Time
	signKey      *rsa.PrivateKey

	omitGroupsClaim bool
	userinfoGroups  []string
	userinfoCalls   int
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, email: "alice@example.com"}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	iss := f.srv.URL
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "userinfo_endpoint": iss + "/userinfo", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json") // x/oauth2 parses untyped bodies as form-encoded
		f.lastVerifier = r.Form.Get("code_verifier")
		if r.Form.Get("code") != "good-code" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		aud, exp, signKey := "sealane", time.Now().Add(time.Hour), key
		if f.aud != "" {
			aud = f.aud
		}
		if !f.exp.IsZero() {
			exp = f.exp
		}
		if f.signKey != nil {
			signKey = f.signKey
		}
		claims := map[string]any{
			"iss": iss, "sub": "alice", "aud": aud, "iat": time.Now().Unix(),
			"exp": exp.Unix(), "nonce": f.nonce, "email": f.email, "groups": f.groups,
		}
		if f.omitGroupsClaim {
			delete(claims, "groups")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": signRS256(t, signKey, claims)})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.userinfoCalls++
		if f.userinfoGroups == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "alice", "groups": f.userinfoGroups})
	})
	return f
}

func signRS256(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	b64 := base64.RawURLEncoding.EncodeToString
	body, _ := json.Marshal(claims)
	signing := b64([]byte(`{"alg":"RS256","typ":"JWT","kid":"k1"}`)) + "." + b64(body)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func oidcTestConfig(idp *fakeIdP, defaultRole string) auth.OIDCConfig {
	return auth.OIDCConfig{
		IssuerURL: idp.srv.URL, ClientID: "sealane", ClientSecret: "secret",
		RedirectURL: "http://sealane.test/api/auth/oidc/callback", Scopes: []string{"openid", "email", "groups"},
		GroupsClaim: "groups", UsernameClaim: "email", DisplayName: "Fake IdP", DefaultRole: defaultRole,
		Roles: map[string][]string{"admin": {"platform-admins"}, "editor": {"sre"}, "viewer": {}},
	}
}

func newOIDCMux(t *testing.T, idp *fakeIdP, defaultRole string) (*http.ServeMux, *Server) {
	t.Helper()
	// capabilities runs a SelfSubjectAccessReview, so the source needs a (fake) kube client.
	srv := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}}, auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.SetOIDC(auth.NewOIDC(oidcTestConfig(idp, defaultRole)))
	mux := http.NewServeMux()
	srv.Routes(mux)
	return mux, srv
}

// startLogin hits /login and returns the IdP redirect query and the state cookie.
func startLogin(t *testing.T, mux *http.ServeMux) (url.Values, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/oidc/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sealane_oidc" {
			return loc.Query(), c
		}
	}
	t.Fatal("no sealane_oidc cookie set")
	return nil, nil
}

func callback(t *testing.T, mux *http.ServeMux, q string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/auth/oidc/callback?"+q, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func issuedSession(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sealane_session" && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestAuthMethodsAdvertisesOIDC(t *testing.T) {
	mux, _ := newOIDCMux(t, newFakeIdP(t), "viewer")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"displayName":"Fake IdP"`) || !strings.Contains(rec.Body.String(), `"local":true`) {
		t.Fatalf("methods = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthMethodsLocalHiddenOnlyWhenSet(t *testing.T) {
	mux, srv := newOIDCMux(t, newFakeIdP(t), "viewer")
	get := func() string {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
		return rec.Body.String()
	}
	if strings.Contains(get(), "localHidden") {
		t.Fatalf("localHidden present by default: %s", get())
	}
	srv.HideLocalLogin(true)
	if b := get(); !strings.Contains(b, `"localHidden":true`) || !strings.Contains(b, `"local":true`) {
		t.Fatalf("methods = %s", b)
	}
}

func TestAuthMethodsLocalHiddenNeedsOIDC(t *testing.T) {
	srv := NewServer(false, &stubSource{}, auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.HideLocalLogin(true)
	mux := http.NewServeMux()
	srv.Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if strings.Contains(rec.Body.String(), "localHidden") {
		t.Fatalf("localHidden without OIDC: %s", rec.Body.String())
	}
}

func TestOIDCFullFlowMapsHighestRole(t *testing.T) {
	logs := captureLogs(t)
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "viewer")
	q, cookie := startLogin(t, mux)
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatalf("authorize URL lacks PKCE/nonce/state: %v", q)
	}
	_, nonce, verifier, err := testSessions.ParseState(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(verifier) {
		t.Fatal("code_challenge is not bound to the verifier in the state cookie")
	}
	idp.nonce, idp.groups = nonce, []string{"sre", "platform-admins"}

	rec := callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("callback = %d %s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if idp.lastVerifier != verifier {
		t.Fatalf("token exchange must send the PKCE verifier from the state cookie")
	}
	sc := issuedSession(rec)
	if sc == nil {
		t.Fatal("no session cookie after successful login")
	}
	id, err := testSessions.Parse(sc.Value)
	if err != nil || id.Name != "alice@example.com" || id.Role != "admin" || id.Provider != auth.ProviderOIDC {
		t.Fatalf("session = %+v (%v)", id, err)
	}
	if strings.Contains(sc.Value, "platform-admins") || strings.Contains(sc.Value, "sre") {
		t.Fatalf("session cookie must not carry groups: %s", sc.Value)
	}
	var login map[string]any
	for _, rec := range logs() {
		if rec["msg"] == "login" {
			login = rec
		}
	}
	if g, _ := login["groups"].([]any); len(g) != 2 || g[0] != "sre" || g[1] != "platform-admins" {
		t.Fatalf("login log groups = %v, want [sre platform-admins] (record %v)", login["groups"], login)
	}

	// capabilities reports the provider; password change is refused for SSO accounts
	req := httptest.NewRequest("GET", "/api/capabilities", nil)
	req.AddCookie(sc)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req)
	if !strings.Contains(rec2.Body.String(), `"authProvider":"oidc"`) || !strings.Contains(rec2.Body.String(), `"authProviderName":"Fake IdP"`) {
		t.Fatalf("capabilities = %s", rec2.Body.String())
	}
	req = httptest.NewRequest("POST", "/api/account/password", strings.NewReader(`{"currentPassword":"x","newPassword":"longenough"}`))
	req.AddCookie(sc)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("password change for an SSO account = %d, want 400", rec3.Code)
	}
}

func TestOIDCCallbackRejectsBadState(t *testing.T) {
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "viewer")
	_, cookie := startLogin(t, mux)
	rec := callback(t, mux, "code=good-code&state=forged", cookie)
	if rec.Header().Get("Location") != "/?oidc_error=state" || issuedSession(rec) != nil {
		t.Fatalf("forged state: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("a forged callback must not clear the state cookie of a sign-in in progress")
	}
	rec = callback(t, mux, "code=good-code&state=whatever", nil)
	if rec.Header().Get("Location") != "/?oidc_error=state" {
		t.Fatalf("missing cookie: %s", rec.Header().Get("Location"))
	}
}

func TestOIDCRefusesUsersWithoutRole(t *testing.T) {
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "")
	q, cookie := startLogin(t, mux)
	_, nonce, _, _ := testSessions.ParseState(cookie.Value)
	idp.nonce, idp.groups = nonce, nil
	rec := callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Header().Get("Location") != "/?oidc_error=access_denied" || issuedSession(rec) != nil {
		t.Fatalf("no-role user: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

// oidcLogin runs one sign-in against idp and returns the callback response.
func oidcLogin(t *testing.T, mux *http.ServeMux, idp *fakeIdP) *httptest.ResponseRecorder {
	t.Helper()
	q, cookie := startLogin(t, mux)
	_, nonce, _, _ := testSessions.ParseState(cookie.Value)
	idp.nonce = nonce
	return callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie)
}

func TestOIDCGroupsFromUserinfoWhenTokenLacksClaim(t *testing.T) {
	logs := captureLogs(t)
	idp := newFakeIdP(t)
	idp.omitGroupsClaim, idp.userinfoGroups = true, []string{"platform-admins"}
	mux, _ := newOIDCMux(t, idp, "viewer")
	sc := issuedSession(oidcLogin(t, mux, idp))
	if sc == nil {
		t.Fatal("no session cookie")
	}
	if id, _ := testSessions.Parse(sc.Value); id.Role != "admin" || idp.userinfoCalls != 1 {
		t.Fatalf("role = %q, userinfo calls = %d; want admin, 1", id.Role, idp.userinfoCalls)
	}
	var login map[string]any
	for _, rec := range logs() {
		if rec["msg"] == "login" {
			login = rec
		}
	}
	if g, _ := login["groups"].([]any); len(g) != 1 || g[0] != "platform-admins" {
		t.Fatalf("login log groups = %v, want [platform-admins]", login["groups"])
	}
}

func TestOIDCEmptyGroupsClaimSkipsUserinfo(t *testing.T) {
	idp := newFakeIdP(t)
	idp.groups, idp.userinfoGroups = []string{}, []string{"platform-admins"}
	mux, _ := newOIDCMux(t, idp, "viewer")
	sc := issuedSession(oidcLogin(t, mux, idp))
	if sc == nil {
		t.Fatal("no session cookie")
	}
	if id, _ := testSessions.Parse(sc.Value); id.Role != "viewer" || idp.userinfoCalls != 0 {
		t.Fatalf("role = %q, userinfo calls = %d; want viewer, 0", id.Role, idp.userinfoCalls)
	}
}

func TestOIDCUserinfoFailureKeepsLogin(t *testing.T) {
	for defaultRole, wantLoc := range map[string]string{"viewer": "/", "": "/?oidc_error=access_denied"} {
		logs := captureLogs(t)
		idp := newFakeIdP(t)
		idp.omitGroupsClaim = true // userinfoGroups nil → /userinfo answers 500
		mux, _ := newOIDCMux(t, idp, defaultRole)
		rec := oidcLogin(t, mux, idp)
		if rec.Header().Get("Location") != wantLoc || idp.userinfoCalls != 1 {
			t.Fatalf("default %q: location = %q, userinfo calls = %d; want %q, 1", defaultRole, rec.Header().Get("Location"), idp.userinfoCalls, wantLoc)
		}
		warned := false
		for _, r := range logs() {
			warned = warned || (r["msg"] == "oidc userinfo" && r["level"] == "WARN")
		}
		if !warned {
			t.Fatalf("default %q: no oidc userinfo warning in %v", defaultRole, logs())
		}
	}
}

func TestOIDCIdPErrorsRedirect(t *testing.T) {
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "viewer")
	q, cookie := startLogin(t, mux)
	rec := callback(t, mux, "error=access_denied&error_description=nope&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Header().Get("Location") != "/?oidc_error=idp" {
		t.Fatalf("idp error param: %s", rec.Header().Get("Location"))
	}
	// An ?error= without our state is a forged link, not an IdP answer.
	if rec = callback(t, mux, "error=access_denied", nil); rec.Header().Get("Location") != "/?oidc_error=state" {
		t.Fatalf("idp error without state: %s", rec.Header().Get("Location"))
	}
	_, nonce, _, _ := testSessions.ParseState(cookie.Value)
	idp.nonce = nonce + "-tampered"
	rec = callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Header().Get("Location") != "/?oidc_error=idp" || issuedSession(rec) != nil {
		t.Fatalf("nonce mismatch: %s", rec.Header().Get("Location"))
	}
	rec = callback(t, mux, "code=bad-code&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Header().Get("Location") != "/?oidc_error=idp" {
		t.Fatalf("bad code: %s", rec.Header().Get("Location"))
	}
}

// The verifier's checks (audience, expiry, signature) are what the whole flow rests on.
func TestOIDCCallbackRejectsBadIDToken(t *testing.T) {
	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for name, spoil := range map[string]func(*fakeIdP){
		"wrong aud":       func(f *fakeIdP) { f.aud = "other" },
		"expired":         func(f *fakeIdP) { f.exp = time.Now().Add(-time.Hour) },
		"unpublished key": func(f *fakeIdP) { f.signKey = foreign },
	} {
		t.Run(name, func(t *testing.T) {
			idp := newFakeIdP(t)
			mux, _ := newOIDCMux(t, idp, "viewer")
			q, cookie := startLogin(t, mux)
			_, nonce, _, _ := testSessions.ParseState(cookie.Value)
			idp.nonce, idp.groups = nonce, []string{"platform-admins"}
			spoil(idp)
			rec := callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?oidc_error=idp" || issuedSession(rec) != nil {
				t.Fatalf("callback = %d %s", rec.Code, rec.Header().Get("Location"))
			}
			cleared := false
			for _, c := range rec.Result().Cookies() {
				cleared = cleared || (c.Name == "sealane_oidc" && c.Value == "" && c.MaxAge < 0)
			}
			if !cleared {
				t.Fatal("state cookie not cleared")
			}
		})
	}
}

func TestCookiesSecureWhenRedirectIsHTTPS(t *testing.T) {
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "viewer")
	if _, c := startLogin(t, mux); c.Secure {
		t.Fatal("state cookie must not be Secure by default (http redirect)")
	}
	SecureCookies = true // what main sets for an https:// OIDC_REDIRECT_URL
	t.Cleanup(func() { SecureCookies = false })
	q, c := startLogin(t, mux)
	if !c.Secure {
		t.Fatal("state cookie must be Secure behind https")
	}
	_, idp.nonce, _, _ = testSessions.ParseState(c.Value)
	if sc := issuedSession(callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), c)); sc == nil || !sc.Secure {
		t.Fatalf("session cookie must be Secure behind https: %v", sc)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/logout", nil))
	if c := rec.Result().Cookies(); len(c) != 1 || !c[0].Secure {
		t.Fatalf("session cookie must be Secure behind https: %v", c)
	}
}

// An SSO user named "admin" must not inherit the local admin's forced password change.
func TestOIDCAdminNotForcedToChangePassword(t *testing.T) {
	dir := t.TempDir()
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	if err := os.WriteFile(filepath.Join(dir, "admin"), []byte("admin:"+string(hash)), 0o600); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	srv := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}}, auth.NewStore(dir), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.SetOIDC(auth.NewOIDC(oidcTestConfig(idp, "viewer")))
	mux := http.NewServeMux()
	srv.Routes(mux)

	q, cookie := startLogin(t, mux)
	_, nonce, _, _ := testSessions.ParseState(cookie.Value)
	idp.nonce, idp.groups, idp.email = nonce, []string{"platform-admins"}, "admin"
	sc := issuedSession(callback(t, mux, "code=good-code&state="+url.QueryEscape(q.Get("state")), cookie))
	if sc == nil {
		t.Fatal("no session cookie after successful login")
	}
	req := httptest.NewRequest("GET", "/api/capabilities", nil)
	req.AddCookie(sc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"mustChangePassword":true`) {
		t.Fatalf("capabilities = %d %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCNotConfigured(t *testing.T) {
	mux := newTestMux(t, &stubSource{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if strings.Contains(rec.Body.String(), "oidc") {
		t.Fatalf("methods must not advertise oidc when unset: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/oidc/login", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("login without oidc = %d, want 404", rec.Code)
	}
}
