package k8s

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
)

// The token controller fills in token Secrets; the fake clientset does not, so this reactor does.
func fakeWithTokenController() *kubefake.Clientset {
	kube := kubefake.NewClientset()
	kube.PrependReactor("create", "secrets", func(a k8stesting.Action) (bool, runtime.Object, error) {
		s := a.(k8stesting.CreateAction).GetObject().(*corev1.Secret)
		if s.Type == corev1.SecretTypeServiceAccountToken {
			s.Data = map[string][]byte{corev1.ServiceAccountTokenKey: []byte("tok-123")}
		}
		return false, nil, nil
	})
	return kube
}

func TestSetupRemoteAccount(t *testing.T) {
	ctx := context.Background()
	kube := fakeWithTokenController()
	base := &rest.Config{Host: "https://api.example:6443", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("CA-PEM")}}

	out, err := SetupRemoteAccount(ctx, kube, base, false, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	kc, err := clientcmd.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	c := kc.Contexts[kc.CurrentContext]
	if kc.AuthInfos[c.AuthInfo].Token != "tok-123" || kc.Clusters[c.Cluster].Server != base.Host || string(kc.Clusters[c.Cluster].CertificateAuthorityData) != "CA-PEM" {
		t.Errorf("kubeconfig = %s", out)
	}
	role, err := kube.RbacV1().ClusterRoles().Get(ctx, "sealane-remote", metav1.GetOptions{})
	if err != nil || !reflect.DeepEqual(role.Rules, ManagedRules) {
		t.Errorf("cluster role = %+v, %v", role, err)
	}
	binding, _ := kube.RbacV1().ClusterRoleBindings().Get(ctx, "sealane-remote", metav1.GetOptions{})
	if len(binding.Subjects) != 1 || binding.Subjects[0].Name != "sealane" || binding.Subjects[0].Namespace != RemoteNamespace {
		t.Errorf("binding subjects = %+v", binding.Subjects)
	}
	if _, err := kube.RbacV1().ClusterRoles().Get(ctx, "sealane-remote-impersonator", metav1.GetOptions{}); err == nil {
		t.Error("impersonation RBAC created with the mode off")
	}

	// Registering again (or with impersonation on) updates in place.
	if _, err := SetupRemoteAccount(ctx, kube, base, true, time.Millisecond); err != nil {
		t.Fatalf("second setup: %v", err)
	}
	imp, err := kube.RbacV1().ClusterRoles().Get(ctx, "sealane-remote-impersonator", metav1.GetOptions{})
	if err != nil || !reflect.DeepEqual(imp.Rules[0].ResourceNames, []string{"sealane:admin", "sealane:editor"}) {
		t.Errorf("impersonator role = %+v, %v", imp, err)
	}
	writer, _ := kube.RbacV1().ClusterRoleBindings().Get(ctx, "sealane-remote-writer", metav1.GetOptions{})
	if len(writer.Subjects) != 2 || writer.Subjects[0].Kind != "Group" {
		t.Errorf("writer binding = %+v", writer.Subjects)
	}
}

func TestSetupRemoteAccount_ForbiddenStopsEarly(t *testing.T) {
	kube := kubefake.NewClientset()
	kube.PrependReactor("create", "clusterroles", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errForbidden
	})
	_, err := SetupRemoteAccount(context.Background(), kube, &rest.Config{Host: "https://x"}, false, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "cluster role sealane-remote") {
		t.Errorf("err = %v, want the failing step named", err)
	}
}

var errForbidden = &forbiddenErr{}

type forbiddenErr struct{}

func (*forbiddenErr) Error() string { return "clusterroles.rbac.authorization.k8s.io is forbidden" }

func TestPrepareKubeconfig(t *testing.T) {
	in := []byte(`apiVersion: v1
kind: Config
clusters:
- {name: a, cluster: {server: "https://a"}}
- {name: b, cluster: {server: "https://b", certificate-authority: /nowhere/ca.crt}}
users:
- {name: ua, user: {token: ta}}
- {name: ub, user: {token: tb}}
contexts:
- {name: a, context: {cluster: a, user: ua}}
- {name: b, context: {cluster: b, user: ub}}
current-context: b
`)
	out, cfg, err := PrepareKubeconfig(in, "a")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://a" || strings.Contains(string(out), "tb") || strings.Contains(string(out), "https://b") {
		t.Errorf("kept more than context a: %s", out)
	}
	if _, _, err := PrepareKubeconfig(in, ""); err == nil || !strings.Contains(err.Error(), "refers to files") {
		t.Errorf("current context b reads a CA file: err = %v", err)
	}
}
