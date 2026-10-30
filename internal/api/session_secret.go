package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return loadOrCreateSecretKey(ctx, kube, ns, name, SessionSecretKey, func() string {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic(err) // no entropy — nothing sane to do
		}
		return hex.EncodeToString(b)
	})
}

// loadOrCreateSecretKey returns key of Secret ns/name, storing gen() there first when it is missing.
// Replicas starting together race to store a value. Only a write against the resourceVersion that
// had none wins; the others get a conflict, read again and use the winner's value.
func loadOrCreateSecretKey(ctx context.Context, kube kubernetes.Interface, ns, name, key string, gen func() string) (string, error) {
	secrets := kube.CoreV1().Secrets(ns)
	for attempt := 0; attempt < 5; attempt++ {
		sec, err := secrets.Get(ctx, name, metav1.GetOptions{})
		switch {
		case err == nil && len(sec.Data[key]) > 0:
			return string(sec.Data[key]), nil
		case err != nil && !apierrors.IsNotFound(err):
			return "", err
		}
		value := gen()
		if err == nil { // the Secret exists (e.g. holds OIDC_CLIENT_SECRET) without this key
			if sec.Data == nil {
				sec.Data = map[string][]byte{}
			}
			sec.Data[key] = []byte(value)
			_, err = secrets.Update(ctx, sec, metav1.UpdateOptions{}) // carries the resourceVersion read above
		} else {
			_, err = secrets.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Data:       map[string][]byte{key: []byte(value)},
			}, metav1.CreateOptions{})
		}
		if err == nil {
			return value, nil
		}
		if !apierrors.IsAlreadyExists(err) && !apierrors.IsConflict(err) {
			return "", err
		}
		// another replica won the race — read what it stored
	}
	return "", fmt.Errorf("secret %s/%s key %s: lost the write race five times", ns, name, key)
}
