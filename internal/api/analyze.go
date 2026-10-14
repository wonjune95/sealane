package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	istioapi "istio.io/api/networking/v1"
	istionet "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/wonjune95/sealane/internal/k8s"
)

// Mesh analysis: cross-resource checks that explain why traffic would not flow,
// computed from routing config alone (plus pods when the ServiceAccount may read them).

type ref struct {
	TypeID    string `json:"typeId"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type finding struct {
	Severity string `json:"severity"` // "error" | "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
	Resource ref    `json:"resource"`
	Related  []ref  `json:"related,omitempty"`
	Service  string `json:"service,omitempty"` // "ns/name" of the Service the finding concerns
}

type analysis struct {
	Findings []finding `json:"findings"`
	Skipped  []string  `json:"skipped,omitempty"` // checks that could not run: "pods" (no RBAC)
	Checked  int       `json:"checked"`           // routing resources inspected
}

const (
	typeVS        = "virtualservices.networking.istio.io"
	typeDR        = "destinationrules.networking.istio.io"
	typeIstioGW   = "gateways.networking.istio.io"
	typeHTTPRoute = "httproutes.gateway.networking.k8s.io"
	typeGateway   = "gateways.gateway.networking.k8s.io"
)

// meshState is everything analyzeMesh reads, fetched once. Indexes whose list
// fails (CRD not installed, RBAC) stay nil and the checks that need them are skipped.
type meshState struct {
	services map[string]*corev1.Service // ns/name; nil when Services cannot be listed
	pods     map[string][]labels.Set    // ns → pod label sets; nil when pods are not readable
	podsOK   bool
	ses      []flowServiceEntry
	drs      []*istionet.DestinationRule  // nil when DestinationRules cannot be listed
	istioGWs map[string]*istioapi.Gateway // ns/name; nil when Istio Gateways cannot be listed
	epCache  map[string]int               // ns/name → ready endpoints, read once per Service
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, analyzeMesh(r.Context(), clientFrom(r.Context()), r.URL.Query().Get("ns")))
}

// analyzeMesh inspects the whole cluster and returns findings whose Resource
// lives in ns (ns == "" → all). Cross-namespace references are always resolved
// cluster-wide; only the reporting scope is filtered.
func analyzeMesh(ctx context.Context, c *k8s.Clients, ns string) analysis {
	out := analysis{Findings: []finding{}}
	st := loadMeshState(ctx, c)
	if !st.podsOK {
		out.Skipped = append(out.Skipped, "pods")
	}
	inScope := func(resNS string) bool { return ns == "" || resNS == ns }
	check := func(resNS string) {
		if inScope(resNS) {
			out.Checked++
		}
	}
	seen := map[string]bool{} // a VS splitting traffic across subsets of one Service must not repeat a finding
	add := func(f finding) {
		key := f.Code + "|" + f.Resource.TypeID + "|" + f.Resource.Namespace + "/" + f.Resource.Name + "|" + f.Message
		if inScope(f.Resource.Namespace) && !seen[key] {
			seen[key] = true
			out.Findings = append(out.Findings, f)
		}
	}
	svcKey := func(b flowBackend) string { return b.Namespace + "/" + b.Name }

	// --- VirtualServices ---
	type gwHost struct{ gw, host string }
	claims := map[gwHost][]ref{}
	if vss, err := c.Istio.NetworkingV1().VirtualServices("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range vss.Items {
			v := vss.Items[i]
			self := ref{TypeID: typeVS, Kind: "VirtualService", Namespace: v.Namespace, Name: v.Name}
			check(v.Namespace)
			for _, g := range v.Spec.Gateways {
				if g == "mesh" {
					continue
				}
				full := g
				if !strings.Contains(g, "/") {
					if name, rest, ok := strings.Cut(g, "."); ok { // legacy "gw.ns.svc.cluster.local" form
						gwNS, _, _ := strings.Cut(rest, ".")
						full = gwNS + "/" + name
					} else {
						full = v.Namespace + "/" + g
					}
				}
				if _, ok := st.istioGWs[full]; !ok && st.istioGWs != nil {
					add(finding{Severity: "error", Code: "vs-gateway-missing", Resource: self,
						Message: fmt.Sprintf("gateway %q does not exist", full)})
				}
				for _, h := range v.Spec.Hosts {
					k := gwHost{full, h}
					if rs := claims[k]; len(rs) == 0 || rs[len(rs)-1] != self { // same gateway or host listed twice
						claims[k] = append(rs, self)
					}
				}
			}
			for _, d := range vsDestinations(&v.Spec) {
				b := resolveDestination(d.Host, v.Namespace, 0)
				if b.External {
					continue
				}
				svc, ok := st.services[svcKey(b)]
				if !ok && st.services != nil {
					add(finding{Severity: "error", Code: "vs-host-missing", Resource: self, Service: svcKey(b),
						Message: fmt.Sprintf("destination %q is not a Service (looked for %s)", d.Host, svcKey(b))})
					continue
				}
				if ok && st.noEndpoints(ctx, c, svc) {
					add(finding{Severity: "warning", Code: "backend-no-endpoints", Resource: self, Service: svcKey(b),
						Message: fmt.Sprintf("Service %s has no ready endpoints", svcKey(b))})
				}
				if d.Subset != "" && !st.subsetDefined(b, d.Subset) {
					add(finding{Severity: "error", Code: "vs-subset-undefined", Resource: self, Service: svcKey(b),
						Message: fmt.Sprintf("subset %q is not defined in any DestinationRule for %s", d.Subset, svcKey(b))})
				}
			}
		}
	}
	for k, refs := range claims {
		if len(refs) < 2 {
			continue
		}
		for i, r := range refs {
			others := append(append([]ref{}, refs[:i]...), refs[i+1:]...)
			add(finding{Severity: "warning", Code: "vs-host-conflict", Resource: r, Related: others,
				Message: fmt.Sprintf("host %q on gateway %s is also claimed by %d other VirtualService(s); rule order between them is undefined", k.host, k.gw, len(others))})
		}
	}

	// --- DestinationRules: host exists, subsets select pods ---
	for _, dr := range st.drs {
		self := ref{TypeID: typeDR, Kind: "DestinationRule", Namespace: dr.Namespace, Name: dr.Name}
		check(dr.Namespace)
		host := dr.Spec.Host
		b := resolveDestination(host, dr.Namespace, 0)
		if b.External || strings.Contains(host, "*") {
			continue
		}
		svc, ok := st.services[svcKey(b)]
		if !ok && st.services == nil {
			continue
		}
		if !ok {
			fqdn := b.Name + "." + b.Namespace + ".svc.cluster.local"
			if serviceEntryFor(st.ses, host) == "" && serviceEntryFor(st.ses, fqdn) == "" {
				add(finding{Severity: "warning", Code: "dr-host-missing", Resource: self, Service: svcKey(b),
					Message: fmt.Sprintf("host %q is not a Service (looked for %s) and no ServiceEntry covers it", host, svcKey(b))})
			}
			continue
		}
		if !st.podsOK || len(svc.Spec.Selector) == 0 {
			continue
		}
		for _, ss := range dr.Spec.Subsets {
			sel := map[string]string{}
			for k, v := range svc.Spec.Selector {
				sel[k] = v
			}
			for k, v := range ss.Labels {
				sel[k] = v
			}
			if !st.anyPodMatches(svc.Namespace, sel) {
				add(finding{Severity: "warning", Code: "dr-subset-no-pods", Resource: self, Service: svcKey(b),
					Message: fmt.Sprintf("subset %q selects no pod (%s)", ss.Name, labels.Set(sel).String())})
			}
		}
	}

	// --- Istio Gateways: selector → pods ---
	for key, g := range st.istioGWs {
		gns, name, _ := strings.Cut(key, "/")
		check(gns)
		if st.podsOK && len(g.Selector) > 0 && !st.anyPodMatches("", g.Selector) {
			add(finding{Severity: "warning", Code: "gateway-selector-no-pods",
				Resource: ref{TypeID: typeIstioGW, Kind: "Gateway", Namespace: gns, Name: name},
				Message:  fmt.Sprintf("selector %s matches no pod in any namespace", labels.Set(g.Selector).String())})
		}
	}

	// --- HTTPRoutes: status, backends ---
	if hrs, err := c.Gateway.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range hrs.Items {
			hr := &hrs.Items[i]
			self := ref{TypeID: typeHTTPRoute, Kind: "HTTPRoute", Namespace: hr.Namespace, Name: hr.Name}
			check(hr.Namespace)
			for _, p := range hr.Status.Parents {
				for _, cond := range p.Conditions {
					if (cond.Type == "Accepted" || cond.Type == "ResolvedRefs") && cond.Status == metav1.ConditionFalse {
						add(finding{Severity: "error", Code: "httproute-not-accepted", Resource: self,
							Message: cond.Reason + ": " + cond.Message})
					}
				}
			}
			for _, rule := range hr.Spec.Rules {
				for _, br := range rule.BackendRefs {
					if br.Kind != nil && *br.Kind != "Service" {
						continue
					}
					b := flowBackend{Namespace: hr.Namespace, Name: string(br.Name)}
					if br.Namespace != nil {
						b.Namespace = string(*br.Namespace)
					}
					svc, ok := st.services[svcKey(b)]
					if !ok && st.services != nil {
						add(finding{Severity: "error", Code: "httproute-backend-missing", Resource: self, Service: svcKey(b),
							Message: fmt.Sprintf("backend Service %s does not exist", svcKey(b))})
					} else if ok && st.noEndpoints(ctx, c, svc) {
						add(finding{Severity: "warning", Code: "backend-no-endpoints", Resource: self, Service: svcKey(b),
							Message: fmt.Sprintf("Service %s has no ready endpoints", svcKey(b))})
					}
				}
			}
		}
	}

	// --- Gateway API Gateways: status ---
	if gws, err := c.Gateway.GatewayV1().Gateways("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range gws.Items {
			g := &gws.Items[i]
			check(g.Namespace)
			for _, cond := range g.Status.Conditions {
				if (cond.Type == "Accepted" || cond.Type == "Programmed") && cond.Status == metav1.ConditionFalse {
					add(finding{Severity: "error", Code: "gateway-not-programmed",
						Resource: ref{TypeID: typeGateway, Kind: "Gateway", Namespace: g.Namespace, Name: g.Name},
						Message:  cond.Type + " is False: " + cond.Reason + ": " + cond.Message})
				}
			}
		}
	}

	sort.SliceStable(out.Findings, func(i, j int) bool {
		a, b := out.Findings[i], out.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity == "error"
		}
		ka, kb := a.Resource.Namespace+"/"+a.Resource.Name, b.Resource.Namespace+"/"+b.Resource.Name
		if ka != kb {
			return ka < kb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message // map-ordered sources (conflicts, gateways) stay stable across refreshes
	})
	return out
}

// vsDestinations flattens http/tls/tcp route destinations.
func vsDestinations(spec *istioapi.VirtualService) []*istioapi.Destination {
	var out []*istioapi.Destination
	for _, h := range spec.Http {
		for _, rt := range h.Route {
			if rt.Destination != nil {
				out = append(out, rt.Destination)
			}
		}
	}
	for _, t := range spec.Tls {
		for _, rt := range t.Route {
			if rt.Destination != nil {
				out = append(out, rt.Destination)
			}
		}
	}
	for _, t := range spec.Tcp {
		for _, rt := range t.Route {
			if rt.Destination != nil {
				out = append(out, rt.Destination)
			}
		}
	}
	return out
}

func loadMeshState(ctx context.Context, c *k8s.Clients) *meshState {
	st := &meshState{epCache: map[string]int{}}
	if svcs, err := c.Kube.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err == nil {
		st.services = map[string]*corev1.Service{}
		for i := range svcs.Items {
			s := &svcs.Items[i]
			st.services[s.Namespace+"/"+s.Name] = s
		}
	}
	if pods, err := c.Kube.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		st.podsOK = true
		st.pods = map[string][]labels.Set{}
		for i := range pods.Items {
			p := &pods.Items[i]
			st.pods[p.Namespace] = append(st.pods[p.Namespace], labels.Set(p.Labels))
		}
	} else if !apierrors.IsForbidden(err) {
		st.podsOK = true // transient error: do not advertise a permission problem; checks simply find no pods
		st.pods = map[string][]labels.Set{}
		// ponytail: a transient list failure reads as "no pods"; acceptable for a 30 s refresh page
	}
	if ses, err := c.Istio.NetworkingV1().ServiceEntries("").List(ctx, metav1.ListOptions{}); err == nil {
		for _, se := range ses.Items {
			st.ses = append(st.ses, flowServiceEntry{Namespace: se.Namespace, Name: se.Name, Hosts: se.Spec.Hosts})
		}
	}
	if drs, err := c.Istio.NetworkingV1().DestinationRules("").List(ctx, metav1.ListOptions{}); err == nil {
		st.drs = append([]*istionet.DestinationRule{}, drs.Items...) // non-nil even when empty
	}
	if gws, err := c.Istio.NetworkingV1().Gateways("").List(ctx, metav1.ListOptions{}); err == nil {
		st.istioGWs = map[string]*istioapi.Gateway{}
		for _, g := range gws.Items {
			st.istioGWs[g.Namespace+"/"+g.Name] = &g.Spec
		}
	}
	return st
}

// noEndpoints reports whether svc has zero ready endpoints, reading each Service's
// Endpoints once per analysis. ExternalName Services never have endpoints, so they never count.
func (st *meshState) noEndpoints(ctx context.Context, c *k8s.Clients, svc *corev1.Service) bool {
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return false
	}
	key := svc.Namespace + "/" + svc.Name
	n, ok := st.epCache[key]
	if !ok {
		n = readyEndpoints(ctx, c, svc.Namespace, svc.Name)
		st.epCache[key] = n
	}
	return n == 0
}

// subsetDefined reports whether some DestinationRule for Service b defines subset.
// Unknown (DestinationRules not listable) counts as defined so nothing is reported.
func (st *meshState) subsetDefined(b flowBackend, subset string) bool {
	if st.drs == nil {
		return true
	}
	for _, dr := range st.drs {
		h := resolveDestination(dr.Spec.Host, dr.Namespace, 0)
		if h.External || h.Namespace != b.Namespace || h.Name != b.Name {
			continue
		}
		for _, ss := range dr.Spec.Subsets {
			if ss.Name == subset {
				return true
			}
		}
	}
	return false
}

// anyPodMatches reports whether a label selector matches a pod in ns ("" = any namespace).
func (st *meshState) anyPodMatches(ns string, sel map[string]string) bool {
	s := labels.SelectorFromSet(labels.Set(sel))
	check := func(sets []labels.Set) bool {
		for _, ls := range sets {
			if s.Matches(ls) {
				return true
			}
		}
		return false
	}
	if ns != "" {
		return check(st.pods[ns])
	}
	for _, sets := range st.pods {
		if check(sets) {
			return true
		}
	}
	return false
}
