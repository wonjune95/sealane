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

	// 요청 테스터용: exec 결과를 흉내내고 실제로 만들어진 argv를 캡처한다
	execOut  string
	execErrS string
	execErr  error
	execArgv []string
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

func newTestMux(t *testing.T, source ClientSource) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	NewServer(false, source, auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{}).Routes(mux)
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
		Value: testSessions.Sign(auth.Identity{Name: "tester", Role: "editor"}, time.Hour),
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
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
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
		Value: testSessions.Sign(auth.Identity{Name: "ro", Role: "viewer"}, time.Hour),
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
	if audit["namespace"] != "default" || audit["name"] != "app" || audit["verb"] != "create" {
		t.Errorf("audit = %v, want create default/app", audit)
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

// If the SA itself is rejected by the API server, capabilities surfaces the 401
// instead of half-working.
func TestCapabilities_SAUnauthorizedIs401(t *testing.T) {
	kube := kubefake.NewClientset()
	kube.PrependReactor("create", "selfsubjectaccessreviews", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("token rejected")
	})
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kube}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/capabilities", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
}

func TestCapabilities_WithSessionIs200(t *testing.T) {
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
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
		Value: testSessions.Sign(auth.Identity{Name: "alice", Role: "viewer"}, time.Hour),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/account/password", strings.NewReader(`{"currentPassword":"wrong","newPassword":"newpw1234"}`))
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current pw: status = %d, want 401: %s", rec.Code, rec.Body.String())
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
		Data:       map[string]string{"admin": "admin:x"},
	})
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin"}, time.Hour),
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
		Value: testSessions.Sign(auth.Identity{Name: "tester", Role: "editor"}, time.Hour),
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

	// role change without password keeps the hash
	if rec := do("PUT", "/api/accounts/bob", `{"role":"viewer","password":""}`, adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("role change: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cm, _ = kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if cm.Data["bob"] != "viewer:"+hash {
		t.Fatalf("role change: entry = %q, want viewer with same hash", cm.Data["bob"])
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
		Value: testSessions.Sign(auth.Identity{Name: "root", Role: "admin"}, time.Hour),
	}
	if rec := do("DELETE", "/api/accounts/admin", "", rootCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("admin 계정은 누구도 삭제 못해야 함: %d", rec.Code)
	}
	if rec := do("DELETE", "/api/accounts/bob", "", adminCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cm, _ = kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if _, exists := cm.Data["bob"]; exists {
		t.Error("bob still present after delete")
	}
}

// Fresh install: no accounts CM → EnsureInitialAdmin creates it with an
// admin entry whose password is "admin".
func TestEnsureInitialAdmin_FreshInstall(t *testing.T) {
	kube := kubefake.NewClientset()
	s := NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{Namespace: "ns1", Name: "accounts"}, ClustersSecretRef{})

	s.EnsureInitialAdmin(context.Background())

	cm, err := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("accounts CM not created: %v", err)
	}
	role, hash, _ := strings.Cut(cm.Data["admin"], ":")
	if role != "admin" {
		t.Fatalf("admin entry = %q, want admin role", cm.Data["admin"])
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("admin")) != nil {
		t.Error("initial admin hash does not verify password \"admin\"")
	}

	// 이미 admin이 있으면 아무것도 바꾸지 않는다.
	s.EnsureInitialAdmin(context.Background())
	cm2, _ := kube.CoreV1().ConfigMaps("ns1").Get(context.Background(), "accounts", metav1.GetOptions{})
	if cm2.Data["admin"] != cm.Data["admin"] {
		t.Error("second run must not rotate the admin password")
	}
}

// Cluster registry API: non-admin 403; invalid kubeconfig 400; a valid-but-
// unreachable kubeconfig is saved (connected=false) and listed; local은 삭제 불가.
func TestClustersAPI_RoundTrip(t *testing.T) {
	kube := kubefake.NewClientset()
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kube}},
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin"}, time.Hour),
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
		Value: testSessions.Sign(auth.Identity{Name: "bob", Role: "editor"}, time.Hour),
	}
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://127.0.0.1:1"}
users:
- name: u
  user: {token: t}
contexts:
- name: x
  context: {cluster: c, user: u}
current-context: x
`
	body, _ := json.Marshal(map[string]string{"kubeconfig": kubeconfig})

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
	if string(sec.Data["prod"]) != kubeconfig {
		t.Fatalf("stored kubeconfig mismatch: %q", sec.Data["prod"])
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
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "petclinic", Namespace: "dev"},
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}}}}},
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
	// istio fake는 시딩 시 Gateway kind를 잘못 버킷팅한다(버전 alias quirk) — typed Create로 넣는다.
	if _, err := istio.NetworkingV1().Gateways("istio-system").Create(context.Background(),
		&istionet.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "ingressgw", Namespace: "istio-system"},
			Spec: istioapi.Gateway{Servers: []*istioapi.Server{{Hosts: []string{"shop.example.com"}}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	// 이그레스: egressgateway 셀렉터 게이트웨이 + 외부 목적지 VS + 커버하는 SE
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
	gw := gwfake.NewSimpleClientset(
		&gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "hr", Namespace: "dev"},
			Spec: gwapi.HTTPRouteSpec{
				Hostnames: []gwapi.Hostname{"api.example.com"},
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
			if !b.External || b.ServiceEntry != "ext-se" {
				t.Fatalf("to-ext backend = %+v (와일드카드 SE 매치 실패)", b)
			}
		}
	}
}

// Initial-password guard: capabilities flags mustChangePassword while the
// account's password is "admin"; changing it clears the flag immediately
// (마운트 동기화를 기다리지 않는 인메모리 마킹).
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
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin"}, time.Hour),
	}
	caps := func() string {
		req := httptest.NewRequest("GET", "/api/capabilities", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if !strings.Contains(caps(), `"mustChangePassword":true`) {
		t.Fatalf("초기 비밀번호인데 플래그 없음: %s", caps())
	}
	req := httptest.NewRequest("POST", "/api/account/password",
		strings.NewReader(`{"currentPassword":"admin","newPassword":"newpass123"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("password change: %d %s", rec.Code, rec.Body.String())
	}
	// 마운트 디렉터리는 아직 옛 해시지만(1분 지연 시뮬레이션) 플래그는 즉시 풀려야 한다
	if strings.Contains(caps(), `"mustChangePassword":true`) {
		t.Fatal("변경 직후에도 플래그가 남아 있음")
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
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{Namespace: "ns1", Name: "clusters"}).Routes(mux)
	req := httptest.NewRequest("GET", "/api/clusters?status=true", nil)
	req.AddCookie(&http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "bob", Role: "viewer"}, time.Hour),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out []struct {
		Name      string
		Connected *bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Name != "local" || out[0].Connected == nil || !*out[0].Connected {
		t.Fatalf("local status wrong: %s", rec.Body.String())
	}
	if out[1].Connected == nil || *out[1].Connected {
		t.Fatalf("unreachable prod should be connected=false: %s", rec.Body.String())
	}
}

// 경로 시뮬레이터: VS는 선언 순서 첫 매치가 이기고(헤더 조건 포함), 매치 실패한
// 룰은 이유와 함께 남는다. HTTPRoute는 순서가 아니라 우선순위(exact > prefix)로 이긴다.
func TestRouteMatch(t *testing.T) {
	kube := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "dev"}},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "dev"},
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}},
	)
	istio := istiofake.NewSimpleClientset()
	if _, err := istio.NetworkingV1().VirtualServices("dev").Create(context.Background(),
		&istionet.VirtualService{ObjectMeta: metav1.ObjectMeta{Name: "shop-vs", Namespace: "dev"},
			Spec: istioapi.VirtualService{
				Hosts:    []string{"shop.example.com"},
				Gateways: []string{"istio-system/gw"},
				Http: []*istioapi.HTTPRoute{
					{ // 카나리: x-user=beta 헤더가 있을 때만 v2
						Match: []*istioapi.HTTPMatchRequest{{
							Headers: map[string]*istioapi.StringMatch{
								"x-user": {MatchType: &istioapi.StringMatch_Exact{Exact: "beta"}},
							},
						}},
						Route: []*istioapi.HTTPRouteDestination{
							{Destination: &istioapi.Destination{Host: "shop", Subset: "v2"}}},
					},
					{ // 기본 경로
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
	gw := gwfake.NewSimpleClientset(
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

	// 헤더 없음 → 1번 룰 불매치, 2번(기본) 룰이 이긴다
	out := get("host=shop.example.com&path=/cart")
	if !out.Matched || len(out.Destinations) != 1 || out.Destinations[0].Subset != "v1" {
		t.Fatalf("기본 경로가 v1이어야 함: %+v", out)
	}
	if out.Destinations[0].Endpoints != 1 || !out.Destinations[0].Exists {
		t.Fatalf("목적지 실체 확인 실패: %+v", out.Destinations[0])
	}
	if !out.Destinations[0].SubsetOK || out.Destinations[0].SubsetDR != "shop-dr" {
		t.Fatalf("v1 subset은 DR에 정의돼 있어야 함: %+v", out.Destinations[0])
	}
	var rejected *routeCandidate
	for i := range out.Candidates {
		if !out.Candidates[i].Matched {
			rejected = &out.Candidates[i]
		}
	}
	if rejected == nil || !strings.Contains(rejected.Reason, "x-user") {
		t.Fatalf("불매치 이유에 헤더가 나와야 함: %+v", out.Candidates)
	}

	// 헤더 있음 → 1번 룰(v2)이 이긴다. v2는 DR에 없으므로 subsetOk=false
	out = get("host=shop.example.com&path=/cart&header=x-user:beta")
	if !out.Matched || out.Destinations[0].Subset != "v2" {
		t.Fatalf("헤더가 있으면 v2로 가야 함: %+v", out)
	}
	if out.Destinations[0].SubsetOK {
		t.Fatal("v2 subset은 DR에 없으므로 subsetOk=false여야 함")
	}

	// 호스트 불일치 → 매치 없음
	if out = get("host=other.example.com&path=/"); out.Matched {
		t.Fatalf("다른 호스트는 매치되면 안 됨: %+v", out)
	}

	// HTTPRoute: 선언 순서상 prefix가 먼저지만 exact가 우선순위로 이긴다
	out = get("host=api.example.com&path=/api/users")
	if !out.Matched {
		t.Fatalf("HTTPRoute 매치 실패: %+v", out)
	}
	var winner *routeCandidate
	for i := range out.Candidates {
		if out.Candidates[i].Winner {
			winner = &out.Candidates[i]
		}
	}
	if winner == nil || winner.RuleIndex != 1 {
		t.Fatalf("exact 룰(index 1)이 이겨야 함: %+v", out.Candidates)
	}

	// PathPrefix는 세그먼트 경계 — /apifoo는 /api prefix에 걸리면 안 된다
	if out = get("host=api.example.com&path=/apifoo"); out.Matched {
		t.Fatalf("/apifoo는 /api prefix에 매치되면 안 됨: %+v", out)
	}
}


// 요청 테스터: 비활성 시 403, admin이 아니면 403, 켜면 argv를 셸 없이 구성하고
// curl 출력을 파싱해 Istio 문맥 힌트까지 붙인다.
func TestRequestTester(t *testing.T) {
	src := &stubSource{
		clients: &k8s.Clients{Kube: kubefake.NewClientset()},
		execOut: "HTTP/1.1 403 Forbidden\r\nserver: istio-envoy\r\nx-envoy-upstream-service-time: 3\r\n\r\nRBAC: access denied",
	}
	adminCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin"}, time.Hour),
	}
	body := `{"namespace":"dev","pod":"p1","method":"GET","url":"http://shop.dev:8080/api","headers":{"x-user":"beta"}}`
	send := func(mux *http.ServeMux, cookie *http.Cookie, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/requesttest", strings.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// 기본은 비활성 → 403
	off := newTestMux(t, src)
	if rec := send(off, adminCookie, body); rec.Code != http.StatusForbidden {
		t.Fatalf("비활성 상태에서 403이어야 함: %d %s", rec.Code, rec.Body.String())
	}

	// 활성화하되 editor는 거부
	on := http.NewServeMux()
	srv := NewServer(false, src, auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.EnableRequestTester()
	srv.Routes(on)
	editorCookie := &http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "bob", Role: "editor"}, time.Hour),
	}
	if rec := send(on, editorCookie, body); rec.Code != http.StatusForbidden {
		t.Fatalf("editor는 403이어야 함: %d", rec.Code)
	}

	// admin은 통과 — 응답 파싱 + 힌트
	rec := send(on, adminCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out reqTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != 403 || out.Headers["server"] != "istio-envoy" || out.Body != "RBAC: access denied" {
		t.Fatalf("curl 출력 파싱 실패: %+v", out)
	}
	if !strings.Contains(out.Hint, "AuthorizationPolicy") {
		t.Fatalf("403 RBAC 힌트가 없음: %q", out.Hint)
	}

	// argv는 셸을 거치지 않고 구성되고, URL은 "--" 뒤에 온다
	argv := strings.Join(src.execArgv, " ")
	if src.execArgv[0] != "curl" || !strings.Contains(argv, "-H x-user: beta") {
		t.Fatalf("argv 구성이 잘못됨: %v", src.execArgv)
	}
	if src.execArgv[len(src.execArgv)-2] != "--" {
		t.Fatalf("URL 앞에 -- 구분자가 있어야 함: %v", src.execArgv)
	}

	// 위험한 입력은 거부: 잘못된 스킴, 헤더 개행 주입, 미지원 메서드
	for _, bad := range []string{
		`{"namespace":"dev","pod":"p1","url":"file:///etc/passwd"}`,
		`{"namespace":"dev","pod":"p1","url":"http://x/","headers":{"a":"b\r\nInjected: 1"}}`,
		`{"namespace":"dev","pod":"p1","url":"http://x/","method":"TRACE"}`,
		`{"pod":"p1","url":"http://x/"}`,
	} {
		if rec := send(on, adminCookie, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("거부되어야 할 입력이 통과함 (%s): %d", bad, rec.Code)
		}
	}

	// curl이 없는 컨테이너 — 이 사실은 stderr가 아니라 exec 에러로 올라온다
	src.execOut, src.execErrS, src.execErr = "", "", errors.New(`OCI runtime exec failed: exec: "curl": executable file not found in $PATH`)
	rec = send(on, adminCookie, body)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !strings.Contains(out.Error, "curl이 없습니다") {
		t.Fatalf("curl 부재 안내가 없음: %+v", out)
	}
}


// 파드 목록: Istio 1.29+ 네이티브 사이드카(restartPolicy=Always인 initContainer)도
// 메시로 인식해야 한다. 일반 init 컨테이너는 exec 대상이 아니므로 목록에서 뺀다.
func TestPodList_NativeSidecar(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	kube := kubefake.NewClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "curl-1", Namespace: "demo"},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "istio-init"},                             // 진짜 init — 제외
					{Name: "istio-proxy", RestartPolicy: &always},    // 네이티브 사이드카
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
		auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	srv.EnableRequestTester()
	srv.Routes(mux)
	req := httptest.NewRequest("GET", "/api/pods?ns=demo", nil)
	req.AddCookie(&http.Cookie{
		Name:  "sealane_session",
		Value: testSessions.Sign(auth.Identity{Name: "admin", Role: "admin"}, time.Hour),
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
		t.Fatalf("네이티브 사이드카 파드를 메시로 인식해야 함: %+v", pods[0])
	}
	if strings.Join(pods[0].Containers, ",") != "istio-proxy,curl" {
		t.Fatalf("일반 init 컨테이너는 빠지고 사이드카는 남아야 함: %v", pods[0].Containers)
	}
	if pods[1].Mesh {
		t.Fatalf("사이드카 없는 파드는 메시가 아니어야 함: %+v", pods[1])
	}
}
