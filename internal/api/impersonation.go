package api

import (
	"context"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
)

// Impersonation mode (opt-in): resource writes and the editor's permission check run as the
// signed-in user's role identity instead of Sealane's ServiceAccount, so cluster RBAC decides
// what admins and editors may change (per namespace, per kind) and the API server's audit log
// names the person. Reads, the change history, accounts and the request tester stay on the
// ServiceAccount.
//
// The identity is fixed per role, sealane:admin or sealane:editor (user and group), so the
// ServiceAccount only needs to impersonate those two names; that bounds what a compromised
// Sealane could become. The person's name and login provider travel as user extras
// (sealane-user, sealane-provider), which the Kubernetes audit log records.
const impersonationPrefix = "sealane:"

// SetImpersonation turns impersonation mode on (IMPERSONATION=true).
func (s *Server) SetImpersonation(on bool) { s.impersonate = on }

func impersonationFor(id auth.Identity) rest.ImpersonationConfig {
	name := impersonationPrefix + id.Role
	return rest.ImpersonationConfig{
		UserName: name,
		Groups:   []string{name},
		Extra:    map[string][]string{"sealane-user": {id.Name}, "sealane-provider": {id.Provider}},
	}
}

// writeClients are the clients a mutation uses: the request's (ServiceAccount) clients, or
// impersonating ones in impersonation mode.
func (s *Server) writeClients(ctx context.Context) (*k8s.Clients, error) {
	if !s.impersonate {
		return clientFrom(ctx), nil
	}
	return factoryFrom(ctx).Impersonating(impersonationFor(identityFrom(ctx)))
}

// writeProvider is resourceProvider for mutations.
func (s *Server) writeProvider(ctx context.Context, typeID string) (*k8s.ResourceProvider, error) {
	rt, err := factoryFrom(ctx).ResolveType(typeID)
	if err != nil {
		return nil, err
	}
	c, err := s.writeClients(ctx)
	if err != nil {
		return nil, err
	}
	return k8s.NewResourceProvider(c, rt), nil
}

// canImpersonated asks the API server whether the impersonated identity may perform verb;
// on any error it answers false, since the write would fail the same way.
func canImpersonated(ctx context.Context, c *k8s.Clients, rt k8s.ResolvedType, verb, ns, name string) bool {
	if !rt.Namespaced {
		ns = ""
	}
	rev, err := c.Kube.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authv1.ResourceAttributes{
			Namespace: ns, Name: name, Verb: verb, Group: rt.GVR.Group, Version: rt.GVR.Version, Resource: rt.GVR.Resource,
		}},
	}, metav1.CreateOptions{})
	return err == nil && rev.Status.Allowed
}
