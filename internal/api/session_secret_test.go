package api

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestLoadSessionSecret(t *testing.T) {
	ctx := t.Context()
	// Missing Secret: one is created and later calls return the same key.
	kube := kubefake.NewSimpleClientset()
	k1, err := LoadSessionSecret(ctx, kube, "ns", "sealane-secret")
	if err != nil || len(k1) != 64 {
		t.Fatalf("create: key %q err %v", k1, err)
	}
	if k2, _ := LoadSessionSecret(ctx, kube, "ns", "sealane-secret"); k2 != k1 {
		t.Fatalf("second call: want %q, got %q", k1, k2)
	}
	// Existing Secret without the key (holds OIDC_CLIENT_SECRET): the key is added, the other data kept.
	kube = kubefake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sealane-secret"},
		Data:       map[string][]byte{"OIDC_CLIENT_SECRET": []byte("s3")},
	})
	k3, err := LoadSessionSecret(ctx, kube, "ns", "sealane-secret")
	if err != nil || len(k3) != 64 {
		t.Fatalf("patch: key %q err %v", k3, err)
	}
	sec, _ := kube.CoreV1().Secrets("ns").Get(ctx, "sealane-secret", metav1.GetOptions{})
	if string(sec.Data["OIDC_CLIENT_SECRET"]) != "s3" || string(sec.Data[SessionSecretKey]) != k3 {
		t.Fatalf("patched secret data: %v", sec.Data)
	}
	// Existing key is reused as-is.
	kube = kubefake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sealane-secret"},
		Data:       map[string][]byte{SessionSecretKey: []byte("fixed")},
	})
	if k, err := LoadSessionSecret(ctx, kube, "ns", "sealane-secret"); k != "fixed" || err != nil {
		t.Fatalf("reuse: key %q err %v", k, err)
	}
}
