package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var vsGVR = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"}

func vsProvider(seed ...runtime.Object) (*ResourceProvider, *dynamicfake.FakeDynamicClient) {
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{vsGVR: "VirtualServiceList"},
		seed...,
	)
	p := &ResourceProvider{dyn: dyn, rt: ResolvedType{
		ResourceType: ResourceType{Kind: "VirtualService", Group: vsGVR.Group, Resource: vsGVR.Resource, Namespaced: true},
		TypeID:       vsTypeID,
		Version:      vsGVR.Version,
		Installed:    true,
		GVR:          vsGVR,
	}}
	return p, dyn
}

func vsObject(ns, name, resourceVersion string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1",
		"kind":       "VirtualService",
		"metadata": map[string]interface{}{
			"name": name, "namespace": ns, "resourceVersion": resourceVersion,
		},
	}}
}

// Integer fields (ports, weights) must survive the JSON round-trip as int64 —
// float64 widening gets rejected as 422 by the API server.
func TestCreate_PreservesIntegerFields(t *testing.T) {
	p, dyn := vsProvider()
	raw := json.RawMessage(`{
		"apiVersion": "networking.istio.io/v1",
		"kind": "VirtualService",
		"metadata": {"name": "app", "namespace": "default"},
		"spec": {"http": [{"route": [{"destination": {"host": "svc", "port": {"number": 8080}}}]}]}
	}`)

	detail, err := p.Create(context.Background(), raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Name != "app" || detail.Namespace != "default" {
		t.Errorf("detail = %s/%s, want default/app", detail.Namespace, detail.Name)
	}

	u, err := dyn.Resource(vsGVR).Namespace("default").Get(context.Background(), "app", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	port := u.Object["spec"].(map[string]interface{})["http"].([]interface{})[0].(map[string]interface{})["route"].([]interface{})[0].(map[string]interface{})["destination"].(map[string]interface{})["port"].(map[string]interface{})["number"]
	if _, ok := port.(int64); !ok {
		t.Errorf("port.number decoded as %T, want int64", port)
	}
}

func TestCreate_MalformedJSONIsBadRequest(t *testing.T) {
	p, _ := vsProvider()
	_, err := p.Create(context.Background(), json.RawMessage(`{not json`), false)
	var bre *BadRequestError
	if !errors.As(err, &bre) {
		t.Fatalf("err = %v (%T), want *BadRequestError", err, err)
	}
}

func TestUpdate_RequiresResourceVersion(t *testing.T) {
	p, _ := vsProvider(vsObject("default", "app", "1"))
	raw := json.RawMessage(`{
		"apiVersion": "networking.istio.io/v1",
		"kind": "VirtualService",
		"metadata": {"name": "app", "namespace": "default"}
	}`)
	_, err := p.Update(context.Background(), "default", "app", raw, false)
	if !errors.Is(err, ErrResourceVersionRequired) {
		t.Fatalf("err = %v, want ErrResourceVersionRequired", err)
	}
}

// The path, not the body, decides which object an update targets — a body with a
// different metadata.name/namespace must not redirect the write.
func TestUpdate_ForcesPathNamespaceAndName(t *testing.T) {
	p, dyn := vsProvider(vsObject("default", "app", "1"))
	raw := json.RawMessage(`{
		"apiVersion": "networking.istio.io/v1",
		"kind": "VirtualService",
		"metadata": {"name": "evil", "namespace": "other", "resourceVersion": "1"},
		"spec": {"hosts": ["a.example.com"]}
	}`)

	detail, err := p.Update(context.Background(), "default", "app", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Namespace != "default" || detail.Name != "app" {
		t.Errorf("updated %s/%s, want default/app", detail.Namespace, detail.Name)
	}
	if _, err := dyn.Resource(vsGVR).Namespace("other").Get(context.Background(), "evil", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("object was written under body coordinates other/evil: err = %v", err)
	}
}

func TestListAndDelete(t *testing.T) {
	p, _ := vsProvider(vsObject("default", "app", "1"), vsObject("default", "web", "1"))

	items, err := p.List(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("List = %d items, want 2", len(items))
	}

	if err := p.Delete(context.Background(), "default", "app"); err != nil {
		t.Fatal(err)
	}
	items, err = p.List(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "web" {
		t.Fatalf("after delete List = %+v, want only web", items)
	}
}

func TestManagedBy(t *testing.T) {
	cases := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		want        *ManagedBy
	}{
		{"unmanaged", nil, nil, nil},
		{"argocd annotation tracking", nil,
			map[string]string{"argocd.argoproj.io/tracking-id": "shop-prod:networking.istio.io/VirtualService:shop/reviews"},
			&ManagedBy{"argocd", "shop-prod"}},
		// Argo CD renders charts with `helm template`, so Helm's label is there too.
		{"argocd wins over helm label", map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			map[string]string{"argocd.argoproj.io/tracking-id": "shop:networking.istio.io/VirtualService:shop/reviews"},
			&ManagedBy{"argocd", "shop"}},
		{"argocd instance label", map[string]string{"argocd.argoproj.io/instance": "shop"}, nil, &ManagedBy{"argocd", "shop"}},
		{"flux kustomization", map[string]string{"kustomize.toolkit.fluxcd.io/name": "apps"}, nil, &ManagedBy{"flux", "apps"}},
		// helm-controller also sets Helm's release annotations.
		{"flux helmrelease", map[string]string{"helm.toolkit.fluxcd.io/name": "shop"},
			map[string]string{"meta.helm.sh/release-name": "shop"}, &ManagedBy{"flux", "shop"}},
		{"helm release", map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			map[string]string{"meta.helm.sh/release-name": "shop"}, &ManagedBy{"helm", "shop"}},
		// Rendered with `helm template` and applied by hand (install.yaml): nothing upgrades it.
		{"helm template only", map[string]string{"app.kubernetes.io/managed-by": "Helm"}, nil, nil},
		{"kustomize label only", map[string]string{"app.kubernetes.io/managed-by": "kustomize"}, nil, nil},
	}
	for _, c := range cases {
		u := &unstructured.Unstructured{}
		u.SetLabels(c.labels)
		u.SetAnnotations(c.annotations)
		got := managedBy(u)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: managedBy = %+v, want %+v", c.name, got, c.want)
		}
	}
}
