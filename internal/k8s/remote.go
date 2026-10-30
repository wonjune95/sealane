package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// ErrExecCredential: the kubeconfig logs in through a credential plugin (aws, gcloud, kubelogin…),
// which the distroless image cannot run.
var ErrExecCredential = errors.New("kubeconfig uses an exec credential plugin")

// PrepareKubeconfig reduces a pasted kubeconfig to one context (the named one, else its current
// context) with credentials inlined, so a whole ~/.kube/config with other clusters' credentials
// is never stored. It returns that kubeconfig and the client config it describes.
func PrepareKubeconfig(data []byte, context string) ([]byte, *rest.Config, error) {
	raw, err := clientcmd.Load(data)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	if context != "" {
		raw.CurrentContext = context
	}
	c, ok := raw.Contexts[raw.CurrentContext]
	if !ok {
		return nil, nil, fmt.Errorf("context %q not found in the kubeconfig", raw.CurrentContext)
	}
	if user := raw.AuthInfos[c.AuthInfo]; user != nil && user.Exec != nil {
		return nil, nil, fmt.Errorf("%w (%s)", ErrExecCredential, user.Exec.Command)
	}
	if err := clientcmdapi.MinifyConfig(raw); err != nil {
		return nil, nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	if err := clientcmdapi.FlattenConfig(raw); err != nil { // file references cannot be read here
		return nil, nil, fmt.Errorf("kubeconfig refers to files; paste one with the certificate and key data inline: %w", err)
	}
	out, err := clientcmd.Write(*raw)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(out)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	return out, cfg, nil
}

// Sealane's own account on a remote cluster, created by SetupRemoteAccount.
const (
	RemoteNamespace = "sealane-remote"
	remoteName      = "sealane-remote"
	remoteSA        = "sealane"
)

// ManagedRules are the rights the chart gives the local ServiceAccount (charts/sealane/templates/rbac.yaml,
// request tester excluded); keep the two in step.
var ManagedRules = []rbacv1.PolicyRule{
	{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list"}},
	{
		APIGroups: []string{"networking.istio.io", "security.istio.io", "telemetry.istio.io", "gateway.networking.k8s.io"},
		Resources: []string{"*"},
		Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
	},
	{APIGroups: []string{""}, Resources: []string{"namespaces", "services", "endpoints", "pods"}, Verbs: []string{"get", "list"}},
}

// SetupRemoteAccount creates or refreshes, on the cluster kube points at, a ServiceAccount with
// ManagedRules (plus the impersonation RBAC when Sealane runs in that mode) and a token for it,
// and returns a kubeconfig that logs in with that token. The caller's credentials are used only
// for this, like `argocd cluster add`; Sealane stores the returned kubeconfig instead of them.
func SetupRemoteAccount(ctx context.Context, kube kubernetes.Interface, base *rest.Config, impersonation bool, poll time.Duration) ([]byte, error) {
	labels := map[string]string{"app.kubernetes.io/managed-by": "sealane"}
	meta := func(name, ns string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels}
	}
	sa := rbacv1.Subject{Kind: "ServiceAccount", Name: remoteSA, Namespace: RemoteNamespace}
	roles := []*rbacv1.ClusterRole{{ObjectMeta: meta(remoteName, ""), Rules: ManagedRules}}
	bindings := []*rbacv1.ClusterRoleBinding{{
		ObjectMeta: meta(remoteName, ""),
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: remoteName},
		Subjects:   []rbacv1.Subject{sa},
	}}
	if impersonation { // same as charts/sealane/templates/impersonation.yaml with bindDefaults
		roles = append(roles,
			&rbacv1.ClusterRole{ObjectMeta: meta(remoteName+"-impersonator", ""), Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"users", "groups"}, Verbs: []string{"impersonate"}, ResourceNames: []string{"sealane:admin", "sealane:editor"}},
				{APIGroups: []string{"authentication.k8s.io"}, Resources: []string{"userextras/sealane-user", "userextras/sealane-provider"}, Verbs: []string{"impersonate"}},
			}},
			&rbacv1.ClusterRole{ObjectMeta: meta(remoteName+"-writer", ""), Rules: ManagedRules[1:2]},
		)
		group := func(n string) rbacv1.Subject {
			return rbacv1.Subject{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: n}
		}
		bindings = append(bindings,
			&rbacv1.ClusterRoleBinding{ObjectMeta: meta(remoteName+"-impersonator", ""),
				RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: remoteName + "-impersonator"}, Subjects: []rbacv1.Subject{sa}},
			&rbacv1.ClusterRoleBinding{ObjectMeta: meta(remoteName+"-writer", ""),
				RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: remoteName + "-writer"}, Subjects: []rbacv1.Subject{group("sealane:admin"), group("sealane:editor")}},
		)
	}

	if _, err := kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: meta(RemoteNamespace, "")}, metav1.CreateOptions{}); ignoreExists(err) != nil {
		return nil, fmt.Errorf("create namespace %s: %w", RemoteNamespace, err)
	}
	if _, err := kube.CoreV1().ServiceAccounts(RemoteNamespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: meta(remoteSA, RemoteNamespace)}, metav1.CreateOptions{}); ignoreExists(err) != nil {
		return nil, fmt.Errorf("create service account: %w", err)
	}
	for _, role := range roles {
		_, err := kube.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			_, err = kube.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{})
		}
		if err != nil {
			return nil, fmt.Errorf("cluster role %s: %w", role.Name, err)
		}
	}
	for _, binding := range bindings {
		_, err := kube.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			_, err = kube.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{})
		}
		if err != nil {
			return nil, fmt.Errorf("cluster role binding %s: %w", binding.Name, err)
		}
	}
	// A long-lived token Secret, the same kind `argocd cluster add` creates; the token controller fills it in.
	tokenSecret := &corev1.Secret{
		ObjectMeta: meta(remoteName+"-token", RemoteNamespace),
		Type:       corev1.SecretTypeServiceAccountToken,
	}
	tokenSecret.Annotations = map[string]string{corev1.ServiceAccountNameKey: remoteSA}
	if _, err := kube.CoreV1().Secrets(RemoteNamespace).Create(ctx, tokenSecret, metav1.CreateOptions{}); ignoreExists(err) != nil {
		return nil, fmt.Errorf("create token secret: %w", err)
	}
	var token []byte
	err := wait.PollUntilContextTimeout(ctx, poll, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		s, err := kube.CoreV1().Secrets(RemoteNamespace).Get(ctx, tokenSecret.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		token = s.Data[corev1.ServiceAccountTokenKey]
		return len(token) > 0, nil
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for the service account token: %w", err)
	}

	cluster := clientcmdapi.NewCluster()
	cluster.Server = base.Host
	cluster.CertificateAuthorityData = base.CAData
	cluster.InsecureSkipTLSVerify = base.Insecure
	cluster.TLSServerName = base.ServerName
	user := clientcmdapi.NewAuthInfo()
	user.Token = string(token)
	kc := clientcmdapi.NewConfig()
	kc.Clusters["remote"] = cluster
	kc.AuthInfos["sealane"] = user
	kc.Contexts["remote"] = &clientcmdapi.Context{Cluster: "remote", AuthInfo: "sealane", Namespace: RemoteNamespace}
	kc.CurrentContext = "remote"
	return clientcmd.Write(*kc)
}

func ignoreExists(err error) error {
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
