package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

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
}

// Server holds injected dependencies and exposes the JSON API handlers.
type Server struct {
	dev        bool
	factory    ClientSource
	accounts   *auth.Store
	sessions   *auth.Sessions
	accountsCM AccountsCMRef // accounts ConfigMap that password changes patch (empty = changes disabled)

	clustersSecret ClustersSecretRef        // remote-cluster kubeconfig Secret (empty = local only)
	clusterMu      sync.Mutex               // guards clusterCache
	clusterCache   map[string]ClientSource  // cluster name → factory (created lazily)

	auditMu  sync.Mutex
	auditLog []AuditEntry // newest first, capped at auditKeep

	// Users who changed their password since this boot — releases the forced change
	// screen without waiting for the mounted CM sync (~1 min). ponytail: per-pod state, so
	// with multiple replicas other pods release it up to 1 min later; move it into the
	// session if replicaCount>1 becomes the default.
	pwChangedMu sync.Mutex
	pwChanged   map[string]bool

	requestTester bool // live request tester (Helm opt-in) — needs pods/exec
}

func NewServer(dev bool, factory ClientSource, accounts *auth.Store, sessions *auth.Sessions, accountsCM AccountsCMRef, clustersSecret ClustersSecretRef) *Server {
	return &Server{dev: dev, factory: factory, accounts: accounts, sessions: sessions, accountsCM: accountsCM, clustersSecret: clustersSecret}
}

// EnableRequestTester turns on the in-pod request tester (admin only, Helm opt-in).
func (s *Server) EnableRequestTester() { s.requestTester = true }

// Routes registers the /api/* handlers on mux. Probes, /metrics and static assets
// are wired in main.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
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
	mux.Handle("GET /api/subsets", s.withAuth(http.HandlerFunc(s.handleSubsets)))
	mux.Handle("GET /api/audit", s.withAuth(http.HandlerFunc(s.handleAudit)))
	// Config-based ingress traffic flow map (Gateway → Route → Service)
	mux.Handle("GET /api/flowmap", s.withAuth(http.HandlerFunc(s.handleFlowMap)))
	// Route simulator — computes from config alone which rule a request matches (no traffic)
	mux.Handle("GET /api/routematch", s.withAuth(http.HandlerFunc(s.handleRouteMatch)))
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

// sessionIdentity resolves the request's identity: fixed dev identity in dev
// mode, otherwise the parsed session cookie.
func (s *Server) sessionIdentity(r *http.Request) (auth.Identity, error) {
	if s.dev {
		return auth.Identity{Name: "dev", Role: "admin"}, nil
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
		id, err := s.sessionIdentity(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
			return
		}
		if r.Method != http.MethodGet && !id.CanWrite() {
			writeError(w, http.StatusForbidden, "Forbidden", "viewer role cannot make changes")
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
