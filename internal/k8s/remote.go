package k8s

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// ErrFileReference: the kubeconfig names a file for a certificate, key or token. Those paths would
// be read inside Sealane's container (its own ServiceAccount token, for one) and sent to whatever
// server the kubeconfig names, so only inline data is accepted.
var ErrFileReference = errors.New("kubeconfig refers to a file")

// PrepareKubeconfig reduces a pasted kubeconfig to one context (the named one, else its current
// context), so a whole ~/.kube/config with other clusters' credentials is never stored. Only
// inline credentials are accepted: no file references, credential plugins or auth providers, and
// the server must be https. It returns that kubeconfig and the client config it describes.
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
	if err := clientcmdapi.MinifyConfig(raw); err != nil {
		return nil, nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	cluster, user := raw.Clusters[c.Cluster], raw.AuthInfos[c.AuthInfo]
	switch {
	case user != nil && user.Exec != nil:
		return nil, nil, fmt.Errorf("%w (%s)", ErrExecCredential, user.Exec.Command)
	case user != nil && user.AuthProvider != nil:
		return nil, nil, fmt.Errorf("%w (auth-provider %s)", ErrExecCredential, user.AuthProvider.Name)
	case cluster.CertificateAuthority != "":
		return nil, nil, fmt.Errorf("%w: certificate-authority; use certificate-authority-data", ErrFileReference)
	case user != nil && (user.ClientCertificate != "" || user.ClientKey != ""):
		return nil, nil, fmt.Errorf("%w: client-certificate/client-key; use the -data fields", ErrFileReference)
	case user != nil && user.TokenFile != "":
		return nil, nil, fmt.Errorf("%w: tokenFile; use token", ErrFileReference)
	case !strings.HasPrefix(cluster.Server, "https://"):
		return nil, nil, fmt.Errorf("invalid kubeconfig: the server must be an https:// URL, got %q", cluster.Server)
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
	remoteCleanup   = "sealane-remote-cleanup"
	remoteSA        = "sealane"
)

// ManagedRules are the rights the chart gives the local ServiceAccount (charts/sealane/templates/rbac.yaml,
// request tester excluded); keep the two in step. In impersonation mode the account itself only
// reads the managed kinds (managedRules(false)); writes go through sealane:admin / sealane:editor.
var ManagedRules = managedRules(true)

var managedGroups = []string{"networking.istio.io", "security.istio.io", "telemetry.istio.io", "gateway.networking.k8s.io"}

func managedRules(write bool) []rbacv1.PolicyRule {
	verbs := []string{"get", "list", "watch"}
	if write {
		verbs = append(verbs, "create", "update", "patch", "delete")
	}
	return []rbacv1.PolicyRule{
		{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list"}},
		{APIGroups: managedGroups, Resources: []string{"*"}, Verbs: verbs},
		{APIGroups: []string{""}, Resources: []string{"namespaces", "services", "endpoints", "pods"}, Verbs: []string{"get", "list"}},
	}
}

// remoteObjects are the cluster-scoped names SetupRemoteAccount may create, in deletion order.
var remoteObjects = []string{remoteName + "-writer", remoteName + "-impersonator", remoteName}

// SetupRemoteAccount creates or refreshes, on the cluster kube points at, a ServiceAccount with
// ManagedRules (plus the impersonation RBAC when Sealane runs in that mode) and a token for it,
// and returns a kubeconfig that logs in with that token. The caller's credentials are used only
// for this, like `argocd cluster add`; Sealane stores the returned kubeconfig instead of them.
func SetupRemoteAccount(ctx context.Context, kube kubernetes.Interface, base *rest.Config, impersonation bool, poll time.Duration) (out []byte, err error) {
	fresh := false // the namespace is new: a failed setup is rolled back, a failed refresh is not
	defer func() {
		if err != nil && fresh { // do not leave half an account behind; the pasted credentials can remove it
			_ = removeRemoteAccount(context.WithoutCancel(ctx), kube, true)
		}
	}()
	labels := map[string]string{"app.kubernetes.io/managed-by": "sealane"}
	meta := func(name, ns string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels}
	}
	sa := rbacv1.Subject{Kind: "ServiceAccount", Name: remoteSA, Namespace: RemoteNamespace}
	roles := []*rbacv1.ClusterRole{
		{ObjectMeta: meta(remoteName, ""), Rules: managedRules(!impersonation)},
		// Lets the account remove itself when the cluster is deleted in Sealane (CleanupRemoteAccount).
		{ObjectMeta: meta(remoteCleanup, ""), Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"clusterroles", "clusterrolebindings"},
				Verbs: []string{"delete"}, ResourceNames: append([]string{remoteCleanup}, remoteObjects...)},
			{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"delete"}, ResourceNames: []string{RemoteNamespace}},
		}},
	}
	bindings := []*rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: meta(remoteName, ""),
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: remoteName},
			Subjects:   []rbacv1.Subject{sa},
		},
		{
			ObjectMeta: meta(remoteCleanup, ""),
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: remoteCleanup},
			Subjects:   []rbacv1.Subject{sa},
		},
	}
	if impersonation { // same as charts/sealane/templates/impersonation.yaml with bindDefaults
		roles = append(roles,
			&rbacv1.ClusterRole{ObjectMeta: meta(remoteName+"-impersonator", ""), Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"users", "groups"}, Verbs: []string{"impersonate"}, ResourceNames: []string{"sealane:admin", "sealane:editor"}},
				{APIGroups: []string{"authentication.k8s.io"}, Resources: []string{"userextras/sealane-user", "userextras/sealane-provider"}, Verbs: []string{"impersonate"}},
			}},
			&rbacv1.ClusterRole{ObjectMeta: meta(remoteName+"-writer", ""), Rules: managedRules(true)[1:2]},
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

	_, err = kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: meta(RemoteNamespace, "")}, metav1.CreateOptions{})
	fresh = err == nil
	if err = ignoreExists(err); err != nil {
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
	err = wait.PollUntilContextTimeout(ctx, poll, 20*time.Second, true, func(ctx context.Context) (bool, error) {
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

// CleanupRemoteAccount removes the account SetupRemoteAccount created, using that account's own
// token (kube), so unregistering a cluster also revokes Sealane's access to it. Deleting the
// namespace deletes the ServiceAccount and its token. The ClusterRole sealane-remote-cleanup stays
// behind: removing it, or its binding, first would take away the right to remove the rest. It
// grants nothing once the account is gone. Accounts registered before 1.19 lack that role, so
// cleanup fails with Forbidden there.
func CleanupRemoteAccount(ctx context.Context, kube kubernetes.Interface) error {
	return removeRemoteAccount(ctx, kube, false)
}

// removeRemoteAccount deletes the remote account's objects; all=true (the caller has admin rights)
// also deletes the cleanup role and binding.
func removeRemoteAccount(ctx context.Context, kube kubernetes.Interface, all bool) error {
	gone := func(err error) bool { return err == nil || apierrors.IsNotFound(err) }
	for _, name := range remoteObjects {
		if err := kube.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); !gone(err) {
			return fmt.Errorf("delete cluster role binding %s: %w", name, err)
		}
		if err := kube.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{}); !gone(err) {
			return fmt.Errorf("delete cluster role %s: %w", name, err)
		}
	}
	if err := kube.CoreV1().Namespaces().Delete(ctx, RemoteNamespace, metav1.DeleteOptions{}); !gone(err) {
		return fmt.Errorf("delete namespace %s: %w", RemoteNamespace, err)
	}
	if all {
		_ = kube.RbacV1().ClusterRoleBindings().Delete(ctx, remoteCleanup, metav1.DeleteOptions{})
		_ = kube.RbacV1().ClusterRoles().Delete(ctx, remoteCleanup, metav1.DeleteOptions{})
	} else {
		// Best effort: the namespace is going, so the token may already be refused.
		_ = kube.RbacV1().ClusterRoleBindings().Delete(ctx, remoteCleanup, metav1.DeleteOptions{})
	}
	return nil
}

// IsDedicatedKubeconfig reports whether a stored kubeconfig is one SetupRemoteAccount produced.
func IsDedicatedKubeconfig(data []byte) bool {
	kc, err := clientcmd.Load(data)
	if err != nil {
		return false
	}
	c := kc.Contexts[kc.CurrentContext]
	return c != nil && c.Namespace == RemoteNamespace && c.AuthInfo == "sealane" && kc.AuthInfos["sealane"] != nil && kc.AuthInfos["sealane"].Token != ""
}
