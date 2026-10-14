package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	istioapi "istio.io/api/networking/v1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/wonjune95/sealane/internal/k8s"
)

func svc(ns, name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}, Spec: corev1.ServiceSpec{Selector: selector}}
}

func pod(ns, name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels}}
}

func vs(ns, name string, hosts, gateways []string, dests ...*istioapi.Destination) *istionet.VirtualService {
	var routes []*istioapi.HTTPRouteDestination
	for _, d := range dests {
		routes = append(routes, &istioapi.HTTPRouteDestination{Destination: d})
	}
	return &istionet.VirtualService{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       istioapi.VirtualService{Hosts: hosts, Gateways: gateways, Http: []*istioapi.HTTPRoute{{Route: routes}}},
	}
}

func codes(a analysis) map[string]int {
	m := map[string]int{}
	for _, f := range a.Findings {
		m[f.Code]++
	}
	return m
}

func TestAnalyzeVirtualServiceDestinations(t *testing.T) {
	kube := kubefake.NewSimpleClientset(
		svc("apps", "shop", map[string]string{"app": "shop"}),
		pod("apps", "shop-1", map[string]string{"app": "shop", "version": "v1"}),
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"},
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}},
	)
	istio := istiofake.NewSimpleClientset(
		vs("apps", "ok", []string{"shop"}, nil, &istioapi.Destination{Host: "shop", Subset: "v1"}),
		vs("apps", "missing", []string{"nope"}, nil, &istioapi.Destination{Host: "nope.apps.svc.cluster.local"}),
		vs("apps", "badsubset", []string{"shop"}, nil, &istioapi.Destination{Host: "shop", Subset: "v9"}),
		vs("apps", "badgw", []string{"shop.example.com"}, []string{"edge/absent"}, &istioapi.Destination{Host: "shop"}),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"},
			Spec: istioapi.DestinationRule{Host: "shop", Subsets: []*istioapi.Subset{
				{Name: "v1", Labels: map[string]string{"version": "v1"}},
				{Name: "v2", Labels: map[string]string{"version": "v2"}}, // no pods
			}}},
	)
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	a := analyzeMesh(context.Background(), c, "")
	got := codes(a)
	for code, n := range map[string]int{"vs-host-missing": 1, "vs-subset-undefined": 1, "vs-gateway-missing": 1, "dr-subset-no-pods": 1} {
		if got[code] != n {
			t.Errorf("%s: want %d, got %d (all: %v)", code, n, got[code], got)
		}
	}
	if got["backend-no-endpoints"] != 0 {
		t.Errorf("shop has an endpoint; got %v", got)
	}
	if len(a.Skipped) != 0 {
		t.Errorf("nothing should be skipped: %v", a.Skipped)
	}
	for _, f := range a.Findings {
		if f.Code == "vs-host-missing" && f.Service != "apps/nope" {
			t.Errorf("service ref: got %q", f.Service)
		}
	}
}

func TestAnalyzeHostConflictAndNamespaceScope(t *testing.T) {
	kube := kubefake.NewSimpleClientset(svc("apps", "shop", nil))
	gw := &istionet.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "edge", Name: "gw"},
		Spec: istioapi.Gateway{Selector: map[string]string{"istio": "ingressgateway"}}}
	istio := istiofake.NewSimpleClientset(
		vs("apps", "a", []string{"shop.example.com"}, []string{"edge/gw"}, &istioapi.Destination{Host: "shop"}),
		vs("other", "b", []string{"shop.example.com"}, []string{"edge/gw"}, &istioapi.Destination{Host: "shop.apps.svc.cluster.local"}),
	)
	// The fakes mis-bucket the Gateway kind when seeding — insert via typed Create (as in server_test.go).
	if _, err := istio.NetworkingV1().Gateways("edge").Create(context.Background(), gw, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	all := analyzeMesh(context.Background(), c, "")
	if codes(all)["vs-host-conflict"] != 2 {
		t.Fatalf("want a conflict on both VSs, got %v", codes(all))
	}
	// no pods at all → the ingress gateway selector matches nothing
	if codes(all)["gateway-selector-no-pods"] != 1 {
		t.Errorf("want gateway-selector-no-pods, got %v", codes(all))
	}
	scoped := analyzeMesh(context.Background(), c, "other")
	for _, f := range scoped.Findings {
		if f.Resource.Namespace != "other" {
			t.Errorf("ns filter leaked %s/%s", f.Resource.Namespace, f.Resource.Name)
		}
	}
	if codes(scoped)["vs-host-conflict"] != 1 {
		t.Errorf("scoped conflict: got %v", codes(scoped))
	}
}

func TestAnalyzeGatewayAPIStatusAndBackends(t *testing.T) {
	kube := kubefake.NewSimpleClientset(svc("apps", "shop", nil)) // exists, no Endpoints object → 0 ready
	falseCond := metav1.Condition{Type: "Accepted", Status: metav1.ConditionFalse, Reason: "NoMatchingParent", Message: "no listener"}
	port := gwapi.PortNumber(80)
	hr := &gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "route"},
		Spec: gwapi.HTTPRouteSpec{Rules: []gwapi.HTTPRouteRule{{BackendRefs: []gwapi.HTTPBackendRef{
			{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "shop", Port: &port}}},
			{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "ghost", Port: &port}}},
		}}}},
		Status: gwapi.HTTPRouteStatus{RouteStatus: gwapi.RouteStatus{Parents: []gwapi.RouteParentStatus{{Conditions: []metav1.Condition{falseCond}}}}},
	}
	g := &gwapi.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "edge", Name: "gw"},
		Status: gwapi.GatewayStatus{Conditions: []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionFalse, Reason: "Invalid", Message: "bad listener"}}}}
	gwc := gwfake.NewSimpleClientset(hr)
	if _, err := gwc.GatewayV1().Gateways("edge").Create(context.Background(), g, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	c := &k8s.Clients{Kube: kube, Istio: istiofake.NewSimpleClientset(), Gateway: gwc}
	got := codes(analyzeMesh(context.Background(), c, ""))
	for code, n := range map[string]int{"httproute-not-accepted": 1, "httproute-backend-missing": 1, "backend-no-endpoints": 1, "gateway-not-programmed": 1} {
		if got[code] != n {
			t.Errorf("%s: want %d, got %d (all: %v)", code, n, got[code], got)
		}
	}
}

func TestAnalyzeSkipsPodChecksWithoutPermission(t *testing.T) {
	kube := kubefake.NewSimpleClientset(svc("apps", "shop", map[string]string{"app": "shop"}))
	kube.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
	})
	istio := istiofake.NewSimpleClientset(&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"},
		Spec: istioapi.DestinationRule{Host: "shop", Subsets: []*istioapi.Subset{{Name: "v1", Labels: map[string]string{"version": "v1"}}}}})
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	a := analyzeMesh(context.Background(), c, "")
	if len(a.Skipped) != 1 || a.Skipped[0] != "pods" {
		t.Fatalf("want skipped=[pods], got %v", a.Skipped)
	}
	if codes(a)["dr-subset-no-pods"] != 0 {
		t.Errorf("pod check must be skipped: %v", codes(a))
	}
}

func TestAnalyzeEndpointRequiresAuth(t *testing.T) {
	mux := newTestMux(t, &stubSource{clients: &k8s.Clients{Kube: kubefake.NewSimpleClientset(), Istio: istiofake.NewSimpleClientset(), Gateway: gwfake.NewSimpleClientset()}})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/analyze", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq("GET", "/api/analyze?ns=apps", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"findings":[]`) {
		t.Fatalf("want 200 with empty findings, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnalyzeServicesListErrorSkipsExistenceChecks(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	kube.PrependReactor("list", "services", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unavailable")
	})
	istio := istiofake.NewSimpleClientset(
		vs("apps", "v", []string{"shop"}, nil, &istioapi.Destination{Host: "shop"}),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"}, Spec: istioapi.DestinationRule{Host: "shop"}},
	)
	port := gwapi.PortNumber(80)
	hr := &gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "route"},
		Spec: gwapi.HTTPRouteSpec{Rules: []gwapi.HTTPRouteRule{{BackendRefs: []gwapi.HTTPBackendRef{
			{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "shop", Port: &port}}},
		}}}}}
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset(hr)}
	got := codes(analyzeMesh(context.Background(), c, ""))
	for _, code := range []string{"vs-host-missing", "httproute-backend-missing", "dr-host-missing"} {
		if got[code] != 0 {
			t.Errorf("%s must not fire when Services cannot be listed: %v", code, got)
		}
	}
}

func TestAnalyzeExternalNameGatewayRefsAndScopedChecked(t *testing.T) {
	ext := svc("apps", "ext", nil)
	ext.Spec.Type = corev1.ServiceTypeExternalName
	ext.Spec.ExternalName = "api.example.com"
	istio := istiofake.NewSimpleClientset(
		// legacy FQDN gateway reference plus the same gateway again in ns/name form
		vs("apps", "a", []string{"x.example.com"}, []string{"gw.edge.svc.cluster.local", "edge/gw"}, &istioapi.Destination{Host: "ext"}),
	)
	if _, err := istio.NetworkingV1().Gateways("edge").Create(context.Background(),
		&istionet.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "edge", Name: "gw"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	c := &k8s.Clients{Kube: kubefake.NewSimpleClientset(ext), Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	a := analyzeMesh(context.Background(), c, "")
	if len(a.Findings) != 0 {
		t.Errorf("want no findings (ExternalName, FQDN gateway ref, no self-conflict), got %+v", a.Findings)
	}
	if a.Checked != 2 {
		t.Errorf("checked: want 2 (VS + gateway), got %d", a.Checked)
	}
	if n := analyzeMesh(context.Background(), c, "apps").Checked; n != 1 {
		t.Errorf("scoped checked: want 1, got %d", n)
	}
}
