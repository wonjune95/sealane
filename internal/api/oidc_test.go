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
	"strings"
	"testing"
	"time"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"

	kubefake "k8s.io/client-go/kubernetes/fake"
)

// fakeIdP is a minimal OpenID provider: discovery, JWKS and a token endpoint that mints
// RS256 id_tokens with whatever nonce/groups the test sets.
type fakeIdP struct {
	srv          *httptest.Server
	key          *rsa.PrivateKey
	nonce        string
	groups       []string
	email        string
	lastVerifier string
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
			"jwks_uri": iss + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
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
		claims := map[string]any{
			"iss": iss, "sub": "alice", "aud": "sealane", "iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce, "email": f.email, "groups": f.groups,
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": signRS256(t, key, claims)})
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

func TestOIDCFullFlowMapsHighestRole(t *testing.T) {
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

func TestOIDCIdPErrorsRedirect(t *testing.T) {
	idp := newFakeIdP(t)
	mux, _ := newOIDCMux(t, idp, "viewer")
	q, cookie := startLogin(t, mux)
	rec := callback(t, mux, "error=access_denied&error_description=nope&state="+url.QueryEscape(q.Get("state")), cookie)
	if rec.Header().Get("Location") != "/?oidc_error=idp" {
		t.Fatalf("idp error param: %s", rec.Header().Get("Location"))
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
