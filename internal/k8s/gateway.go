package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayRef is a selectable gateway for route parent/gateway dropdowns. Type
// distinguishes Gateway API gateways (HTTPRoute.parentRefs) from Istio gateways
// (VirtualService.gateways).
type GatewayRef struct {
	Type      string `json:"type"` // "gateway-api" | "istio"
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// ServiceRef is a selectable backend service with its ports (backendRef dropdown
// + existence warning — dry-run does not validate backend targets).
type ServiceRef struct {
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Ports     []int32 `json:"ports"`
	// Services page counts, set only for /api/services?mesh=true.
	Routes           int `json:"routes,omitempty"` // VirtualServices for its host + HTTPRoutes to it
	DestinationRules int `json:"destinationRules,omitempty"`
	Policies         int `json:"policies,omitempty"`
	Findings         int `json:"findings,omitempty"`
}

// ListGateways returns both Gateway API and Istio gateways in a namespace. A
// source the user cannot read (or that is not installed) is skipped, not fatal.
func (c *Clients) ListGateways(ctx context.Context, ns string) []GatewayRef {
	out := []GatewayRef{}
	if gws, err := c.Gateway.GatewayV1().Gateways(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for i := range gws.Items {
			out = append(out, GatewayRef{Type: "gateway-api", Namespace: gws.Items[i].Namespace, Name: gws.Items[i].Name})
		}
	}
	if igws, err := c.Istio.NetworkingV1().Gateways(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for _, g := range igws.Items {
			out = append(out, GatewayRef{Type: "istio", Namespace: g.Namespace, Name: g.Name})
		}
	}
	return out
}

// SubsetRef is a selectable DestinationRule subset (version) for the VirtualService
// `subset` field — so users pick a defined subset instead of typing it (and seeing
// it silently ignored). Host scopes which destinations the subset is valid for.
type SubsetRef struct {
	Namespace string `json:"namespace"`
	DR        string `json:"dr"`   // owning DestinationRule name (for "go to")
	Host      string `json:"host"` // the DR's host this subset applies to
	Name      string `json:"name"`
}

// ListSubsets returns subsets declared by DestinationRules in a namespace. Skipped
// (not fatal) when DR is unreadable/uninstalled.
func (c *Clients) ListSubsets(ctx context.Context, ns string) []SubsetRef {
	out := []SubsetRef{}
	drs, err := c.Istio.NetworkingV1().DestinationRules(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for _, dr := range drs.Items {
		for _, s := range dr.Spec.Subsets {
			out = append(out, SubsetRef{Namespace: dr.Namespace, DR: dr.Name, Host: dr.Spec.Host, Name: s.Name})
		}
	}
	return out
}

func (c *Clients) ListServices(ctx context.Context, ns string) ([]ServiceRef, error) {
	svcs, err := c.Kube.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ServiceRef, 0, len(svcs.Items))
	for i := range svcs.Items {
		ports := make([]int32, 0, len(svcs.Items[i].Spec.Ports))
		for _, p := range svcs.Items[i].Spec.Ports {
			ports = append(ports, p.Port)
		}
		out = append(out, ServiceRef{Namespace: svcs.Items[i].Namespace, Name: svcs.Items[i].Name, Ports: ports})
	}
	return out, nil
}
