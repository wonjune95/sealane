// Command server runs Sealane — a routing console for Istio and Gateway API: a
// single binary serving the embedded React SPA and a JSON API over the Kubernetes API.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wonjune95/sealane/internal/api"
	"github.com/wonjune95/sealane/internal/assets"
	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
	"github.com/wonjune95/sealane/internal/observability"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.Bool("dev", false, "dev mode: skip OIDC verification, use local kubeconfig")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (dev only; default loading rules if empty)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// dev safety guard: --dev bypasses login and uses a local kubeconfig, so it must
	// never run in a cluster. KUBERNETES_SERVICE_HOST is injected into every pod;
	// its presence means we are in-cluster — refuse to start to prevent auth bypass.
	if *dev && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		slog.Error("--dev refused: in-cluster environment detected (would bypass login)")
		os.Exit(1)
	}

	factory, err := k8s.NewClientFactory(*dev, *kubeconfig)
	if err != nil {
		slog.Error("kube client init failed", "err", err)
		os.Exit(1)
	}
	// App-local accounts: a mounted ConfigMap dir (one file per user,
	// "role:bcryptHash"). Sessions are HMAC cookies; empty SESSION_SECRET means a
	// per-boot random key (restart = re-login).
	accountsDir := os.Getenv("ACCOUNTS_DIR")
	if accountsDir == "" {
		accountsDir = "/etc/sealane/accounts"
	}
	if !*dev {
		if _, err := os.Stat(accountsDir); err != nil {
			slog.Warn("accounts dir not readable — no one can log in until the ConfigMap is mounted", "dir", accountsDir, "err", err)
		}
	}
	srv := api.NewServer(*dev, factory, auth.NewStore(accountsDir), auth.NewSessions(os.Getenv("SESSION_SECRET")),
		api.AccountsCMRef{Namespace: os.Getenv("POD_NAMESPACE"), Name: os.Getenv("ACCOUNTS_CONFIGMAP")},
		// 멀티클러스터: 원격 클러스터 kubeconfig를 담는 Secret (미설정이면 local만)
		api.ClustersSecretRef{Namespace: os.Getenv("POD_NAMESPACE"), Name: os.Getenv("CLUSTERS_SECRET")})
	// 실제 요청 테스터는 pods/exec 권한이 필요해 기본 비활성 — 헬름에서 켠다.
	if os.Getenv("REQUEST_TESTER") == "true" {
		srv.EnableRequestTester()
		slog.Info("request tester enabled (admin 전용, pods/exec 사용)")
	}
	// admin 계정이 없으면 초기 계정 admin/admin을 만든다.
	srv.EnsureInitialAdmin(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		// P2: verify API-server reachability + CRD discovery before reporting ready.
		writeText(w, http.StatusOK, "ready")
	})
	mux.Handle("GET /metrics", observability.MetricsHandler())
	srv.Routes(mux)
	mux.Handle("/", assets.Handler())

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(observability.Instrument(securityHeaders(mux))),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", *addr, "dev", *dev)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	slog.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// securityHeaders sets baseline hardening headers. The SPA is fully self-contained
// (no external scripts/styles/fonts), so a same-origin CSP is safe. CodeMirror and
// React inject inline styles, hence style-src 'unsafe-inline'. frame-ancestors 'none'
// + X-Frame-Options block clickjacking on the token-entry screen.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// logRequests is a minimal structured access log.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "dur_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
