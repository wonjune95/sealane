package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	istioapi "istio.io/api/networking/v1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/wonjune95/sealane/internal/k8s"
)

// The Services list with ?mesh=true carries the counts the service view would show for each Service.
func TestServicesMeshCounts(t *testing.T) {
	t.Setenv("ISTIO_NAMESPACE", "istio-system")
	ready := func(name string) *corev1.Endpoints { //nolint:staticcheck // production flow map reads core Endpoints
		return &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: name}, //nolint:staticcheck // see above
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}} //nolint:staticcheck // see above
	}
	reviews := svc("shop", "reviews", map[string]string{"app": "reviews"})
	reviews.Spec.Ports = []corev1.ServicePort{{Port: 80}}
	kube := kubefake.NewSimpleClientset(
		reviews,
		svc("shop", "ratings", map[string]string{"app": "ratings"}),
		svc("shop", "details", map[string]string{"app": "details"}),
		ready("reviews"), ready("ratings"), ready("details"),
	)
	port := gwapi.PortNumber(80)
	hr := &gwapi.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "reviews-route"},
		Spec: gwapi.HTTPRouteSpec{Rules: []gwapi.HTTPRouteRule{{BackendRefs: []gwapi.HTTPBackendRef{
			{BackendRef: gwapi.BackendRef{BackendObjectReference: gwapi.BackendObjectReference{Name: "reviews", Port: &port}}},
		}}}}}
	istio := istiofake.NewSimpleClientset(
		vs("shop", "reviews-vs", []string{"reviews"}, nil, &istioapi.Destination{Host: "reviews"}),
		vs("shop", "ghost-vs", []string{"ghost"}, nil, &istioapi.Destination{Host: "ghost"}),
		vs("shop", "details-vs", []string{"details"}, nil, &istioapi.Destination{Host: "details", Subset: "v2"}),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "reviews-dr"}, Spec: istioapi.DestinationRule{Host: "reviews.shop.svc.cluster.local"}},
		authzPolicy("shop", "ns-wide", nil),
		authzPolicy("shop", "ratings-only", map[string]string{"app": "ratings"}),
	)
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset(hr)}
	mux := newTestMux(t, &stubSource{clients: c})

	get := func(url string) []map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, authedReq("GET", url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d %s", url, rec.Code, rec.Body.String())
		}
		var out []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	counts := map[string][4]float64{}
	for _, s := range get("/api/services?ns=shop&mesh=true") {
		n := func(k string) float64 { f, _ := s[k].(float64); return f }
		counts[s["name"].(string)] = [4]float64{n("routes"), n("destinationRules"), n("policies"), n("findings")}
	}
	want := map[string][4]float64{"reviews": {2, 1, 1, 0}, "ratings": {0, 0, 2, 0}, "details": {1, 0, 1, 1}}
	for name, w := range want {
		if counts[name] != w {
			t.Errorf("%s: want routes/destinationRules/policies/findings %v, got %v", name, w, counts[name])
		}
	}
	// The ghost VS's vs-host-missing finding exists but concerns no listed Service.
	if a := analyzeMesh(t.Context(), c, "shop"); codes(a)["vs-host-missing"] != 1 {
		t.Errorf("want one vs-host-missing finding, got %+v", a.Findings)
	}

	for _, s := range get("/api/services?ns=shop") {
		for _, k := range []string{"routes", "destinationRules", "policies", "findings"} {
			if _, ok := s[k]; ok {
				t.Errorf("plain list carries %q: %v", k, s)
			}
		}
	}
}
