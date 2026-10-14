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
	corev1 "k8s.io/api/core/v1"
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
	root := istioRoot()
	// The snapshot and the namespace-scoped policy lists load side by side.
	var st *meshSnapshot
	var nsPolicies, rootPolicies []securityPolicy
	var wg sync.WaitGroup
	wg.Go(func() { st = loadMeshSnapshot(ctx, c, fullSnapshot) })
	wg.Go(func() { nsPolicies = securityPolicies(ctx, c, ns) })
	if ns != root {
		wg.Go(func() { rootPolicies = securityPolicies(ctx, c, root) })
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, serviceMeshOf(st, svc, nsPolicies, rootPolicies, root, analyzeSnapshot(st, "").Findings))
}

// istioRoot is the Istio root namespace, where selector-less policies apply mesh-wide.
func istioRoot() string {
	if root := os.Getenv("ISTIO_NAMESPACE"); root != "" {
		return root
	}
	return "istio-system"
}

// serviceMeshOf assembles the service view of svc from a fullSnapshot, the security
// policies in svc's namespace and in the root namespace, and the cluster-wide findings.
func serviceMeshOf(st *meshSnapshot, svc *corev1.Service, nsPolicies, rootPolicies []securityPolicy, root string, findings []finding) serviceMesh {
	ns, name := svc.Namespace, svc.Name
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
	for _, f := range findings {
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
	return out
}

// addMeshCounts fills each Service's route, DestinationRule, policy and finding
// counts (GET /api/services?mesh=true) from one snapshot and one policy list.
// ponytail: builds the full service view once per Service (services x routing resources);
// index the resources by host if clusters with thousands of Services make the list slow.
func addMeshCounts(ctx context.Context, c *k8s.Clients, ns string, svcs []k8s.ServiceRef) {
	root := istioRoot()
	var st *meshSnapshot
	var pols, rootPols []securityPolicy
	var wg sync.WaitGroup
	wg.Go(func() { st = loadMeshSnapshot(ctx, c, fullSnapshot) })
	wg.Go(func() { pols = securityPolicies(ctx, c, ns) }) // ns == "": every namespace, root included
	if ns != "" && ns != root {
		wg.Go(func() { rootPols = securityPolicies(ctx, c, root) })
	}
	wg.Wait()
	byNS := map[string][]securityPolicy{}
	for _, p := range slices.Concat(pols, rootPols) {
		byNS[p.Namespace] = append(byNS[p.Namespace], p)
	}
	findings := analyzeSnapshot(st, "").Findings
	for i := range svcs {
		svc := st.services[svcs[i].Namespace+"/"+svcs[i].Name]
		if svc == nil {
			continue
		}
		m := serviceMeshOf(st, svc, byNS[svc.Namespace], byNS[root], root, findings)
		for _, v := range m.VirtualServices {
			if v.Match == "host" {
				svcs[i].Routes++
			}
		}
		svcs[i].Routes += len(m.HTTPRoutes)
		svcs[i].DestinationRules, svcs[i].Policies, svcs[i].Findings = len(m.DestinationRules), len(m.Policies), len(m.Findings)
	}
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
	var authz, peer, req []securityPolicy
	var wg sync.WaitGroup
	wg.Go(func() {
		if l, err := sec.AuthorizationPolicies(ns).List(ctx, opts); err == nil {
			for _, p := range l.Items {
				authz = append(authz, securityPolicy{ref{"authorizationpolicies.security.istio.io", "AuthorizationPolicy", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(),
					targetRefs(p.Spec.GetTargetRef(), p.Spec.GetTargetRefs())})
			}
		}
	})
	wg.Go(func() {
		if l, err := sec.PeerAuthentications(ns).List(ctx, opts); err == nil {
			for _, p := range l.Items {
				peer = append(peer, securityPolicy{ref{"peerauthentications.security.istio.io", "PeerAuthentication", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(), nil})
			}
		}
	})
	wg.Go(func() {
		if l, err := sec.RequestAuthentications(ns).List(ctx, opts); err == nil {
			for _, p := range l.Items {
				req = append(req, securityPolicy{ref{"requestauthentications.security.istio.io", "RequestAuthentication", p.Namespace, p.Name}, p.Spec.GetSelector().GetMatchLabels(),
					targetRefs(p.Spec.GetTargetRef(), p.Spec.GetTargetRefs())})
			}
		}
	})
	wg.Wait()
	return slices.Concat(authz, peer, req)
}
