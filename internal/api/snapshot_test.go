package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	istioapi "istio.io/api/networking/v1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/wonjune95/sealane/internal/k8s"
)

func TestSnapshotListsEndpointsOnce(t *testing.T) {
	var objs []runtime.Object
	var vsObjs []runtime.Object
	for i := range 4 {
		name := fmt.Sprintf("svc%d", i)
		objs = append(objs, svc("apps", name, nil))
		vsObjs = append(vsObjs, vs("apps", "vs-"+name, []string{name + ".example.com"}, []string{"edge/gw"}, &istioapi.Destination{Host: name}))
	}
	kube := kubefake.NewSimpleClientset(objs...)
	c := &k8s.Clients{Kube: kube, Istio: istiofake.NewSimpleClientset(vsObjs...), Gateway: gwfake.NewSimpleClientset()}
	endpointCalls := func(what string) {
		t.Helper()
		lists, gets := 0, 0
		for _, a := range kube.Actions() {
			if a.GetResource().Resource != "endpoints" {
				continue
			}
			switch a.GetVerb() {
			case "list":
				lists++
			case "get":
				gets++
			}
		}
		if lists != 1 || gets != 0 {
			t.Errorf("%s: want 1 endpoints list and 0 gets, got %d lists and %d gets", what, lists, gets)
		}
		kube.ClearActions()
	}

	if got := codes(analyzeMesh(context.Background(), c, "")); got["backend-no-endpoints"] != 4 {
		t.Errorf("analyze: want 4 backend-no-endpoints, got %v", got)
	}
	endpointCalls("analyze")

	rec := httptest.NewRecorder()
	newTestMux(t, &stubSource{clients: c}).ServeHTTP(rec, authedReq("GET", "/api/flowmap", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("flowmap: %d %s", rec.Code, rec.Body.String())
	}
	endpointCalls("flowmap")
}

func TestSnapshotListFailureKeepsSkipSemantics(t *testing.T) {
	istio := istiofake.NewSimpleClientset(
		vs("apps", "sub", []string{"shop"}, nil, &istioapi.Destination{Host: "shop", Subset: "v9"}),
		vs("apps", "gone", []string{"gone"}, nil, &istioapi.Destination{Host: "gone"}),
		&istionet.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"}, Spec: istioapi.DestinationRule{Host: "shop"}},
	)
	istio.PrependReactor("list", "destinationrules", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unavailable")
	})
	kube := kubefake.NewSimpleClientset(svc("apps", "shop", nil),
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "shop"}, //nolint:staticcheck // production code reads core Endpoints
			Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}}}) //nolint:staticcheck // see Endpoints above
	c := &k8s.Clients{Kube: kube, Istio: istio, Gateway: gwfake.NewSimpleClientset()}
	got := codes(analyzeMesh(context.Background(), c, ""))
	if got["vs-subset-undefined"] != 0 {
		t.Errorf("DestinationRules not listable: vs-subset-undefined must be skipped, got %v", got)
	}
	if got["vs-host-missing"] != 1 || got["backend-no-endpoints"] != 0 {
		t.Errorf("other kinds must still be checked: %v", got)
	}
}
