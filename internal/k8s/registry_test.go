package k8s

import (
	"os"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const vsTypeID = "virtualservices.networking.istio.io"

// discoveryFactory returns a ClientFactory whose discovery is served from the
// given fake resource lists, plus a counter of discovery constructions.
func discoveryFactory(resources []*metav1.APIResourceList) (*ClientFactory, *fakediscovery.FakeDiscovery, *int) {
	fd := kubefake.NewClientset().Discovery().(*fakediscovery.FakeDiscovery)
	fd.Resources = resources
	calls := 0
	f := &ClientFactory{}
	f.newDiscovery = func() (discovery.DiscoveryInterface, error) {
		calls++
		return fd, nil
	}
	return f, fd, &calls
}

func findType(t *testing.T, cat []ResolvedType, typeID string) ResolvedType {
	t.Helper()
	for _, rt := range cat {
		if rt.TypeID == typeID {
			return rt
		}
	}
	t.Fatalf("type %s not in catalog", typeID)
	return ResolvedType{}
}

func TestCatalog_PrefersGAVersionAndSkipsSubresources(t *testing.T) {
	f, _, _ := discoveryFactory([]*metav1.APIResourceList{
		{GroupVersion: "networking.istio.io/v1beta1", APIResources: []metav1.APIResource{{Name: "virtualservices"}}},
		{GroupVersion: "networking.istio.io/v1", APIResources: []metav1.APIResource{
			{Name: "virtualservices"},
			{Name: "virtualservices/status"},
		}},
	})
	cat, err := f.Catalog()
	if err != nil {
		t.Fatal(err)
	}

	vs := findType(t, cat, vsTypeID)
	if !vs.Installed {
		t.Error("virtualservices should be installed")
	}
	if vs.Version != "v1" {
		t.Errorf("Version = %q, want v1 (GA preferred over v1beta1)", vs.Version)
	}
	if vs.GVR.Version != "v1" {
		t.Errorf("GVR.Version = %q, want v1", vs.GVR.Version)
	}

	// A registered kind absent from discovery reports as not installed.
	hr := findType(t, cat, "httproutes.gateway.networking.k8s.io")
	if hr.Installed {
		t.Error("httproutes should not be installed")
	}
}

func TestResolveType_Errors(t *testing.T) {
	f, _, _ := discoveryFactory(nil)

	if _, err := f.ResolveType(vsTypeID); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("uninstalled type err = %v, want 'not installed'", err)
	}
	if _, err := f.ResolveType("bogus.example.com"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("unknown type err = %v, want 'unknown'", err)
	}
}

// The catalog the API serves and the one ResolveType consults must be the same
// snapshot: a kind must never list as installed while its CRUD path says
// "not installed" (or vice versa).
func TestCatalogCached_SingleConsistentSnapshot(t *testing.T) {
	f, fd, calls := discoveryFactory(nil)

	cat1, err := f.CatalogCached()
	if err != nil {
		t.Fatal(err)
	}
	if findType(t, cat1, vsTypeID).Installed {
		t.Fatal("virtualservices should start uninstalled")
	}

	// CRD gets installed after the first discovery — the cached snapshot must not
	// drift apart between the listing and ResolveType.
	fd.Resources = []*metav1.APIResourceList{
		{GroupVersion: "networking.istio.io/v1", APIResources: []metav1.APIResource{{Name: "virtualservices"}}},
	}

	cat2, err := f.CatalogCached()
	if err != nil {
		t.Fatal(err)
	}
	listed := findType(t, cat2, vsTypeID).Installed
	_, resolveErr := f.ResolveType(vsTypeID)
	if listed != (resolveErr == nil) {
		t.Fatalf("catalog listing (installed=%v) disagrees with ResolveType (err=%v)", listed, resolveErr)
	}
	if *calls != 1 {
		t.Errorf("discovery constructed %d times, want 1 (cached)", *calls)
	}
}

// The chart grants exactly the kinds of the registry, no wildcard.
func TestChartGrantsTheRegistryKinds(t *testing.T) {
	b, err := os.ReadFile("../../charts/sealane/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	_, after, _ := strings.Cut(string(b), `define "sealane.managedResources" -}}`)
	list, _, _ := strings.Cut(after, "{{")
	want := `["` + strings.Join(ManagedResources(), `", "`) + `"]`
	if strings.TrimSpace(list) != want {
		t.Errorf("chart list = %s\nwant          %s", strings.TrimSpace(list), want)
	}
}
