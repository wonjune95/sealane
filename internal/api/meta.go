package api

import (
	"context"
	"net/http"
	"strings"

	authzv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type capabilities struct {
	HTTPRouteInstalled      bool   `json:"httpRouteInstalled"`
	VirtualServiceInstalled bool   `json:"virtualServiceInstalled"`
	GatewayAPIVersion       string `json:"gatewayAPIVersion,omitempty"`
	IstioAPIVersion         string `json:"istioApiVersion,omitempty"`
	IstiodVersion           string `json:"istiodVersion,omitempty"` // control-plane version, e.g. "1.30.2"
	NamespaceListAllowed    bool   `json:"namespaceListAllowed"`
	DevMode                 bool   `json:"devMode"`
	User                    string `json:"user,omitempty"`
	Role                    string `json:"role,omitempty"`
	MustChangePassword      bool   `json:"mustChangePassword,omitempty"` // still the initial password (admin) — the UI forces a change
	RequestTester           bool   `json:"requestTester,omitempty"`      // whether the live request tester is enabled
}

// handleCapabilities reports CRD availability plus the session's identity/role.
// Authentication itself happens in withAuth (session cookie) — an unauthenticated
// request never reaches here.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	crd, err := factoryFrom(r.Context()).DetectCRDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	nsAllowed, err := s.canListNamespaces(r.Context())
	if apierrors.IsUnauthorized(err) {
		writeK8sError(w, err)
		return
	}
	// Other SSAR errors (transient, RBAC on SSAR itself) degrade to "not allowed".
	writeJSON(w, http.StatusOK, capabilities{
		HTTPRouteInstalled:      crd.HTTPRoute,
		VirtualServiceInstalled: crd.VirtualService,
		GatewayAPIVersion:       crd.GatewayAPIVersion,
		IstioAPIVersion:         crd.IstioAPIVersion,
		IstiodVersion:           factoryFrom(r.Context()).IstiodVersion(r.Context()),
		NamespaceListAllowed:    nsAllowed,
		DevMode:                 s.dev,
		User:                    identityFrom(r.Context()).User(),
		Role:                    identityFrom(r.Context()).Role,
		MustChangePassword:      s.mustChangePassword(identityFrom(r.Context()).Name),
		RequestTester:           s.requestTester,
	})
}

func (s *Server) canListNamespaces(ctx context.Context) (bool, error) {
	review := &authzv1.SelfSubjectAccessReview{
		Spec: authzv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authzv1.ResourceAttributes{Verb: "list", Resource: "namespaces"},
		},
	}
	res, err := clientFrom(ctx).Kube.AuthorizationV1().
		SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return res.Status.Allowed, nil
}

// handleAccess answers verb permissions from the session's app role:
// reads for everyone, writes for editor/admin. Kept as an endpoint so the UI's
// read-only affordances (disabled buttons, banners) work unchanged.
func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	if _, err := factoryFrom(r.Context()).ResolveType(r.PathValue("type")); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	canWrite := identityFrom(r.Context()).CanWrite()
	out := map[string]bool{}
	for _, v := range strings.Split(r.URL.Query().Get("verbs"), ",") {
		switch v = strings.TrimSpace(v); v {
		case "":
		case "get", "list", "watch":
			out[v] = true
		default:
			out[v] = canWrite
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleResourceTypes returns the curated resource registry with install state
// and discovered version (drives the UI category/kind navigation). It reads the
// same cached snapshot ResolveType uses, so the listing and the CRUD path never
// disagree about what is installed.
func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {
	cat, err := factoryFrom(r.Context()).CatalogCached()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cat)
}

// handleResourceSchema returns a kind's spec OpenAPI schema for auto-form rendering.
func (s *Server) handleResourceSchema(w http.ResponseWriter, r *http.Request) {
	raw, err := factoryFrom(r.Context()).SpecSchema(r.PathValue("type"))
	if err != nil {
		writeError(w, http.StatusNotFound, "NotFound", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// handleGateways lists Gateway API + Istio gateways for the route form dropdowns.
func (s *Server) handleGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, clientFrom(ctx).ListGateways(ctx, r.URL.Query().Get("ns")))
}

// handleServices lists services (+ ports) for backendRef selection and the
// "service does not exist" warning badge.
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	svcs, err := clientFrom(ctx).ListServices(ctx, r.URL.Query().Get("ns"))
	if err != nil {
		if apierrors.IsForbidden(err) {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, svcs)
}

// handleSubsets lists DestinationRule subsets for the VirtualService `subset`
// reference dropdown.
func (s *Server) handleSubsets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, clientFrom(ctx).ListSubsets(ctx, r.URL.Query().Get("ns")))
}

// handleNamespaces lists namespaces. When the user lacks list permission it
// returns an empty list (UI falls back to manual namespace entry) rather than erroring.
func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := clientFrom(ctx).Kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			writeJSON(w, http.StatusOK, []string{})
			return
		}
		writeK8sError(w, err)
		return
	}
	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	writeJSON(w, http.StatusOK, names)
}
