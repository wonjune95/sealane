package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// SessionSecretKey is the key under which the session-signing key is kept in the Secret.
const SessionSecretKey = "SESSION_SECRET"

// LoadSessionSecret returns the session-signing key stored in Secret ns/name under
// SessionSecretKey, generating and storing one when it is missing. This gives
// install.yaml and GitOps installs (no Helm lookup) a key that survives restarts
// and is shared by every replica, like Argo CD's generated argocd-secret. The pod
// also reads that Secret through envFrom, so later boots get it as SESSION_SECRET
// and never call this.
func LoadSessionSecret(ctx context.Context, kube kubernetes.Interface, ns, name string) (string, error) {
	secrets := kube.CoreV1().Secrets(ns)
	for attempt := 0; attempt < 2; attempt++ {
		sec, err := secrets.Get(ctx, name, metav1.GetOptions{})
		switch {
		case err == nil && len(sec.Data[SessionSecretKey]) > 0:
			return string(sec.Data[SessionSecretKey]), nil
		case err != nil && !apierrors.IsNotFound(err):
			return "", err
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		key := hex.EncodeToString(b)
		if err == nil { // Secret exists (e.g. holds OIDC_CLIENT_SECRET) but has no key yet
			patch, _ := json.Marshal(map[string]any{"data": map[string][]byte{SessionSecretKey: []byte(key)}}) // []byte → base64, as Secret data wants
			_, err = secrets.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		} else {
			_, err = secrets.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data:       map[string][]byte{SessionSecretKey: []byte(key)},
			}, metav1.CreateOptions{})
		}
		if err == nil {
			return key, nil
		}
		if !apierrors.IsAlreadyExists(err) && !apierrors.IsConflict(err) {
			return "", err
		}
		// another replica won the race — read what it stored
	}
	return "", fmt.Errorf("session secret %s/%s: lost the create race twice", ns, name)
}
