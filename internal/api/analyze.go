package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	istioapi "istio.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
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
	Path     string `json:"path,omitempty"`    // offending field in Resource, e.g. spec.http[1].route[0].destination.subset
	Value    string `json:"value,omitempty"`   // the offending scalar at Path
}

type analysis struct {
	Findings []finding `json:"findings"`
	Skipped  []string  `json:"skipped,omitempty"` // checks that could not run: "pods" (not listable)
	Checked  int       `json:"checked"`           // routing resources inspected
}

const (
	typeVS        = "virtualservices.networking.istio.io"
	typeDR        = "destinationrules.networking.istio.io"
	typeIstioGW   = "gateways.networking.istio.io"
	typeHTTPRoute = "httproutes.gateway.networking.k8s.io"
	typeGateway   = "gateways.gateway.networking.k8s.io"
)

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, analyzeMesh(r.Context(), clientFrom(r.Context()), r.URL.Query().Get("ns")))
}

// analyzeMesh inspects the whole cluster and returns findings whose Resource
// lives in ns (ns == "" → all). Cross-namespace references are always resolved
// cluster-wide; only the reporting scope is filtered.
func analyzeMesh(ctx context.Context, c *k8s.Clients, ns string) analysis {
	return analyzeSnapshot(loadMeshSnapshot(ctx, c, fullSnapshot), ns)
}

// analyzeSnapshot is analyzeMesh over an already-loaded snapshot. It needs a
// fullSnapshot: a nil kind reads as "could not be listed" and its checks are skipped.
func analyzeSnapshot(st *meshSnapshot, ns string) analysis {
	out := analysis{Findings: []finding{}}
	if st.pods == nil {
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
	var gwIndex map[string]bool // ns/name of Istio Gateways; nil when they cannot be listed
	if st.istioGWs != nil {
		gwIndex = map[string]bool{}
		for _, g := range st.istioGWs {
			gwIndex[g.Namespace+"/"+g.Name] = true
		}
	}
	for _, v := range st.vss {
		self := ref{TypeID: typeVS, Kind: "VirtualService", Namespace: v.Namespace, Name: v.Name}
		check(v.Namespace)
		for gi, g := range v.Spec.Gateways {
			if g == "mesh" {
				continue
			}
			full := istioGatewayKey(g, v.Namespace)
			if gwIndex != nil && !gwIndex[full] {
				add(finding{Severity: "error", Code: "vs-gateway-missing", Resource: self,
					Path: fmt.Sprintf("spec.gateways[%d]", gi), Value: g,
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
				fqdn := b.Name + "." + b.Namespace + ".svc.cluster.local"
				if serviceEntryFor(st.ses, d.Host) != "" || serviceEntryFor(st.ses, fqdn) != "" {
					continue // VM / multicluster host backed by a ServiceEntry
				}
				add(finding{Severity: "error", Code: "vs-host-missing", Resource: self, Service: svcKey(b),
					Path: d.path + ".host", Value: d.Host,
					Message: fmt.Sprintf("destination %q is not a Service (looked for %s)", d.Host, svcKey(b))})
				continue
			}
			if ok && st.noEndpoints(svc) {
				add(finding{Severity: "warning", Code: "backend-no-endpoints", Resource: self, Service: svcKey(b),
					Message: fmt.Sprintf("Service %s has no ready endpoints", svcKey(b))})
			}
			if d.Subset != "" && !st.subsetDefined(b, d.Subset) {
				add(finding{Severity: "error", Code: "vs-subset-undefined", Resource: self, Service: svcKey(b),
					Path: d.path + ".subset", Value: d.Subset,
					Message: fmt.Sprintf("subset %q is not defined in any DestinationRule for %s", d.Subset, svcKey(b))})
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
					Path: "spec.host", Value: host,
					Message: fmt.Sprintf("host %q is not a Service (looked for %s) and no ServiceEntry covers it", host, svcKey(b))})
			}
			continue
		}
		if st.pods == nil || len(svc.Spec.Selector) == 0 {
			continue
		}
		for si, ss := range dr.Spec.Subsets {
			sel := map[string]string{}
			for k, v := range svc.Spec.Selector {
				sel[k] = v
			}
			for k, v := range ss.Labels {
				sel[k] = v
			}
			if !st.anyPodMatches(svc.Namespace, sel) {
				add(finding{Severity: "warning", Code: "dr-subset-no-pods", Resource: self, Service: svcKey(b),
					Path: fmt.Sprintf("spec.subsets[%d].name", si), Value: ss.Name,
					Message: fmt.Sprintf("subset %q selects no pod (%s)", ss.Name, labels.Set(sel).String())})
			}
		}
	}

	// --- Istio Gateways: selector → pods ---
	for _, g := range st.istioGWs {
		check(g.Namespace)
		if st.pods != nil && len(g.Spec.Selector) > 0 && !st.anyPodMatches("", g.Spec.Selector) {
			add(finding{Severity: "warning", Code: "gateway-selector-no-pods",
				Resource: ref{TypeID: typeIstioGW, Kind: "Gateway", Namespace: g.Namespace, Name: g.Name},
				Path:     "spec.selector", Value: labels.Set(g.Spec.Selector).String(),
				Message: fmt.Sprintf("selector %s matches no pod in any namespace", labels.Set(g.Spec.Selector).String())})
		}
	}

	// --- HTTPRoutes: status, backends ---
	for i := range st.hrs {
		hr := &st.hrs[i]
		self := ref{TypeID: typeHTTPRoute, Kind: "HTTPRoute", Namespace: hr.Namespace, Name: hr.Name}
		check(hr.Namespace)
		for _, p := range hr.Status.Parents {
			for _, cond := range p.Conditions {
				if (cond.Type == "Accepted" || cond.Type == "ResolvedRefs") && cond.Status == metav1.ConditionFalse {
					add(finding{Severity: "error", Code: "httproute-not-accepted", Resource: self,
						Message: condText(cond)})
				}
			}
		}
		for ri, rule := range hr.Spec.Rules {
			for bi, br := range rule.BackendRefs {
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
						Path: fmt.Sprintf("spec.rules[%d].backendRefs[%d].name", ri, bi), Value: string(br.Name),
						Message: fmt.Sprintf("backend Service %s does not exist", svcKey(b))})
				} else if ok && st.noEndpoints(svc) {
					add(finding{Severity: "warning", Code: "backend-no-endpoints", Resource: self, Service: svcKey(b),
						Message: fmt.Sprintf("Service %s has no ready endpoints", svcKey(b))})
				}
			}
		}
	}

	// --- Gateway API Gateways: status ---
	for i := range st.gws {
		g := &st.gws[i]
		check(g.Namespace)
		for _, cond := range g.Status.Conditions {
			if (cond.Type == "Accepted" || cond.Type == "Programmed") && cond.Status == metav1.ConditionFalse {
				add(finding{Severity: "error", Code: "gateway-not-programmed",
					Resource: ref{TypeID: typeGateway, Kind: "Gateway", Namespace: g.Namespace, Name: g.Name},
					Message:  cond.Type + " is False: " + condText(cond)})
			}
		}
	}

	// --- Namespaces: injection on, running pods without a sidecar ---
	// A Namespace is cluster-scoped (Resource.Namespace ""), so it is scoped by name, not through add.
	bare := map[string][]string{}
	for _, p := range st.pods {
		if p.phase == string(corev1.PodRunning) && !p.sidecar && !p.injectOptOut {
			bare[p.ns] = append(bare[p.ns], p.name)
		}
	}
	for _, n := range st.namespaces {
		if !n.injection {
			continue
		}
		check(n.name)
		names := bare[n.name]
		if !inScope(n.name) || len(names) == 0 {
			continue // also when pods could not be listed (bare is empty)
		}
		sort.Strings(names)
		msg := fmt.Sprintf("%d running pod(s) without a sidecar in an injection-enabled namespace: %s", len(names), strings.Join(names[:min(3, len(names))], ", "))
		if len(names) > 3 {
			msg += fmt.Sprintf(" +%d", len(names)-3)
		}
		out.Findings = append(out.Findings, finding{Severity: "warning", Code: "namespace-injection-no-sidecar", Message: msg,
			Resource: ref{TypeID: "namespaces", Kind: "Namespace", Namespace: "", Name: n.name}})
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

// condText is "Reason: Message", or the reason alone when the message is empty.
func condText(c metav1.Condition) string {
	if c.Message == "" {
		return c.Reason
	}
	return c.Reason + ": " + c.Message
}

// istioGatewayKey normalises a VirtualService gateway reference to "ns/name".
func istioGatewayKey(g, vsNS string) string {
	if strings.Contains(g, "/") {
		return g
	}
	if name, rest, ok := strings.Cut(g, "."); ok { // legacy "gw.ns.svc.cluster.local" form
		gwNS, _, _ := strings.Cut(rest, ".")
		return gwNS + "/" + name
	}
	return vsNS + "/" + g
}

// vsDest is a route destination plus its field path, e.g. spec.http[0].route[1].destination.
type vsDest struct {
	*istioapi.Destination
	path string
}

// vsDestinations flattens http/tls/tcp route destinations.
func vsDestinations(spec *istioapi.VirtualService) []vsDest {
	var out []vsDest
	add := func(section string, i, j int, d *istioapi.Destination) {
		if d != nil {
			out = append(out, vsDest{d, fmt.Sprintf("spec.%s[%d].route[%d].destination", section, i, j)})
		}
	}
	for i, h := range spec.Http {
		for j, rt := range h.Route {
			add("http", i, j, rt.Destination)
		}
	}
	for i, t := range spec.Tls {
		for j, rt := range t.Route {
			add("tls", i, j, rt.Destination)
		}
	}
	for i, t := range spec.Tcp {
		for j, rt := range t.Route {
			add("tcp", i, j, rt.Destination)
		}
	}
	return out
}

// noEndpoints reports whether svc has zero ready endpoints.
// ExternalName Services never have endpoints, so they never count.
func (st *meshSnapshot) noEndpoints(svc *corev1.Service) bool {
	return svc.Spec.Type != corev1.ServiceTypeExternalName && readyEndpoints(st, svc.Namespace, svc.Name) == 0
}

// subsetDefined reports whether some DestinationRule for Service b defines subset.
// Unknown (DestinationRules not listable) counts as defined so nothing is reported.
func (st *meshSnapshot) subsetDefined(b flowBackend, subset string) bool {
	if st.drs == nil {
		return true
	}
	for _, dr := range st.drs {
		if !hostsMatch([]string{dr.Spec.Host}, b.Name+"."+b.Namespace+".svc.cluster.local", dr.Namespace) {
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
func (st *meshSnapshot) anyPodMatches(ns string, sel map[string]string) bool {
	s := labels.SelectorFromSet(labels.Set(sel))
	for _, p := range st.pods {
		if (ns == "" || p.ns == ns) && s.Matches(p.labels) {
			return true
		}
	}
	return false
}
