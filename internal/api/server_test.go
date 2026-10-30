package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"

	istioapi "istio.io/api/networking/v1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
)

var vsGVR = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"}

const vsTypeID = "virtualservices.networking.istio.io"

// stubSource implements ClientSource for handler tests.
type stubSource struct {
	clients *k8s.Clients
	baseErr error

	// For the request tester: fakes the exec result and captures the argv actually built
	execOut  string
	execErrS string
	execErr  error
	execArgv []string

	// Impersonation: the identity asked for, and the clients handed out for it (nil = clients)
	impersonated []rest.ImpersonationConfig
	asClients    *k8s.Clients
}

func (s *stubSource) Impersonating(as rest.ImpersonationConfig) (*k8s.Clients, error) {
	s.impersonated = append(s.impersonated, as)
	if s.asClients != nil {
		return s.asClients, nil
	}
	return s.clients, nil
}

func (s *stubSource) Base() (*k8s.Clients, error) {
	return s.clients, s.baseErr
}

func (s *stubSource) ResolveType(typeID string) (k8s.ResolvedType, error) {
	if typeID != vsTypeID {
		return k8s.ResolvedType{}, errors.New("unknown resource type: " + typeID)
	}
	return k8s.ResolvedType{
		ResourceType: k8s.ResourceType{Kind: "VirtualService", Group: vsGVR.Group, Resource: vsGVR.Resource, Namespaced: true},
		TypeID:       vsTypeID,
		Version:      vsGVR.Version,
		Installed:    true,
		GVR:          vsGVR,
	}, nil
}

func (s *stubSource) CatalogCached() ([]k8s.ResolvedType, error) { return nil, nil }
func (s *stubSource) DetectCRDs() (k8s.CRDInfo, error)           { return k8s.CRDInfo{}, nil }
func (s *stubSource) SpecSchema(string) (json.RawMessage, error) { return nil, errors.New("none") }
func (s *stubSource) IstiodVersion(context.Context) string       { return "" }

func (s *stubSource) ExecInPod(_ context.Context, _, _, _ string, argv []string, _ int) (string, string, error) {
	s.execArgv = argv
	return s.execOut, s.execErrS, s.execErr
}

var testSessions = auth.NewSessions("test-secret")

// testHash is the stored hash of every fixture account (not a usable password; tests that log
// in write their own). testCookie sessions carry its password version, so they stay valid.
const testHash = "$2a$04$fixture.fixture.fixture.fixture.fixture.fixture.fixtu"

// testStore holds the fixture accounts the tests sign sessions for.
func testStore(t *testing.T) *auth.Store {
	t.Helper()
	dir := t.TempDir()
	for name, role := range map[string]string{"tester": "editor", "ro": "viewer", "admin": "admin", "root": "admin", "bob": "editor", "alice": "viewer"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(role+":"+testHash), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return auth.NewStore(dir)
}

// testCookie is a session cookie value for a fixture account.
func testCookie(name, role string) string {
	return testSessions.Sign(auth.Identity{Name: name, Role: role, PV: auth.PasswordVersion(testHash)}, time.Hour)
}

func newTestMux(t *testing.T, source ClientSource) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	NewServer(false, source, testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{}).Routes(mux)
	return mux
}

// authedReq builds a request carrying a valid editor session cookie.
func authedReq(method, path string, body *strings.Reader) *http.Request {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, body)
	}
	r.AddCookie(&http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("tester", "editor"),
	})
	return r
}

// captureLogs redirects slog to a buffer and returns the emitted JSON records.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return func() []map[string]any {
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err == nil {
				out = append(out, rec)
			}
		}
		return out
	}
}

// A request without a session cookie must be rejected as 401.
func TestWithAuth_MissingSessionIs401(t *testing.T) {
	mux := newTestMux(t, &stubSource{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/resourceTypes", nil))
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "session-required" {
		t.Fatalf("status = %d, want 401 session-required", rec.Code)
	}
}

// A forged/tampered cookie must be rejected as 401.
func TestWithAuth_BadCookieIs401(t *testing.T) {
	mux := newTestMux(t, &stubSource{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/resourceTypes", nil)
	req.AddCookie(&http.Cookie{Name: "sealane_session", Value: "dGVzdGVy|admin|9999999999|forged"})
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// viewer role must not pass mutating methods (403 before any k8s call).
func TestWithAuth_ViewerWriteIs403(t *testing.T) {
	mux := newTestMux(t, &stubSource{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/resources/"+vsTypeID, strings.NewReader(vsBody))
	req.AddCookie(&http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("ro", "viewer"),
	})
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuth_FactoryFailureIs500(t *testing.T) {
	mux := newTestMux(t, &stubSource{baseErr: errors.New("boom")})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/resourceTypes", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// Login round-trip against a real accounts dir: wrong password 401, right
// password sets a cookie that authenticates subsequent requests.
func TestLogin_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alice"), []byte("editor:"+string(hash)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}},
		auth.NewStore(dir), testSessions, AccountsCMRef{}, ClustersSecretRef{}).Routes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"s3cret"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/capabilities", nil)
	req.AddCookie(cookies[0])
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities with session: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func fakeDynamicClients(seed ...runtime.Object) (*k8s.Clients, *dynamicfake.FakeDynamicClient) {
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{vsGVR: "VirtualServiceList"},
		seed...,
	)
	return &k8s.Clients{Dynamic: dyn}, dyn
}

const vsBody = `{
	"apiVersion": "networking.istio.io/v1",
	"kind": "VirtualService",
	"metadata": {"name": "app", "namespace": "default"},
	"spec": {"hosts": ["app.example.com"]}
}`

func findAudit(records []map[string]any) map[string]any {
	for _, r := range records {
		if r["audit"] == true {
			return r
		}
	}
	return nil
}

func TestCreateResource_SuccessIsAudited(t *testing.T) {
	logs := captureLogs(t)
	clients, _ := fakeDynamicClients()
	mux := newTestMux(t, &stubSource{clients: clients})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("POST", "/api/resources/"+vsTypeID, strings.NewReader(vsBody)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	audit := findAudit(logs())
	if audit == nil {
		t.Fatal("no audit record emitted")
	}
	if audit["namespace"] != "default" || audit["name"] != "app" || audit["verb"] != "create" || audit["provider"] != "local" {
		t.Errorf("audit = %v, want create default/app by a local user", audit)
	}
}

// Same name from local and OIDC is two people: the audit log and history keep them apart.
func TestCreateResource_AuditRecordsOIDCProvider(t *testing.T) {
	logs := captureLogs(t)
	clients, _ := fakeDynamicClients()
	mux := newTestMux(t, &stubSource{clients: clients})

	req := httptest.NewRequest("POST", "/api/resources/"+vsTypeID, strings.NewReader(vsBody))
	req.AddCookie(&http.Cookie{Name: "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "tester", Role: "editor", Provider: auth.ProviderOIDC}, time.Hour)})
	mux.ServeHTTP(httptest.NewRecorder(), req)

	if audit := findAudit(logs()); audit == nil || audit["provider"] != "oidc" {
		t.Fatalf("audit = %v, want provider oidc", audit)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/audit", nil))
	if !strings.Contains(rec.Body.String(), `"provider":"oidc"`) {
		t.Fatalf("history entry lacks the provider: %s", rec.Body.String())
	}
}

// A create rejected by the API server must still be audited with the intended
// namespace/name from the request body — "who tried what where" is the point
// of the audit log.
func TestCreateResource_FailureAuditKeepsTarget(t *testing.T) {
	logs := captureLogs(t)
	clients, dyn := fakeDynamicClients()
	dyn.PrependReactor("create", "virtualservices", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("admission webhook denied")
	})
	mux := newTestMux(t, &stubSource{clients: clients})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("POST", "/api/resources/"+vsTypeID, strings.NewReader(vsBody)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}

	audit := findAudit(logs())
	if audit == nil {
		t.Fatal("no audit record emitted")
	}
	if audit["namespace"] != "default" || audit["name"] != "app" {
		t.Errorf("failed-create audit lost its target: %v, want default/app", audit)
	}
}

// If the SA itself is rejected by the API server, capabilities surfaces it as a cluster error
// (502 cluster-auth-failed) instead of half-working, and not as a 401 that would look like a
// signed-out user.
func TestCapabilities_SAUnauthorizedIs502(t *testing.T) {
	kube := kubefake.NewClientset()
	kube.PrependReactor("create", "selfsubjectaccessreviews", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("token rejected")
	})
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kube}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/capabilities", nil))
	if rec.Code != http.StatusBadGateway || errCode(t, rec) != "cluster-auth-failed" {
		t.Fatalf("status = %d, want 502 cluster-auth-failed: %s", rec.Code, rec.Body.String())
	}
}

func TestCapabilities_WithSessionIs200(t *testing.T) {
	AppVersion = "9.9.9-test"
	t.Cleanup(func() { AppVersion = "dev" })
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"appVersion":"9.9.9-test"`) {
		t.Fatalf("capabilities must carry appVersion: %s", rec.Body.String())
	}
}

func TestCreateResource_DryRunIsNotAudited(t *testing.T) {
	logs := captureLogs(t)
	clients, _ := fakeDynamicClients()
	mux := newTestMux(t, &stubSource{clients: clients})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("POST", "/api/resources/"+vsTypeID+"?dryRun=true", strings.NewReader(vsBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if audit := findAudit(logs()); audit != nil {
		t.Errorf("dry-run should not be audited: %v", audit)
	}
}

// Password change: wrong current password 401; correct one patches the accounts
// ConfigMap with a hash the new password verifies against.
func TestPasswordChange_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpw123"), bcrypt.MinCost)
	if err := os.WriteFile(filepath.Join(dir, "alice"), []byte("viewer:"+string(hash)), 0o600); err != nil {
		t.Fatal(err)
	}
	kube := kubefake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "accounts", Namespace: "ns1"},
		Data:       map[string]string{"alice": "viewer:" + string(hash)},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(dir), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	cookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "alice", Role: "viewer", PV: auth.PasswordVersion(string(hash))}, time.Hour),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/account/password", strings.NewReader(`{"currentPassword":"wrong","newPassword":"newpw1234"}`))
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current pw: status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "password-incorrect" {
		t.Fatalf("wrong current pw: code = %q, want password-incorrect", code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/account/password", strings.NewReader(`{"currentPassword":"oldpw123","newPassword":"newpw1234"}`))
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	cm, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	role, newHash, _ := strings.Cut(cm.Data["alice"], ":")
	if role != "viewer" {
		t.Errorf("role = %q, want viewer preserved", role)
	}
	if bcrypt.CompareHashAndPassword([]byte(newHash), []byte("newpw1234")) != nil {
		t.Error("patched hash does not verify the new password")
	}
}

// Account management API: non-admin 403; admin can create (hash verifies),
// list, change role keeping the hash, and delete — but not self-delete.
func TestAccountsAPI_RoundTrip(t *testing.T) {
	kube := kubefake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "accounts", Namespace: "ns1"},
		Data:       map[string]string{"admin": "admin:" + testHash, "root": "admin:" + testHash, "tester": "editor:" + testHash},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("admin", "admin"),
	}
	do := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		mux.ServeHTTP(rec, req)
		return rec
	}

	// editor may not manage accounts
	editorCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("tester", "editor"),
	}
	if rec := do("GET", "/api/accounts", "", editorCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("editor list: status = %d, want 403", rec.Code)
	}

	// create bob
	if rec := do("PUT", "/api/accounts/bob", `{"role":"editor","password":"bobpw1234"}`, adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("create: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cm, _ := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	role, hash, _ := strings.Cut(cm.Data["bob"], ":")
	if role != "editor" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("bobpw1234")) != nil {
		t.Fatalf("created entry %q: role/hash mismatch", cm.Data["bob"])
	}

	bobCookie := &http.Cookie{Name: "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "bob", Role: "editor", PV: auth.PasswordVersion(hash)}, time.Hour)}
	if rec := do("GET", "/api/namespaces", "", bobCookie); rec.Code == http.StatusUnauthorized {
		t.Fatalf("bob's session: status = %d: %s", rec.Code, rec.Body.String())
	}

	// role change without password keeps the hash
	if rec := do("PUT", "/api/accounts/bob", `{"role":"viewer","password":""}`, adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("role change: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cm, _ = kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if cm.Data["bob"] != "viewer:"+hash {
		t.Fatalf("role change: entry = %q, want viewer with same hash", cm.Data["bob"])
	}
	// A session issued for the old role ends at once.
	if rec := do("GET", "/api/namespaces", "", bobCookie); rec.Code != http.StatusUnauthorized || errCode(t, rec) != "session-revoked" {
		t.Fatalf("demoted session: status = %d, want 401 session-revoked: %s", rec.Code, rec.Body.String())
	}

	// list shows both accounts
	rec := do("GET", "/api/accounts", "", adminCookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bob"`) {
		t.Fatalf("list: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// self-delete blocked; deleting bob works
	if rec := do("DELETE", "/api/accounts/admin", "", adminCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("self-delete: status = %d, want 400", rec.Code)
	}
	rootCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("root", "admin"),
	}
	if rec := do("DELETE", "/api/accounts/admin", "", rootCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("nobody should be able to delete the admin account: %d", rec.Code)
	}
	if rec := do("DELETE", "/api/accounts/bob", "", adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cm, _ = kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if _, exists := cm.Data["bob"]; exists {
		t.Error("bob still present after delete")
	}
}

// Fresh install: no accounts CM → EnsureInitialAdmin stores a random password in the initial-admin
// Secret and creates the CM with an admin entry that verifies it. Without a Secret name it does nothing.
func TestEnsureInitialAdmin_FreshInstall(t *testing.T) {
	kube := kubefake.NewClientset()
	s := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{})

	s.EnsureInitialAdmin(context.Background())
	if _, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("no Secret to hold a password, yet an account was made: %v", err)
	}

	s.SetInitialAdminSecret("sealane-initial-admin")
	s.EnsureInitialAdmin(context.Background())
	cm, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("accounts CM not created: %v", err)
	}
	sec, err := kube.CoreV1().Secrets("ns1").Get(context.Background(), "sealane-initial-admin", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("initial-admin Secret not created: %v", err)
	}
	pw := string(sec.Data["password"])
	role, hash, _ := strings.Cut(cm.Data["admin"], ":")
	if role != "admin" || len(pw) < 20 || pw == "admin" {
		t.Fatalf("admin entry = %q, password length %d", cm.Data["admin"], len(pw))
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		t.Error("admin hash does not verify the password in the Secret")
	}
	if s.mustChangePassword("admin") {
		t.Error("a random initial password must not force a change")
	}

	// If an admin already exists, nothing changes.
	s.EnsureInitialAdmin(context.Background())
	cm2, _ := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	sec2, _ := kube.CoreV1().Secrets("ns1").Get(context.Background(), "sealane-initial-admin", metav1.GetOptions{})
	if cm2.Data["admin"] != cm.Data["admin"] || string(sec2.Data["password"]) != pw {
		t.Error("second run must not rotate the admin password")
	}
}

// An existing install whose admin still has the old default password keeps it but must change it.
func TestEnsureInitialAdmin_LegacyDefaultIsKept(t *testing.T) {
	legacy, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	kube := kubefake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "accounts", Namespace: "ns1"},
		Data:       map[string]string{"admin": "admin:" + string(legacy)},
	})
	s := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{})
	s.SetInitialAdminSecret("sealane-initial-admin")
	s.EnsureInitialAdmin(context.Background())
	if _, err := kube.CoreV1().Secrets("ns1").Get(context.Background(), "sealane-initial-admin", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("an admin exists, so no initial password should be made: %v", err)
	}
	if !s.mustChangePassword("admin") {
		t.Error("admin/admin must be forced to change")
	}
}

// Cluster registry API: non-admin 403; invalid kubeconfig 400; a valid-but-
// unreachable kubeconfig is saved (connected=false) and listed; local cannot be deleted.
func TestClustersAPI_RoundTrip(t *testing.T) {
	kube := kubefake.NewClientset()
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("admin", "admin"),
	}
	do := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		var rd *strings.Reader
		if body != "" {
			rd = strings.NewReader(body)
		} else {
			rd = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, rd)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	editorCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("bob", "editor"),
	}
	// A whole ~/.kube/config: only the chosen context may be stored.
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://127.0.0.1:1"}
- name: other
  cluster: {server: "https://127.0.0.1:2"}
users:
- name: u
  user: {token: t}
- name: other-admin
  user: {token: other-secret}
- name: eks
  user:
    exec: {apiVersion: client.authentication.k8s.io/v1beta1, command: aws}
contexts:
- name: x
  context: {cluster: c, user: u}
- name: other
  context: {cluster: other, user: other-admin}
- name: cloud
  context: {cluster: other, user: eks}
current-context: other
`
	body, _ := json.Marshal(map[string]string{"kubeconfig": kubeconfig, "context": "x"})

	if rec := do("PUT", "/api/clusters/prod", string(body), editorCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("editor upsert: status = %d, want 403", rec.Code)
	}
	if rec := do("GET", "/api/clusters", "", editorCookie); !strings.Contains(rec.Body.String(), `"local"`) {
		t.Fatalf("list should contain local: %s", rec.Body.String())
	}
	if rec := do("PUT", "/api/clusters/prod", `{"kubeconfig":"not: a: kubeconfig"}`, adminCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid kubeconfig: status = %d, want 400", rec.Code)
	}
	rec := do("PUT", "/api/clusters/prod", string(body), adminCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"connected":false`) {
		t.Fatalf("unreachable cluster should report connected=false: %s", rec.Body.String())
	}
	sec, err := kube.CoreV1().Secrets("ns1").Get(context.Background(), "clusters", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret not created: %v", err)
	}
	stored := string(sec.Data["prod"])
	if !strings.Contains(stored, "https://127.0.0.1:1") || !strings.Contains(stored, "token: t") ||
		strings.Contains(stored, "other-secret") || !strings.Contains(stored, "current-context: x") {
		t.Fatalf("stored kubeconfig should hold only context x: %q", stored)
	}
	exec, _ := json.Marshal(map[string]string{"kubeconfig": kubeconfig, "context": "cloud"})
	if rec := do("PUT", "/api/clusters/eks", string(exec), adminCookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"kubeconfig-exec"`) {
		t.Fatalf("exec credential: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	missing, _ := json.Marshal(map[string]string{"kubeconfig": kubeconfig, "context": "nope"})
	if rec := do("PUT", "/api/clusters/nope", string(missing), adminCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown context: status = %d, want 400", rec.Code)
	}
	if rec := do("GET", "/api/clusters", "", adminCookie); !strings.Contains(rec.Body.String(), `"prod"`) {
		t.Fatalf("list should contain prod: %s", rec.Body.String())
	}
	if rec := do("DELETE", "/api/clusters/local", "", adminCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete local: status = %d, want 400", rec.Code)
	}
	if rec := do("DELETE", "/api/clusters/prod", "", adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	sec, _ = kube.CoreV1().Secrets("ns1").Get(context.Background(), "clusters", metav1.GetOptions{})
	if _, stillThere := sec.Data["prod"]; stillThere {
		t.Fatal("prod key should be deleted")
	}
}

// Flow map: istio Gateway + attached VS with a resolvable backend appear in the
// graph (exists + endpoint count); mesh-only VS is excluded from the ingress flow.
func TestFlowMap(t *testing.T) {
	kube := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "petclinic", Namespace: "dev"}},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "petclinic", Namespace: "dev"}, //nolint:staticcheck // production flow map reads core Endpoints
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}}}}}, //nolint:staticcheck // see Endpoints above
	)
	istio := istiofake.NewSimpleClientset(
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "dev"},
			Spec: istioapi.VirtualService{
				Hosts:    []string{"shop.example.com"},
				Gateways: []string{"istio-system/ingressgw"},
				Http: []*istioapi.HTTPRoute{{Route: []*istioapi.HTTPRouteDestination{
					{Destination: &istioapi.Destination{Host: "petclinic"}}}}},
			}},
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "mesh-only", Namespace: "dev"},
			Spec: istioapi.VirtualService{Hosts: []string{"internal"}, Gateways: []string{"mesh"}}},
	)
	// The istio fake mis-buckets the Gateway kind when seeding (version alias quirk) — insert via typed Create.
	if _, err := istio.NetworkingV1().Gateways("istio-system").Create(context.Background(),
		&istionet.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "ingressgw", Namespace: "istio-system"},
			Spec: istioapi.Gateway{Servers: []*istioapi.Server{{Hosts: []string{"shop.example.com"}}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	// Egress: gateway with the egressgateway selector + VS to an external destination + covering SE
	if _, err := istio.NetworkingV1().Gateways("istio-system").Create(context.Background(),
		&istionet.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "egressgw", Namespace: "istio-system"},
			Spec: istioapi.Gateway{Selector: map[string]string{"istio": "egressgateway"}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := istio.NetworkingV1().VirtualServices("dev").Create(context.Background(),
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "to-ext", Namespace: "dev"},
			Spec: istioapi.VirtualService{
				Hosts:    []string{"api.example.com"},
				Gateways: []string{"istio-system/egressgw"},
				Http: []*istioapi.HTTPRoute{{Route: []*istioapi.HTTPRouteDestination{
					{Destination: &istioapi.Destination{Host: "api.example.com"}}}}},
			}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := istio.NetworkingV1().ServiceEntries("dev").Create(context.Background(),
		&istionet.ServiceEntry{ObjectMeta: metav1.ObjectMeta{Name: "ext-se", Namespace: "dev"},
			Spec: istioapi.ServiceEntry{Hosts: []string{"*.example.com"}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	gw := gwfake.NewClientset(
		&gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "hr", Namespace: "dev"},
			Spec: gwapi.HTTPRouteSpec{
				Hostnames:       []gwapi.Hostname{"api.example.com"},
				CommonRouteSpec: gwapi.CommonRouteSpec{ParentRefs: []gwapi.ParentReference{{Name: "tgw"}}},
				Rules: []gwapi.HTTPRouteRule{{BackendRefs: []gwapi.HTTPBackendRef{
					{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "missing-svc"}}}}}},
			}},
	)
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kube, Istio: istio, Gateway: gw}})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/flowmap", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Gateways []struct {
			Name   string
			Egress bool
		}
		Routes []struct {
			Name     string
			Gateways []string
			Backends []struct {
				Name         string
				Exists       bool
				Endpoints    int
				External     bool
				ServiceEntry string
			}
		}
		ServiceEntries []struct{ Name string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Gateways) != 2 {
		t.Fatalf("gateways = %+v", out.Gateways)
	}
	for _, g := range out.Gateways {
		if wantEgress := g.Name == "egressgw"; g.Egress != wantEgress {
			t.Fatalf("gateway %s egress = %v", g.Name, g.Egress)
		}
	}
	if len(out.ServiceEntries) != 1 || out.ServiceEntries[0].Name != "ext-se" {
		t.Fatalf("serviceEntries = %+v", out.ServiceEntries)
	}
	if len(out.Routes) != 3 {
		t.Fatalf("mesh-only VS should be excluded, routes = %+v", out.Routes)
	}
	for _, rt := range out.Routes {
		switch rt.Name {
		case "shop":
			b := rt.Backends[0]
			if !b.Exists || b.Endpoints != 2 || b.Name != "petclinic" {
				t.Fatalf("shop backend = %+v", b)
			}
			if rt.Gateways[0] != "istio-system/ingressgw" {
				t.Fatalf("shop gateways = %v", rt.Gateways)
			}
		case "hr":
			if rt.Backends[0].Exists {
				t.Fatal("missing-svc should not exist")
			}
			if rt.Gateways[0] != "dev/tgw" {
				t.Fatalf("hr gateways = %v", rt.Gateways)
			}
		case "to-ext":
			b := rt.Backends[0]
			if !b.External || b.ServiceEntry != "dev/ext-se" {
				t.Fatalf("to-ext backend = %+v (wildcard SE match failed)", b)
			}
		}
	}
}

// Initial-password guard: capabilities flags mustChangePassword while the
// account's password is "admin"; changing it clears the flag immediately
// (in-memory marking that does not wait for the mount sync).
func TestMustChangePassword_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	if err := os.WriteFile(filepath.Join(dir, "admin"), []byte("admin:"+string(hash)), 0o600); err != nil {
		t.Fatal(err)
	}
	kube := kubefake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "accounts", Namespace: "ns1"},
		Data:       map[string]string{"admin": "admin:" + string(hash)},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(dir), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{}).Routes(mux)
	cookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin", PV: auth.PasswordVersion(string(hash))}, time.Hour),
	}
	caps := func() string {
		req := httptest.NewRequest("GET", "/api/capabilities", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if !strings.Contains(caps(), `"mustChangePassword":true`) {
		t.Fatalf("initial password but flag missing: %s", caps())
	}
	// Until the password is changed, everything but capabilities and the change itself is refused.
	req := httptest.NewRequest("GET", "/api/namespaces", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "password-change-required" {
		t.Fatalf("API before the change: %d %s, want 403 password-change-required", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("POST", "/api/account/password",
		strings.NewReader(`{"currentPassword":"admin","newPassword":"newpass123"}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("password change: %d %s", rec.Code, rec.Body.String())
	}
	// The old session predates the new password, so it ends; the response carries a fresh one.
	if body := caps(); !strings.Contains(body, "session-revoked") {
		t.Fatalf("old session still valid after the password change: %s", body)
	}
	var fresh *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sealane_session" {
			fresh = c
		}
	}
	if fresh == nil {
		t.Fatal("password change did not re-issue the session cookie")
	}
	// The mounted directory still has the old hash, but the store reads the ConfigMap: the flag clears at once.
	cookie = fresh
	if body := caps(); strings.Contains(body, `"mustChangePassword":true`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("new session: %s", body)
	}
}

// Clusters list with ?status=true: local is connected, an unreachable remote
// reports connected=false with an error.
func TestClustersList_Status(t *testing.T) {
	kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster: {server: \"https://127.0.0.1:1\"}\nusers:\n- name: u\n  user: {token: t}\ncontexts:\n- name: x\n  context: {cluster: c, user: u}\ncurrent-context: x\n"
	kube := kubefake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "clusters", Namespace: "ns1"},
		Data:       map[string][]byte{"prod": []byte(kubeconfig)},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	type entry struct {
		Name      string
		Connected *bool
	}
	list := func(name, role string) ([]entry, string) {
		req := httptest.NewRequest("GET", "/api/clusters?status=true", nil)
		req.AddCookie(&http.Cookie{Name: "sealane_session", Value: testCookie(name, role)})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var out []entry
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v: %s", name, err, rec.Body.String())
		}
		return out, rec.Body.String()
	}
	// Only admins get status: it makes Sealane contact every registered cluster.
	if out, body := list("ro", "viewer"); len(out) != 2 || out[0].Connected != nil || out[1].Connected != nil {
		t.Fatalf("viewer got cluster status: %s", body)
	}
	out, body := list("admin", "admin")
	if len(out) != 2 || out[0].Name != "local" || out[0].Connected == nil || !*out[0].Connected {
		t.Fatalf("local status wrong: %s", body)
	}
	if out[1].Connected == nil || *out[1].Connected {
		t.Fatalf("unreachable prod should be connected=false: %s", body)
	}
}

// Route simulator: for a VS the first match in declaration order wins (header conditions
// included), and rules that fail to match are kept with a reason. For HTTPRoute,
// precedence (exact > prefix) wins, not order.
func TestRouteMatch(t *testing.T) {
	kube := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "dev"}},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "dev"}, //nolint:staticcheck // production flow map reads core Endpoints
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}}, //nolint:staticcheck // see Endpoints above
	)
	istio := istiofake.NewSimpleClientset()
	if _, err := istio.NetworkingV1().VirtualServices("dev").Create(context.Background(),
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "shop-vs", Namespace: "dev"},
			Spec: istioapi.VirtualService{
				Hosts:    []string{"shop.example.com"},
				Gateways: []string{"istio-system/gw"},
				Http: []*istioapi.HTTPRoute{
					{ // canary: v2 only when the x-user=beta header is present
						Match: []*istioapi.HTTPMatchRequest{{
							Headers: map[string]*istioapi.StringMatch{
								"x-user": {MatchType: &istioapi.StringMatch_Exact{Exact: "beta"}},
							},
						}},
						Route: []*istioapi.HTTPRouteDestination{
							{Destination: &istioapi.Destination{Host: "shop", Subset: "v2"}}},
					},
					{ // default route
						Route: []*istioapi.HTTPRouteDestination{
							{Destination: &istioapi.Destination{Host: "shop", Subset: "v1"}}},
					},
				},
			}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := istio.NetworkingV1().DestinationRules("dev").Create(context.Background(),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Name: "shop-dr", Namespace: "dev"},
			Spec: istioapi.DestinationRule{Host: "shop", Subsets: []*istioapi.Subset{{Name: "v1"}}}},
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	exact, prefix := gwapi.PathMatchExact, gwapi.PathMatchPathPrefix
	pApi, pApiUsers := "/api", "/api/users"
	gw := gwfake.NewClientset(
		&gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "api-hr", Namespace: "dev"},
			Spec: gwapi.HTTPRouteSpec{
				Hostnames: []gwapi.Hostname{"api.example.com"},
				Rules: []gwapi.HTTPRouteRule{
					{Matches: []gwapi.HTTPRouteMatch{{Path: &gwapi.HTTPPathMatch{Type: &prefix, Value: &pApi}}}},
					{Matches: []gwapi.HTTPRouteMatch{{Path: &gwapi.HTTPPathMatch{Type: &exact, Value: &pApiUsers}}}},
				},
			}},
	)
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kube, Istio: istio, Gateway: gw}})
	get := func(q string) routeMatchResult {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, authedReq("GET", "/api/routematch?"+q, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var out routeMatchResult
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// no header → rule 1 does not match, rule 2 (default) wins
	out := get("host=shop.example.com&path=/cart")
	if !out.Matched || len(out.Destinations) != 1 || out.Destinations[0].Subset != "v1" {
		t.Fatalf("default route should be v1: %+v", out)
	}
	if out.Destinations[0].Endpoints != 1 || !out.Destinations[0].Exists {
		t.Fatalf("destination verification failed: %+v", out.Destinations[0])
	}
	if !out.Destinations[0].SubsetOK || out.Destinations[0].SubsetDR != "shop-dr" {
		t.Fatalf("v1 subset should be defined in the DR: %+v", out.Destinations[0])
	}
	var rejected *routeCandidate
	for i := range out.Candidates {
		if !out.Candidates[i].Matched {
			rejected = &out.Candidates[i]
		}
	}
	if rejected == nil || !strings.Contains(rejected.Reason, "x-user") {
		t.Fatalf("non-match reason should mention the header: %+v", out.Candidates)
	}

	// header present → rule 1 (v2) wins. v2 is not in the DR, so subsetOk=false
	out = get("host=shop.example.com&path=/cart&header=x-user:beta")
	if !out.Matched || out.Destinations[0].Subset != "v2" {
		t.Fatalf("with the header it should go to v2: %+v", out)
	}
	if out.Destinations[0].SubsetOK {
		t.Fatal("v2 subset is not in the DR, so subsetOk should be false")
	}

	// host mismatch → no match
	if out = get("host=other.example.com&path=/"); out.Matched {
		t.Fatalf("a different host must not match: %+v", out)
	}

	// HTTPRoute: prefix is declared first, but exact wins by precedence
	out = get("host=api.example.com&path=/api/users")
	if !out.Matched {
		t.Fatalf("HTTPRoute match failed: %+v", out)
	}
	var winner *routeCandidate
	for i := range out.Candidates {
		if out.Candidates[i].Winner {
			winner = &out.Candidates[i]
		}
	}
	if winner == nil || winner.RuleIndex != 1 {
		t.Fatalf("exact rule (index 1) should win: %+v", out.Candidates)
	}

	// PathPrefix uses segment boundaries — /apifoo must not match the /api prefix
	if out = get("host=api.example.com&path=/apifoo"); out.Matched {
		t.Fatalf("/apifoo must not match the /api prefix: %+v", out)
	}
}

// Route simulator: a destination whose Service does not expose the request port is flagged
// (Envoy answers 503 with no cluster), while a gateway VS without a destination port uses
// the single port of the Service the way Istio does.
func TestRouteMatchPort(t *testing.T) {
	kube := kubefake.NewClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "ns"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}}})
	istio := istiofake.NewSimpleClientset(
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "ns"},
			Spec: istioapi.VirtualService{Hosts: []string{"echo.ns.svc.cluster.local"},
				Http: []*istioapi.HTTPRoute{{Route: []*istioapi.HTTPRouteDestination{
					{Destination: &istioapi.Destination{Host: "echo"}}}}}}},
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "echo-ingress", Namespace: "ns"},
			Spec: istioapi.VirtualService{Hosts: []string{"echo.example.test"}, Gateways: []string{"echo-gw"},
				Http: []*istioapi.HTTPRoute{{Route: []*istioapi.HTTPRouteDestination{
					{Destination: &istioapi.Destination{Host: "echo"}}}}}}},
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "echo-pinned", Namespace: "ns"},
			Spec: istioapi.VirtualService{Hosts: []string{"pinned.example.test"}, Gateways: []string{"echo-gw"},
				Http: []*istioapi.HTTPRoute{{Route: []*istioapi.HTTPRouteDestination{
					{Destination: &istioapi.Destination{Host: "echo", Port: &istioapi.PortSelector{Number: 9090}}}}}}}},
	)
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewClientset()}})
	get := func(q string) routeMatchResult {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, authedReq("GET", "/api/routematch?"+q, nil))
		var out routeMatchResult
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Destinations) != 1 {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		return out
	}

	out := get("host=echo.ns.svc.cluster.local&path=/")
	if d := out.Destinations[0]; d.PortOK || len(d.ServicePorts) != 1 || d.ServicePorts[0] != 8080 {
		t.Fatalf("port 80 is not exposed, portOk should be false: %+v", d)
	}
	want := "destination ns/echo does not expose port 80 (service ports: 8080) — Envoy will answer 503"
	if !slices.Contains(out.Notes, want) {
		t.Fatalf("missing port note: %q", out.Notes)
	}
	if out = get("host=echo.ns.svc.cluster.local&path=/&scheme=https"); !slices.ContainsFunc(out.Notes,
		func(n string) bool { return strings.Contains(n, "port 443") }) {
		t.Fatalf("https without a port should check 443: %q", out.Notes)
	}
	if out = get("host=echo.ns.svc.cluster.local&path=/&port=8080"); !out.Destinations[0].PortOK || len(out.Notes) != 0 {
		t.Fatalf("port 8080 is exposed: %+v", out)
	}
	if out = get("host=echo.example.test&path=/"); !out.Destinations[0].PortOK {
		t.Fatalf("gateway VS without a destination port uses the single service port: %+v", out)
	}
	if out = get("host=pinned.example.test&path=/&port=8080"); out.Destinations[0].PortOK {
		t.Fatalf("an explicit destination port 9090 is checked, not the request port: %+v", out)
	}
}

// Request tester: 403 when disabled, 403 for non-admins; when enabled it builds argv
// without a shell, parses curl output and attaches an Istio-context hint.
func TestRequestTester(t *testing.T) {
	src := &stubSource{
		clients: &k8s.Clients{Kube: kubefake.NewClientset()},
		execOut: "HTTP/1.1 403 Forbidden\r\nserver: istio-envoy\r\nx-envoy-upstream-service-time: 3\r\n\r\nRBAC: access denied",
	}
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("admin", "admin"),
	}
	body := `{"namespace":"dev","pod":"p1","method":"GET","url":"http://shop.dev:8080/api","headers":{"x-user":"beta"}}`
	send := func(mux *http.ServeMux, cookie *http.Cookie, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/requesttest", strings.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// disabled by default → 403
	off := newTestMux(t, src)
	if rec := send(off, adminCookie, body); rec.Code != http.StatusForbidden {
		t.Fatalf("should be 403 while disabled: %d %s", rec.Code, rec.Body.String())
	}

	// enabled, but editor is rejected
	on := http.NewServeMux()
	srv := NewServer(false, src, testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.EnableRequestTester()
	srv.Routes(on)
	editorCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("bob", "editor"),
	}
	if rec := send(on, editorCookie, body); rec.Code != http.StatusForbidden {
		t.Fatalf("editor should get 403: %d", rec.Code)
	}

	// admin passes — response parsing + hint
	rec := send(on, adminCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out reqTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != 403 || out.Headers["server"] != "istio-envoy" || out.Body != "RBAC: access denied" {
		t.Fatalf("failed to parse curl output: %+v", out)
	}
	if !strings.Contains(out.Hint, "AuthorizationPolicy") {
		t.Fatalf("missing 403 RBAC hint: %q", out.Hint)
	}

	// argv is built without a shell, and the URL comes after "--"
	argv := strings.Join(src.execArgv, " ")
	if src.execArgv[0] != "curl" || !strings.Contains(argv, "-H x-user: beta") {
		t.Fatalf("argv built incorrectly: %v", src.execArgv)
	}
	if src.execArgv[len(src.execArgv)-2] != "--" {
		t.Fatalf("URL should be preceded by the -- separator: %v", src.execArgv)
	}

	// dangerous input is rejected: bad scheme, header newline injection, unsupported method
	for _, bad := range []string{
		`{"namespace":"dev","pod":"p1","url":"file:///etc/passwd"}`,
		`{"namespace":"dev","pod":"p1","url":"http://x/","headers":{"a":"b\r\nInjected: 1"}}`,
		`{"namespace":"dev","pod":"p1","url":"http://x/","method":"TRACE"}`,
		`{"pod":"p1","url":"http://x/"}`,
	} {
		if rec := send(on, adminCookie, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("input that should be rejected was accepted (%s): %d", bad, rec.Code)
		}
	}

	// container without curl — this surfaces as an exec error, not in stderr
	src.execOut, src.execErrS, src.execErr = "", "", errors.New(`OCI runtime exec failed: exec: "curl": executable file not found in $PATH`)
	rec = send(on, adminCookie, body)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !strings.Contains(out.Error, "curl is not available") {
		t.Fatalf("missing curl-not-available message: %+v", out)
	}
}

// Pod list: Istio 1.29+ native sidecars (initContainers with restartPolicy=Always) must
// also count as mesh. Regular init containers cannot be exec'd into, so they are left out.
func TestPodList_NativeSidecar(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	kube := kubefake.NewClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "curl-1", Namespace: "demo"},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "istio-init"},                          // real init — excluded
					{Name: "istio-proxy", RestartPolicy: &always}, // native sidecar
				},
				Containers: []corev1.Container{{Name: "curl"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "plain-1", Namespace: "demo"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	mux := http.NewServeMux()
	srv := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.EnableRequestTester()
	srv.Routes(mux)
	req := httptest.NewRequest("GET", "/api/pods?ns=demo", nil)
	req.AddCookie(&http.Cookie{
		Name:  "sealane_session",
		Value: testCookie("admin", "admin"),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var pods []podRef
	if err := json.Unmarshal(rec.Body.Bytes(), &pods); err != nil {
		t.Fatalf("%v (body=%s)", err, rec.Body.String())
	}
	if len(pods) != 2 {
		t.Fatalf("pods = %+v", pods)
	}
	if !pods[0].Mesh {
		t.Fatalf("native-sidecar pod should count as mesh: %+v", pods[0])
	}
	if strings.Join(pods[0].Containers, ",") != "istio-proxy,curl" {
		t.Fatalf("regular init container should be dropped and the sidecar kept: %v", pods[0].Containers)
	}
	if pods[1].Mesh {
		t.Fatalf("pod without a sidecar should not count as mesh: %+v", pods[1])
	}
}

// ServiceEntries of the same name in several namespaces: the route's own namespace wins.
func TestServiceEntryFor_PrefersRouteNamespace(t *testing.T) {
	ses := []flowServiceEntry{
		{Namespace: "a", Name: "pay", Hosts: []string{"*.example.com"}},
		{Namespace: "b", Name: "pay", Hosts: []string{"pay.example.com"}},
	}
	for ns, want := range map[string]string{"b": "b/pay", "a": "a/pay", "c": "a/pay"} {
		if got := serviceEntryFor(ses, "pay.example.com", ns); got != want {
			t.Errorf("route in %s: got %q, want %q", ns, got, want)
		}
	}
	if got := serviceEntryFor(ses, "other.org", "a"); got != "" {
		t.Errorf("uncovered host matched %q", got)
	}
}

// Two replicas on a fresh install: the one that loses the Secret write uses the winner's password,
// and its ConfigMap create that finds the ConfigMap already made patches it instead.
func TestEnsureInitialAdmin_SecondReplicaUsesTheStoredPassword(t *testing.T) {
	kube := kubefake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sealane-initial-admin", Namespace: "ns1"},
		Data:       map[string][]byte{"password": []byte("from-the-other-replica")},
	})
	created := false
	kube.PrependReactor("create", "configmaps", func(a ktesting.Action) (bool, runtime.Object, error) {
		if created {
			return false, nil, nil
		}
		created = true // the other replica creates the ConfigMap between our read and our create
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "accounts", Namespace: "ns1"}, Data: map[string]string{"admin": "admin:other-hash"}}
		if err := kube.Tracker().Add(cm); err != nil {
			t.Fatal(err)
		}
		return true, nil, apierrors.NewAlreadyExists(corev1.Resource("configmaps"), "accounts")
	})
	s := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{})
	s.SetInitialAdminSecret("sealane-initial-admin")
	s.EnsureInitialAdmin(context.Background())
	cm, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _ := strings.Cut(cm.Data["admin"], ":")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("from-the-other-replica")) != nil {
		t.Errorf("admin entry %q does not verify the stored password", cm.Data["admin"])
	}
}

// A transient API error at first start is retried until the admin exists.
func TestKeepEnsuringInitialAdmin_RetriesTransientErrors(t *testing.T) {
	kube := kubefake.NewClientset()
	failures := 2
	kube.PrependReactor("create", "secrets", func(ktesting.Action) (bool, runtime.Object, error) {
		if failures > 0 {
			failures--
			return true, nil, apierrors.NewServiceUnavailable("etcd leader change")
		}
		return false, nil, nil
	})
	s := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{})
	s.SetInitialAdminSecret("sealane-initial-admin")
	if s.EnsureInitialAdmin(context.Background()) {
		t.Fatal("a failed Secret write must not count as settled")
	}
	s.KeepEnsuringInitialAdmin(context.Background(), time.Millisecond)
	cm, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if err != nil || !strings.HasPrefix(cm.Data["admin"], "admin:") {
		t.Fatalf("admin not created after retries: %v %v", err, cm)
	}
}

// Deleting with cleanup when the cluster already refuses the stored token (an earlier cleanup
// removed the account, then the local delete failed): the registration is removed.
func TestClusterDelete_CleanupAfterTokenRevoked(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unauthorized","code":401}`))
	}))
	defer srv.Close()
	kc := "apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster: {server: \"" + srv.URL + "\", insecure-skip-tls-verify: true}\n" +
		"users:\n- name: sealane\n  user: {token: gone}\ncontexts:\n- name: x\n  context: {cluster: c, user: sealane, namespace: sealane-remote}\ncurrent-context: x\n"
	kube := kubefake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "clusters", Namespace: "ns1"},
		Data:       map[string][]byte{"old": []byte(kc)},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	req := httptest.NewRequest("DELETE", "/api/clusters/old?cleanup=true", nil)
	req.AddCookie(&http.Cookie{Name: "sealane_session", Value: testCookie("admin", "admin")})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	sec, _ := kube.CoreV1().Secrets("ns1").Get(context.Background(), "clusters", metav1.GetOptions{})
	if _, ok := sec.Data["old"]; ok {
		t.Error("registration still there")
	}
}
