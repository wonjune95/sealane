// Command server runs Sealane — a routing console for Istio and Gateway API: a
// single binary serving the embedded React SPA and a JSON API over the Kubernetes API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wonjune95/sealane/internal/api"
	"github.com/wonjune95/sealane/internal/assets"
	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
	"github.com/wonjune95/sealane/internal/observability"
)

var version = "dev" // set with -ldflags "-X main.version=..."

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.Bool("dev", false, "dev mode: skip OIDC verification, use local kubeconfig")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (dev only; default loading rules if empty)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("sealane " + version)
		return
	}

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
	api.AppVersion = version
	observability.SetBuildInfo(version)
	// No SESSION_SECRET env (install.yaml, GitOps): keep a generated key in the
	// SESSION_SECRET_NAME Secret so restarts and replicas share it.
	sessionSecret := os.Getenv("SESSION_SECRET")
	if name := os.Getenv("SESSION_SECRET_NAME"); sessionSecret == "" && name != "" && !*dev {
		// A per-boot key here would differ between replicas (random sign-outs, failed SSO sign-ins), so
		// keep trying for a while and then stop: the restart and this log make the problem visible.
		var err error
		for attempt := 1; ; attempt++ {
			var c *k8s.Clients
			if c, err = factory.Base(); err == nil {
				sessionSecret, err = api.LoadSessionSecret(context.Background(), c.Kube, os.Getenv("POD_NAMESPACE"), name)
			}
			if err == nil || attempt == 6 {
				break
			}
			slog.Warn("session secret: retrying", "secret", name, "attempt", attempt, "err", err)
			time.Sleep(5 * time.Second)
		}
		if err != nil {
			slog.Error("session secret: cannot read or store the session key; check the Role allows get/update/create on this Secret",
				"secret", name, "err", err)
			os.Exit(1)
		}
	}
	store := auth.NewStore(accountsDir)
	store.SourceErrors = observability.AccountReadErrors.Inc
	srv := api.NewServer(*dev, factory, store, auth.NewSessions(sessionSecret),
		api.AccountsCMRef{Namespace: os.Getenv("POD_NAMESPACE"), Name: os.Getenv("ACCOUNTS_CONFIGMAP")},
		// Multi-cluster: Secret holding remote-cluster kubeconfigs (unset = local only)
		api.ClustersSecretRef{Namespace: os.Getenv("POD_NAMESPACE"), Name: os.Getenv("CLUSTERS_SECRET")})
	// The live request tester needs pods/exec, so it is off by default — enable it via Helm.
	if os.Getenv("REQUEST_TESTER") == "true" {
		srv.EnableRequestTester()
		slog.Info("request tester enabled (admin only, uses pods/exec)")
	}
	// Behind TLS-terminating proxies that do not send X-Forwarded-Proto, mark cookies Secure explicitly.
	api.SecureCookies = os.Getenv("SECURE_COOKIES") == "true"
	// Single sign-on (optional): OIDC_* env from the chart. Discovery is lazy, so a
	// misconfigured issuer shows up on first use, not at boot.
	switch cfg, err := auth.OIDCConfigFromEnv(); {
	case err != nil:
		slog.Error("oidc config invalid", "err", err)
		os.Exit(1)
	case cfg != nil:
		srv.SetOIDC(auth.NewOIDC(*cfg))
		api.SecureCookies = api.SecureCookies || strings.HasPrefix(cfg.RedirectURL, "https://")
		slog.Info("oidc enabled", "issuer", cfg.IssuerURL, "displayName", cfg.DisplayName)
	}
	srv.HideLocalLogin(os.Getenv("LOCAL_LOGIN") == "hidden")
	if os.Getenv("IMPERSONATION") == "true" {
		srv.SetImpersonation(true)
		slog.Info("impersonation enabled: resource writes run as sealane:<role>")
	}
	// Change history in ConfigMaps next to Sealane (shared by replicas); unset = in memory only.
	if name, ns := os.Getenv("HISTORY_CONFIGMAP"), os.Getenv("POD_NAMESPACE"); name != "" && ns != "" && !*dev {
		srv.SetHistory(api.HistoryRef{Namespace: ns, Name: name})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		srv.MigrateLegacyHistoryObjects(ctx)
		cancel()
	}
	// No admin account yet: create "admin" with a random password kept in the INITIAL_ADMIN_SECRET Secret.
	srv.SetInitialAdminSecret(os.Getenv("INITIAL_ADMIN_SECRET"))
	if !srv.EnsureInitialAdmin(context.Background()) {
		go srv.KeepEnsuringInitialAdmin(context.Background(), 10*time.Second) // retried in the background
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if err := factory.Ready(); err != nil {
			slog.Warn("readiness check failed", "err", err)
			writeText(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		writeText(w, http.StatusOK, "ready")
	})
	mux.Handle("GET /metrics", observability.MetricsHandler())
	srv.Routes(mux)
	mux.Handle("/", assets.Handler())

	// Browsers' cross-site POST/PUT/DELETE are refused (CSRF), using Sec-Fetch-Site or Origin vs Host.
	// TRUSTED_ORIGINS lists extra origins, for a proxy that rewrites the Host header.
	csrf := http.NewCrossOriginProtection()
	for _, o := range strings.Split(os.Getenv("TRUSTED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			if err := csrf.AddTrustedOrigin(o); err != nil {
				slog.Error("TRUSTED_ORIGINS invalid", "origin", o, "err", err)
				os.Exit(1)
			}
		}
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(observability.Instrument(securityHeaders(csrf.Handler(mux)))),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", *addr, "dev", *dev, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	<-ctx.Done()
	stop() // a second Ctrl-C during the delay/drain exits immediately

	delay := 5 * time.Second
	if v := os.Getenv("SHUTDOWN_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			delay = d
		} else {
			slog.Warn("ignoring invalid SHUTDOWN_DELAY", "value", v)
		}
	}
	slog.Info("shutting down", "delay", delay)
	time.Sleep(delay) // ponytail: fixed sleep so endpoint removal reaches ingress/sidecars before the listener closes; readiness flip if 5s proves short
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
		if api.SecureCookies || r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
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
