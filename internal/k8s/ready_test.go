package k8s

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/rest"
)

func TestReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"34","gitVersion":"v1.34.0"}`))
	}))
	defer srv.Close()

	f, err := newFactory(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("newFactory: %v", err)
	}
	if err := f.Ready(); err != nil {
		t.Fatalf("Ready() against a healthy API server: %v", err)
	}

	down, err := newFactory(&rest.Config{Host: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("newFactory: %v", err)
	}
	if err := down.Ready(); err == nil {
		t.Fatal("Ready() against an unreachable API server must fail")
	}
}
