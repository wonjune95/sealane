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
	// 요청 테스터 전용: 파드 안에서 argv를 실행한다 (셸 없음).
	ExecInPod(ctx context.Context, ns, pod, container string, argv []string, maxBytes int) (string, string, error)
}

// Server holds injected dependencies and exposes the JSON API handlers.
type Server struct {
	dev        bool
	factory    ClientSource
	accounts   *auth.Store
	sessions   *auth.Sessions
	accountsCM AccountsCMRef // 비밀번호 변경이 patch할 계정 ConfigMap (빈 값이면 변경 불가)

	clustersSecret ClustersSecretRef        // 원격 클러스터 kubeconfig Secret (빈 값이면 local만)
	clusterMu      sync.Mutex               // clusterCache 보호
	clusterCache   map[string]ClientSource  // 클러스터명 → 팩토리 (지연 생성)

	auditMu  sync.Mutex
	auditLog []AuditEntry // newest first, capped at auditKeep

	// 이번 부팅에서 비밀번호를 바꾼 사용자 — 마운트 CM 동기화(~1분)를 기다리지 않고
	// 강제 변경 화면을 풀어준다. ponytail: 파드별 상태라 다중 복제본에선 다른 파드가
	// 최대 1분 늦게 풀린다; replicaCount>1이 기본이 되면 세션에 실어야 한다.
	pwChangedMu sync.Mutex
	pwChanged   map[string]bool

	requestTester bool // 실제 요청 테스터 (헬름 opt-in) — pods/exec 권한이 필요하다
}

func NewServer(dev bool, factory ClientSource, accounts *auth.Store, sessions *auth.Sessions, accountsCM AccountsCMRef, clustersSecret ClustersSecretRef) *Server {
	return &Server{dev: dev, factory: factory, accounts: accounts, sessions: sessions, accountsCM: accountsCM, clustersSecret: clustersSecret}
}

// EnableRequestTester turns on the in-pod request tester (admin 전용, 헬름 opt-in).
func (s *Server) EnableRequestTester() { s.requestTester = true }

// Routes registers the /api/* handlers on mux. Probes, /metrics and static assets
// are wired in main.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	// 세션만 확인하고 역할 게이트는 없다 — viewer도 본인 비밀번호는 바꿀 수 있다.
	mux.HandleFunc("POST /api/account/password", s.handlePasswordChange)
	// 계정 관리 — 핸들러 내부에서 admin 역할을 강제한다.
	mux.HandleFunc("GET /api/accounts", s.handleAccountsList)
	mux.HandleFunc("PUT /api/accounts/{name}", s.handleAccountUpsert)
	mux.HandleFunc("DELETE /api/accounts/{name}", s.handleAccountDelete)
	// 멀티클러스터 — 목록은 로그인 사용자 전체, 등록/삭제는 admin 전용.
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
	// 설정 기반 인그레스 트래픽 흐름도 (Gateway → Route → Service)
	mux.Handle("GET /api/flowmap", s.withAuth(http.HandlerFunc(s.handleFlowMap)))
	// 경로 시뮬레이터 — 설정만으로 요청이 어느 룰에 매치되는지 계산 (트래픽 없음)
	mux.Handle("GET /api/routematch", s.withAuth(http.HandlerFunc(s.handleRouteMatch)))
	// 실제 요청 테스터 (opt-in + admin) — 고른 파드 안에서 curl을 실행한다
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
			writeError(w, http.StatusForbidden, "Forbidden", "viewer 역할은 변경할 수 없습니다")
			return
		}
		// ?cluster= 로 대상 클러스터를 고른다 (빈 값/local = 파드가 있는 클러스터).
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
