package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/wonjune95/sealane/internal/auth"
)

func impersonationServer(t *testing.T, on bool) (*http.ServeMux, *stubSource) {
	t.Helper()
	base, _ := fakeDynamicClients()
	as, _ := fakeDynamicClients()
	src := &stubSource{clients: base, asClients: as}
	s := NewServer(false, src, auth.NewStore(t.TempDir()), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	s.SetImpersonation(on)
	mux := http.NewServeMux()
	s.Routes(mux)
	return mux, src
}

func TestImpersonation_WritesActAsRoleIdentity(t *testing.T) {
	mux, src := impersonationServer(t, true)
	serve(t, mux, "POST", "/api/resources/"+vsTypeID, vsBody, http.StatusCreated)

	want := rest.ImpersonationConfig{
		UserName: "sealane:editor",
		Groups:   []string{"sealane:editor"},
		Extra:    map[string][]string{"sealane-user": {"tester"}, "sealane-provider": {"local"}},
	}
	if len(src.impersonated) != 1 || !reflect.DeepEqual(src.impersonated[0], want) {
		t.Fatalf("impersonated = %+v, want %+v", src.impersonated, want)
	}
	// The object went through the impersonating clients, not the ServiceAccount's.
	if _, err := src.asClients.Dynamic.Resource(vsGVR).Namespace("default").Get(context.Background(), "app", metav1.GetOptions{}); err != nil {
		t.Errorf("not created as the impersonated identity: %v", err)
	}
	if _, err := src.clients.Dynamic.Resource(vsGVR).Namespace("default").Get(context.Background(), "app", metav1.GetOptions{}); err == nil {
		t.Error("created with the ServiceAccount's clients")
	}
	// Reads stay on the ServiceAccount.
	serve(t, mux, "GET", "/api/resources/"+vsTypeID, "", http.StatusOK)
	if len(src.impersonated) != 1 {
		t.Errorf("a read impersonated: %+v", src.impersonated)
	}
}

func TestImpersonation_OffUsesServiceAccount(t *testing.T) {
	mux, src := impersonationServer(t, false)
	serve(t, mux, "POST", "/api/resources/"+vsTypeID, vsBody, http.StatusCreated)
	if len(src.impersonated) != 0 {
		t.Errorf("impersonated with the mode off: %+v", src.impersonated)
	}
}

func TestAccess_ImpersonationAsksClusterRBAC(t *testing.T) {
	mux, src := impersonationServer(t, true)
	kube := kubefake.NewClientset()
	var asked []authv1.ResourceAttributes
	kube.PrependReactor("create", "selfsubjectaccessreviews", func(a k8stesting.Action) (bool, runtime.Object, error) {
		r := a.(k8stesting.CreateAction).GetObject().(*authv1.SelfSubjectAccessReview)
		asked = append(asked, *r.Spec.ResourceAttributes)
		r.Status.Allowed = r.Spec.ResourceAttributes.Verb == "update"
		return true, r, nil
	})
	src.asClients.Kube = kube

	var got map[string]bool
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/access/"+vsTypeID+"/default/app?verbs=get,update,delete", "", http.StatusOK), &got)
	if !reflect.DeepEqual(got, map[string]bool{"get": true, "update": true, "delete": false}) {
		t.Errorf("access = %v, want get and update only", got)
	}
	if len(asked) != 2 || asked[0].Namespace != "default" || asked[0].Name != "app" || asked[0].Resource != "virtualservices" || asked[0].Group != "networking.istio.io" {
		t.Errorf("access reviews = %+v", asked)
	}
}
