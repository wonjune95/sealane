package api

import (
	"cmp"
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	istioapi "istio.io/api/networking/v1"
	istiotype "istio.io/api/type/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/wonjune95/sealane/internal/k8s"
)

// Service mesh view: everything that routes to, configures or secures one Service.

type policyRef struct {
	ref
	Scope string `json:"scope"` // "workload" (selector matches) | "namespace" (no selector) | "mesh" (root namespace, no selector)
}

type vsRef struct {
	ref
	Match string `json:"match"` // "host" (the VS is for this Service) | "destination" (the VS routes to it)
}

type serviceMesh struct {
	Namespace        string            `json:"namespace"`
	Name             string            `json:"name"`
	Selector         map[string]string `json:"selector,omitempty"`
	Ports            []int32           `json:"ports"`
	Endpoints        int               `json:"endpoints"`
	VirtualServices  []vsRef           `json:"virtualServices"`
	DestinationRules []ref             `json:"destinationRules"`
	HTTPRoutes       []ref             `json:"httpRoutes"`
	Gateways         []ref             `json:"gateways"` // Istio Gateways named by those VSs + Gateway API Gateways in those HTTPRoutes' parentRefs
	Policies         []policyRef       `json:"policies"` // AuthorizationPolicy, PeerAuthentication, RequestAuthentication
	Findings         []finding         `json:"findings"` // analyzeMesh findings with Service == ns/name or on a listed resource
}

func (s *Server) handleServiceMesh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := clientFrom(ctx)
	ns, name := r.PathValue("ns"), r.PathValue("name")
	svc, err := c.Kube.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	root := os.Getenv("ISTIO_NAMESPACE")
	if root == "" {
		root = "istio-system"
	}
	// The snapshot and the namespace-scoped policy lists load side by side.
	var st *meshSnapshot
	var nsPolicies, rootPolicies []securityPolicy
	var wg sync.WaitGroup
	wg.Go(func() { st = loadMeshSnapshot(ctx, c, snapshotOpts{pods: true}) })
	wg.Go(func() { nsPolicies = securityPolicies(ctx, c, ns) })
	if ns != root {
		wg.Go(func() { rootPolicies = securityPolicies(ctx, c, root) })
	}
	wg.Wait()

	key := ns + "/" + name
	out := serviceMesh{Namespace: ns, Name: name, Selector: svc.Spec.Selector, Ports: []int32{},
		Endpoints: readyEndpoints(st, ns, name), VirtualServices: []vsRef{}, DestinationRules: []ref{},
		HTTPRoutes: []ref{}, Gateways: []ref{}, Policies: []policyRef{}, Findings: []finding{}}
	for _, p := range svc.Spec.Ports {
		out.Ports = append(out.Ports, p.Port)
	}
	fqdn := name + "." + ns + ".svc.cluster.local"
	isSvc := func(b flowBackend) bool { return !b.External && b.Namespace+"/"+b.Name == key }
	gws := map[ref]bool{}

	for _, v := range st.vss {
		match := ""
		// a bare "*" host is a gateway catch-all, never "the VS for this Service"
		hosts := slices.DeleteFunc(slices.Clone(v.Spec.Hosts), func(h string) bool { return h == "*" })
		if hostsMatch(hosts, fqdn, v.Namespace) {
			match = "host"
		} else if slices.ContainsFunc(vsDestinations(&v.Spec), func(d *istioapi.Destination) bool {
			return isSvc(resolveDestination(d.Host, v.Namespace, 0))
		}) {
			match = "destination"
		}
		if match == "" {
			continue
		}
		out.VirtualServices = append(out.VirtualServices, vsRef{ref{typeVS, "VirtualService", v.Namespace, v.Name}, match})
		for _, g := range v.Spec.Gateways {
			if g != "mesh" {
				gns, gname, _ := strings.Cut(istioGatewayKey(g, v.Namespace), "/")
				gws[ref{typeIstioGW, "Gateway", gns, gname}] = true
			}
		}
	}
	for _, dr := range st.drs {
		if hostsMatch([]string{dr.Spec.Host}, fqdn, dr.Namespace) {
			out.DestinationRules = append(out.DestinationRules, ref{typeDR, "DestinationRule", dr.Namespace, dr.Name})
		}
	}
	for _, hr := range st.hrs {
		found := false
		for _, rule := range hr.Spec.Rules {
			for _, br := range rule.BackendRefs {
				bns := hr.Namespace
				if br.Namespace != nil {
					bns = string(*br.Namespace)
				}
				found = found || (br.Kind == nil || *br.Kind == "Service") && bns+"/"+string(br.Name) == key
			}
		}
		if !found {
			continue
		}
		out.HTTPRoutes = append(out.HTTPRoutes, ref{typeHTTPRoute, "HTTPRoute", hr.Namespace, hr.Name})
		for _, p := range hr.Spec.ParentRefs {
			if p.Kind != nil && *p.Kind != "Gateway" {
				continue
			}
			pns := hr.Namespace
			if p.Namespace != nil {
				pns = string(*p.Namespace)
			}
			gws[ref{typeGateway, "Gateway", pns, string(p.Name)}] = true
		}
	}
	for g := range gws {
		out.Gateways = append(out.Gateways, g)
	}

	// ponytail: workload scope compares the policy selector with the Service selector, not with pod labels; a policy that selects pods by a label the Service selector lacks is missed.
	for _, p := range nsPolicies {
		switch {
		case len(p.targets) > 0: // targetRef(s) replace the selector; cross-namespace refs are not supported
			if slices.ContainsFunc(p.targets, func(t *istiotype.PolicyTargetReference) bool {
				return t.Kind == "Service" && t.Name == name && (t.Namespace == "" || t.Namespace == ns)
			}) {
				out.Policies = append(out.Policies, policyRef{p.ref, "workload"})
			}
		case len(p.sel) == 0 && ns == root:
			out.Policies = append(out.Policies, policyRef{p.ref, "mesh"})
		case len(p.sel) == 0:
			out.Policies = append(out.Policies, policyRef{p.ref, "namespace"})
		case labels.SelectorFromSet(p.sel).Matches(labels.Set(svc.Spec.Selector)):
			out.Policies = append(out.Policies, policyRef{p.ref, "workload"})
		}
	}
	if ns != root {
		for _, p := range rootPolicies {
			if len(p.sel) == 0 && len(p.targets) == 0 {
				out.Policies = append(out.Policies, policyRef{p.ref, "mesh"})
			}
		}
	}

	// Findings on the resources this page lists count as well (e.g. a bound Gateway with no pods).
	listed := map[ref]bool{}
	for _, v := range out.VirtualServices {
		listed[v.ref] = true
	}
	for _, r := range slices.Concat(out.DestinationRules, out.HTTPRoutes, out.Gateways) {
		listed[r] = true
	}
	for _, f := range analyzeSnapshot(st, "").Findings {
		if f.Service == key || listed[f.Resource] {
			out.Findings = append(out.Findings, f)
		}
	}

	byRef := func(a, b ref) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name), cmp.Compare(a.TypeID, b.TypeID))
	}
	slices.SortFunc(out.VirtualServices, func(a, b vsRef) int { return byRef(a.ref, b.ref) })
	slices.SortFunc(out.DestinationRules, byRef)
	slices.SortFunc(out.HTTPRoutes, byRef)
	slices.SortFunc(out.Gateways, byRef)
	slices.SortFunc(out.Policies, func(a, b policyRef) int { return byRef(a.ref, b.ref) })
	writeJSON(w, http.StatusOK, out)
}

type securityPolicy struct {
	ref
	sel     map[string]string
	targets []*istiotype.PolicyTargetReference
}

func targetRefs(one *istiotype.PolicyTargetReference, many []*istiotype.PolicyTargetReference) []*istiotype.PolicyTargetReference {
	if one != nil {
		return append([]*istiotype.PolicyTargetReference{one}, many...)
	}
	return many
}

// securityPolicies lists AuthorizationPolicies, PeerAuthentications and
// RequestAuthentications in ns; a kind that cannot be listed is left out.
func securityPolicies(ctx context.Context, c *k8s.Clients, ns string) []securityPolicy {
	sec, opts := c.Istio.SecurityV1(), metav1.ListOptions{}
	var out []securityPolicy
	if l, err := sec.AuthorizationPolicies(ns).List(ctx, opts); err == nil {
		for _, p := range l.Items {
			out = append(out, securityPolicy{ref{"authorizationpolicies.security.istio.io", "AuthorizationPolicy", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(),
				targetRefs(p.Spec.GetTargetRef(), p.Spec.GetTargetRefs())})
		}
	}
	if l, err := sec.PeerAuthentications(ns).List(ctx, opts); err == nil {
		for _, p := range l.Items {
			out = append(out, securityPolicy{ref{"peerauthentications.security.istio.io", "PeerAuthentication", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(), nil})
		}
	}
	if l, err := sec.RequestAuthentications(ns).List(ctx, opts); err == nil {
		for _, p := range l.Items {
			out = append(out, securityPolicy{ref{"requestauthentications.security.istio.io", "RequestAuthentication", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(),
				targetRefs(p.Spec.GetTargetRef(), p.Spec.GetTargetRefs())})
		}
	}
	return out
}
