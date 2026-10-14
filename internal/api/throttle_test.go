package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
)

func TestThrottleBlocksAfterMaxFailures(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	th := newThrottle()
	th.now = func() time.Time { return now }
	for i := 0; i < throttleMax-1; i++ {
		th.fail("u:bob")
	}
	if d := th.blocked("u:bob"); d != 0 {
		t.Fatalf("4 failures must not block, got %v", d)
	}
	th.fail("u:bob")
	if d := th.blocked("u:bob"); d <= 0 || d > throttleBlock {
		t.Fatalf("5th failure blocks for up to %v, got %v", throttleBlock, d)
	}
	now = now.Add(throttleBlock + time.Second)
	if d := th.blocked("u:bob"); d != 0 {
		t.Fatalf("block expired, got %v", d)
	}
	th.fail("u:bob") // still inside the window: 6 failures → blocked again at once
	if th.blocked("u:bob") == 0 {
		t.Fatal("a failure after the block, still inside the window, blocks again")
	}
	now = now.Add(throttleWindow + time.Second)
	if th.blocked("u:bob") != 0 {
		t.Fatal("window passed, failures forgotten")
	}
	th.fail("u:bob")
	th.reset("u:bob")
	if len(th.fails) != 0 {
		t.Fatal("reset clears the key")
	}
}

func TestLoginReturns429WhenThrottled(t *testing.T) {
	dir := t.TempDir()
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err := os.WriteFile(filepath.Join(dir, "bob"), []byte("viewer:"+string(hash)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewServer(false, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewClientset()}},
		auth.NewStore(dir), testSessions, AccountsCMRef{}, ClustersSecretRef{}).Routes(mux)
	post := func(pw string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"bob","password":"`+pw+`"}`)))
		return rec
	}
	for i := 0; i < throttleMax; i++ {
		if rec := post("wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	rec := post("wrong")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d, Retry-After = %q; want 429 with header", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := post("s3cret"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("right password while blocked: status = %d, want 429", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "10.1.2.3:4444"
	if clientIP(r) != "10.1.2.3" {
		t.Fatal(clientIP(r))
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if clientIP(r) != "203.0.113.9" {
		t.Fatal(clientIP(r))
	}
}
