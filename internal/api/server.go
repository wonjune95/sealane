package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
)

// ClientSource is the k8s.ClientFactory surface the handlers depend on,
// abstracted so tests can substitute fakes.
type ClientSource interface {
	Base() (*k8s.Clients, error)
	ResolveType(typeID string) (k8s.ResolvedType, error)
	CatalogCached() ([]k8s.ResolvedType, error)
	DetectCRDs() (k8s.CRDInfo, error)
	SpecSchema(typeID string) (json.RawMessage, error)
	IstiodVersion(ctx context.Context) string
	// Request tester only: runs argv inside a pod (no shell).
	ExecInPod(ctx context.Context, ns, pod, container string, argv []string, maxBytes int) (string, string, error)
	// Impersonation mode only: clients acting as another identity.
	Impersonating(as rest.ImpersonationConfig) (*k8s.Clients, error)
}

// Server holds injected dependencies and exposes the JSON API handlers.
type Server struct {
	dev        bool
	factory    ClientSource
	accounts   *auth.Store
	sessions   *auth.Sessions
	accountsCM AccountsCMRef // accounts ConfigMap that password changes patch (empty = changes disabled)

	clustersSecret ClustersSecretRef       // remote-cluster kubeconfig Secret (empty = local only)
	clusterMu      sync.Mutex              // guards clusterCache
	clusterCache   map[string]ClientSource // cluster name → factory (created lazily)

	auditMu  sync.Mutex
	auditLog []AuditEntry // newest first, capped at auditKeep
	history  HistoryRef   // in-cluster change history (empty Name = memory only)

	adminPending       atomic.Bool // the initial admin could not be made yet (readiness)
	initialAdminSecret string      // Secret that receives a generated initial admin password

	requestTester bool // live request tester (Helm opt-in) — needs pods/exec
	impersonate   bool // writes run as the user's role identity (impersonation.go)

	oidc *auth.OIDC // nil = local accounts only

	localHidden bool // the login page hides the password form (the API still accepts local logins)

	loginThrottle *throttle
}

func NewServer(dev bool, factory ClientSource, accounts *auth.Store, sessions *auth.Sessions, accountsCM AccountsCMRef, clustersSecret ClustersSecretRef) *Server {
	s := &Server{dev: dev, factory: factory, accounts: accounts, sessions: sessions, accountsCM: accountsCM, clustersSecret: clustersSecret, loginThrottle: newThrottle()}
	if accountsCM.Name != "" && !dev {
		// Read accounts through the API (cached briefly) rather than the mounted directory, which
		// kubelet refreshes only about once a minute: a deleted, demoted or re-passworded account's
		// sessions then end within seconds on every replica.
		accounts.SetSource(func() (map[string]string, error) {
			client, err := factory.Base()
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cm, err := client.Kube.CoreV1().ConfigMaps(accountsCM.Namespace).Get(ctx, accountsCM.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return map[string]string{}, nil
			}
			if err != nil {
				return nil, err
			}
			if cm.Data == nil {
				return map[string]string{}, nil
			}
			return cm.Data, nil
		})
	}
	return s
}

// EnableRequestTester turns on the in-pod request tester (admin only, Helm opt-in).
func (s *Server) EnableRequestTester() { s.requestTester = true }

// Routes registers the /api/* handlers on mux. Probes, /metrics and static assets
// are wired in main.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/methods", s.handleAuthMethods)
	mux.HandleFunc("GET /api/auth/oidc/login", s.handleOIDCLogin)
	mux.HandleFunc("GET /api/auth/oidc/callback", s.handleOIDCCallback)
	// Session check only, no role gate — viewers can change their own password too.
	mux.HandleFunc("POST /api/account/password", s.handlePasswordChange)
	// Account management — the handlers enforce the admin role.
	mux.HandleFunc("GET /api/accounts", s.handleAccountsList)
	mux.HandleFunc("PUT /api/accounts/{name}", s.handleAccountUpsert)
	mux.HandleFunc("DELETE /api/accounts/{name}", s.handleAccountDelete)
	// Multi-cluster — any logged-in user can list; register/delete is admin only.
	mux.HandleFunc("GET /api/clusters", s.handleClustersList)
	mux.HandleFunc("PUT /api/clusters/{name}", s.handleClusterUpsert)
	mux.HandleFunc("DELETE /api/clusters/{name}", s.handleClusterDelete)
	mux.Handle("GET /api/capabilities", s.withAuth(http.HandlerFunc(s.handleCapabilities)))
	mux.Handle("GET /api/resourceTypes", s.withAuth(http.HandlerFunc(s.handleResourceTypes)))
	mux.Handle("GET /api/resourceTypes/{type}/schema", s.withAuth(http.HandlerFunc(s.handleResourceSchema)))
	mux.Handle("GET /api/access/{type}/{ns}/{name}", s.withAuth(http.HandlerFunc(s.handleAccess)))
	mux.Handle("GET /api/namespaces", s.withAuth(http.HandlerFunc(s.handleNamespaces)))
	mux.Handle("GET /api/gateways", s.withAuth(http.HandlerFunc(s.handleGateways)))
	mux.Handle("GET /api/services", s.withAuth(http.HandlerFunc(s.handleServices)))
	mux.Handle("GET /api/services/{ns}/{name}/mesh", s.withAuth(http.HandlerFunc(s.handleServiceMesh)))
	mux.Handle("GET /api/subsets", s.withAuth(http.HandlerFunc(s.handleSubsets)))
	mux.Handle("GET /api/audit", s.withAuth(http.HandlerFunc(s.handleAudit)))
	mux.Handle("GET /api/history", s.withAuth(http.HandlerFunc(s.handleHistory)))
	mux.Handle("GET /api/history/{id}", s.withAuth(http.HandlerFunc(s.handleRevision)))
	// Config-based ingress traffic flow map (Gateway → Route → Service)
	mux.Handle("GET /api/flowmap", s.withAuth(http.HandlerFunc(s.handleFlowMap)))
	// Route simulator — computes from config alone which rule a request matches (no traffic)
	mux.Handle("GET /api/routematch", s.withAuth(http.HandlerFunc(s.handleRouteMatch)))
	// Mesh analysis — cross-resource checks from config alone (plus pods when readable)
	mux.Handle("GET /api/analyze", s.withAuth(http.HandlerFunc(s.handleAnalyze)))
	// Live request tester (opt-in + admin) — runs curl inside the chosen pod
	mux.Handle("GET /api/pods", s.withAuth(http.HandlerFunc(s.handlePods)))
	mux.Handle("POST /api/requesttest", s.withAuth(http.HandlerFunc(s.handleRequestTest)))
	// Unmatched /api/* returns JSON 404 (not the SPA index.html fallback).
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NotFound", "no such API endpoint: "+r.URL.Path)
	})
	// Generic resource engine (any registered kind, typeID = resource.group).
	mux.Handle("GET /api/resources/{type}", s.withAuth(http.HandlerFunc(s.handleListResources)))
	mux.Handle("GET /api/resources/{type}/{ns}/{name}", s.withAuth(http.HandlerFunc(s.handleGetResource)))
	mux.Handle("POST /api/resources/{type}", s.withAuth(http.HandlerFunc(s.handleCreateResource)))
	mux.Handle("PUT /api/resources/{type}/{ns}/{name}", s.withAuth(http.HandlerFunc(s.handleUpdateResource)))
	mux.Handle("DELETE /api/resources/{type}/{ns}/{name}", s.withAuth(http.HandlerFunc(s.handleDeleteResource)))
}

// session authenticates a request and writes the error response when it fails. A local session is
// only as good as the account behind it: deleted, demoted, or signed in before the latest password
// change → signed out. An account still on the old fixed "admin" password may do nothing but change
// it (passwordChange=true marks the requests that stay allowed).
func (s *Server) session(w http.ResponseWriter, r *http.Request, passwordChange bool) (auth.Identity, bool) {
	id, err := s.sessionIdentity(r)
	if err != nil {
		// Only Sealane's own session errors are 401 (with a code); the UI shows the sign-in page for them.
		writeCoded(w, http.StatusUnauthorized, "Unauthorized", "session-required", err.Error())
		return id, false
	}
	if s.dev || id.Provider != auth.ProviderLocal {
		return id, true
	}
	role, pv, ok := s.accounts.Lookup(id.Name)
	if !ok || role != id.Role || pv != id.PV {
		clearCookie(w, r, sessionCookie, "/")
		writeCoded(w, http.StatusUnauthorized, "Unauthorized", "session-revoked", "your session ended because the account changed; sign in again")
		return id, false
	}
	if !passwordChange && s.mustChangePassword(id.Name) {
		writeCoded(w, http.StatusForbidden, "Forbidden", "password-change-required", "change the initial password first")
		return id, false
	}
	return id, true
}

// sessionIdentity resolves the request's identity: fixed dev identity in dev
// mode, otherwise the parsed session cookie.
func (s *Server) sessionIdentity(r *http.Request) (auth.Identity, error) {
	if s.dev {
		return auth.Identity{Name: "dev", Role: "admin", Provider: auth.ProviderLocal}, nil
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return auth.Identity{}, auth.ErrSessionInvalid
	}
	return s.sessions.Parse(c.Value)
}

// withAuth authenticates the session cookie (app-local auth), enforces
// the role on mutating methods, and injects the shared SA-backed client plus the
// user's identity into the context. Dev mode skips the session entirely.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.session(w, r, r.URL.Path == "/api/capabilities")
		if !ok {
			return
		}
		if r.Method != http.MethodGet && !id.CanWrite() {
			writeCoded(w, http.StatusForbidden, "Forbidden", "viewer-readonly", "viewer role cannot make changes")
			return
		}
		// ?cluster= selects the target cluster (empty/local = the cluster the pod runs in).
		cluster := r.URL.Query().Get("cluster")
		factory, err := s.clusterSource(r.Context(), cluster)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}
		client, err := factory.Base()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", err.Error())
			return
		}
		ctx := withIdentity(withFactory(withClient(r.Context(), client), factory), id)
		if cluster != "" && cluster != "local" {
			ctx = withCluster(ctx, cluster)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
