package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	istioapi "istio.io/api/networking/v1"
	securityapi "istio.io/api/security/v1"
	istiotype "istio.io/api/type/v1beta1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	securityv1 "istio.io/client-go/pkg/apis/security/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/wonjune95/sealane/internal/k8s"
)

func authzPolicy(ns, name string, sel map[string]string) *securityv1.AuthorizationPolicy {
	p := &securityv1.AuthorizationPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}
	if sel != nil {
		p.Spec = securityapi.AuthorizationPolicy{Selector: &istiotype.WorkloadSelector{MatchLabels: sel}}
	}
	return p
}

func targeted(ns, name string, one *istiotype.PolicyTargetReference, many []*istiotype.PolicyTargetReference) *securityv1.AuthorizationPolicy {
	p := authzPolicy(ns, name, nil)
	p.Spec.TargetRef, p.Spec.TargetRefs = one, many
	return p
}

func TestServiceMesh(t *testing.T) {
	t.Setenv("ISTIO_NAMESPACE", "istio-system")
	kube := kubefake.NewSimpleClientset(svc("apps", "shop", map[string]string{"app": "shop"}))
	port := gwapi.PortNumber(80)
	edge := gwapi.Namespace("edge")
	hr := &gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop-route"},
		Spec: gwapi.HTTPRouteSpec{
			CommonRouteSpec: gwapi.CommonRouteSpec{ParentRefs: []gwapi.ParentReference{{Namespace: &edge, Name: "gw"}}},
			Rules: []gwapi.HTTPRouteRule{{BackendRefs: []gwapi.HTTPBackendRef{
				{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "shop", Port: &port}}},
			}}}}}
	istio := istiofake.NewSimpleClientset(
		vs("apps", "shop-vs", []string{"shop"}, nil, &istioapi.Destination{Host: "shop"}),
		vs("apps", "caller", []string{"other"}, nil, &istioapi.Destination{Host: "shop"}),
		vs("apps", "elsewhere", []string{"other"}, nil, &istioapi.Destination{Host: "other"}),
		vs("edge", "catch-all", []string{"*"}, []string{"edge/public"}, &istioapi.Destination{Host: "other.apps.svc.cluster.local"}),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop-dr"}, Spec: istioapi.DestinationRule{Host: "shop.apps.svc.cluster.local"}},
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "apps-wide"}, Spec: istioapi.DestinationRule{Host: "*.apps.svc.cluster.local"}},
		authzPolicy("apps", "allow", map[string]string{"app": "shop"}),
		authzPolicy("apps", "ns-wide", nil),
		authzPolicy("apps", "unrelated", map[string]string{"app": "cart"}),
		targeted("apps", "by-target", &istiotype.PolicyTargetReference{Kind: "Service", Name: "shop"}, nil),
		targeted("apps", "other-target", &istiotype.PolicyTargetReference{Kind: "Service", Name: "cart"}, nil),
		targeted("istio-system", "ingress-only", nil, []*istiotype.PolicyTargetReference{{Group: "gateway.networking.k8s.io", Kind: "Gateway", Name: "ingress"}}),
		&securityv1.PeerAuthentication{ObjectMeta: metav1.ObjectMeta{Namespace: "istio-system", Name: "default"}},
	)
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset(hr)}
	mux := newTestMux(t, &stubSource{clients: c})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/services/apps/shop/mesh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var m serviceMesh
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	vsMatch := map[string]string{}
	for _, v := range m.VirtualServices {
		vsMatch[v.Name] = v.Match
	}
	if len(vsMatch) != 2 || vsMatch["shop-vs"] != "host" || vsMatch["caller"] != "destination" {
		t.Errorf("virtualServices: %+v", m.VirtualServices)
	}
	if len(m.DestinationRules) != 2 || m.DestinationRules[0].Name != "apps-wide" || m.DestinationRules[1].Name != "shop-dr" {
		t.Errorf("destinationRules: %+v", m.DestinationRules)
	}
	if len(m.HTTPRoutes) != 1 || m.HTTPRoutes[0].Name != "shop-route" {
		t.Errorf("httpRoutes: %+v", m.HTTPRoutes)
	}
	wantGW := ref{TypeID: typeGateway, Kind: "Gateway", Namespace: "edge", Name: "gw"}
	if len(m.Gateways) != 1 || m.Gateways[0] != wantGW {
		t.Errorf("gateways: %+v", m.Gateways)
	}
	scopes := map[string]string{}
	for _, p := range m.Policies {
		scopes[p.Namespace+"/"+p.Name] = p.Scope
	}
	want := map[string]string{"apps/allow": "workload", "apps/by-target": "workload", "apps/ns-wide": "namespace", "istio-system/default": "mesh"}
	if len(scopes) != len(want) {
		t.Errorf("policies: %+v", m.Policies)
	}
	for k, v := range want {
		if scopes[k] != v {
			t.Errorf("policy %s: want scope %q, got %q", k, v, scopes[k])
		}
	}
	if m.Endpoints != 0 || m.Ports == nil || m.Findings == nil {
		t.Errorf("endpoints/ports/findings: %d %v %v", m.Endpoints, m.Ports, m.Findings)
	}
	// shop has no Endpoints object: the findings concerning it are backend-no-endpoints, none for "other"
	for _, f := range m.Findings {
		if f.Service != "apps/shop" {
			t.Errorf("finding for another Service: %+v", f)
		}
	}
	if len(m.Findings) == 0 {
		t.Error("want the backend-no-endpoints findings for apps/shop")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/services/apps/ghost/mesh", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing Service: want 404, got %d", rec.Code)
	}
}

func TestServiceMeshIstioGatewaysAndEmptySlices(t *testing.T) {
	t.Setenv("ISTIO_NAMESPACE", "istio-system")
	istio := istiofake.NewSimpleClientset(
		vs("apps", "a", []string{"shop"}, []string{"mesh", "gw.edge.svc.cluster.local", "edge/gw", "local"}, &istioapi.Destination{Host: "shop"}),
	)
	c := &k8s.Clients{Kube: kubefake.NewSimpleClientset(svc("apps", "shop", nil)), Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	mux := newTestMux(t, &stubSource{clients: c})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/services/apps/shop/mesh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var m serviceMesh
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	want := []ref{
		{TypeID: typeIstioGW, Kind: "Gateway", Namespace: "apps", Name: "local"},
		{TypeID: typeIstioGW, Kind: "Gateway", Namespace: "edge", Name: "gw"},
	}
	if len(m.Gateways) != 2 || m.Gateways[0] != want[0] || m.Gateways[1] != want[1] {
		t.Errorf("gateways: %+v", m.Gateways)
	}
	if m.DestinationRules == nil || m.HTTPRoutes == nil || m.Policies == nil {
		t.Errorf("slices must be non-null: %s", rec.Body.String())
	}
}

// Findings on the resources listed on the page count too, not only those tagged with the Service:
// a Gateway bound by the Service's VS whose selector matches no pod is shown on the Service page.
func TestServiceMeshFindingsOnListedResources(t *testing.T) {
	t.Setenv("ISTIO_NAMESPACE", "istio-system")
	istio := istiofake.NewSimpleClientset(
		vs("apps", "shop-vs", []string{"shop"}, []string{"edge/public"}, &istioapi.Destination{Host: "shop"}))
	// The fakes mis-bucket the Gateway kind when seeding — insert via typed Create (as in analyze_test.go).
	for _, name := range []string{"public", "unrelated"} {
		if _, err := istio.NetworkingV1().Gateways("edge").Create(context.Background(), &istionet.Gateway{
			ObjectMeta: metav1.ObjectMeta{Namespace: "edge", Name: name},
			Spec:       istioapi.Gateway{Selector: map[string]string{"istio": "nowhere"}}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	c := &k8s.Clients{Kube: kubefake.NewSimpleClientset(svc("apps", "shop", nil)), Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	mux := newTestMux(t, &stubSource{clients: c})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/services/apps/shop/mesh", nil))
	var m serviceMesh
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range m.Findings {
		if f.Code == "gateway-selector-no-pods" {
			got = append(got, f.Resource.Name)
		}
	}
	if len(got) != 1 || got[0] != "public" {
		t.Fatalf("want the gateway-selector-no-pods finding for edge/public only, got %v in %+v", got, m.Findings)
	}
}
