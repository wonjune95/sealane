package api

import (
	"net/http"
	"sort"
	"strings"
)

// Config-based ingress traffic flow map: Gateway → Route (VS/HTTPRoute) → Service.
// Draws "where does traffic for this host go" from routing resources alone, without metrics.

type flowBackend struct {
	Namespace    string `json:"namespace,omitempty"`
	Name         string `json:"name"`
	Port         int32  `json:"port,omitempty"`
	External     bool   `json:"external,omitempty"` // host that does not resolve to a cluster service
	Exists       bool   `json:"exists"`
	Endpoints    int    `json:"endpoints"`              // number of ready endpoints (roughly pods)
	ServiceEntry string `json:"serviceEntry,omitempty"` // name of the SE covering the external host
}

type flowRoute struct {
	Kind      string        `json:"kind"` // VirtualService | HTTPRoute
	TypeID    string        `json:"typeId"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Hosts     []string      `json:"hosts"`
	Gateways  []string      `json:"gateways"` // gateway node ids ("ns/name")
	Backends  []flowBackend `json:"backends"`
}

type flowGateway struct {
	Kind      string   `json:"kind"` // Gateway(API) | IstioGateway
	TypeID    string   `json:"typeId"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Hosts     []string `json:"hosts"`
	Egress    bool     `json:"egress,omitempty"` // egress gateway (selector istio=egressgateway heuristic)
}

type flowServiceEntry struct {
	TypeID    string   `json:"typeId"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Hosts     []string `json:"hosts"`
}

type flowMap struct {
	Gateways       []flowGateway      `json:"gateways"`
	Routes         []flowRoute        `json:"routes"`
	ServiceEntries []flowServiceEntry `json:"serviceEntries"`
}

// handleFlowMap builds the graph from live resources. Sources whose CRD is missing
// are skipped silently (multi-cluster: each cluster has a different install).
func (s *Server) handleFlowMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := clientFrom(ctx)
	out := flowMap{Gateways: []flowGateway{}, Routes: []flowRoute{}, ServiceEntries: []flowServiceEntry{}}

	st := loadMeshSnapshot(ctx, c, snapshotOpts{ses: true, gateways: true})

	// Gateways (Gateway API + Istio)
	for i := range st.gws {
		g := &st.gws[i]
		hosts := []string{}
		for _, l := range g.Spec.Listeners {
			if l.Hostname != nil && *l.Hostname != "" {
				hosts = append(hosts, string(*l.Hostname))
			}
		}
		out.Gateways = append(out.Gateways, flowGateway{
			Kind: "Gateway", TypeID: "gateways.gateway.networking.k8s.io",
			Namespace: g.Namespace, Name: g.Name, Hosts: dedupe(hosts),
		})
	}
	for _, g := range st.istioGWs {
		hosts := []string{}
		for _, srv := range g.Spec.Servers {
			for _, h := range srv.Hosts {
				// for "ns/host", keep only host
				if _, after, found := strings.Cut(h, "/"); found {
					h = after
				}
				if h != "*" {
					hosts = append(hosts, h)
				}
			}
		}
		out.Gateways = append(out.Gateways, flowGateway{
			Kind: "IstioGateway", TypeID: "gateways.networking.istio.io",
			Namespace: g.Namespace, Name: g.Name, Hosts: dedupe(hosts),
			Egress: g.Spec.Selector["istio"] == "egressgateway" || strings.Contains(g.Name, "egress"),
		})
	}

	// ServiceEntry — registry of external destinations leaving the mesh (egress flow)
	out.ServiceEntries = append(out.ServiceEntries, st.ses...)

	// HTTPRoute → Gateway API gateways
	for i := range st.hrs {
		hr := &st.hrs[i]
		fr := flowRoute{
			Kind: "HTTPRoute", TypeID: "httproutes.gateway.networking.k8s.io",
			Namespace: hr.Namespace, Name: hr.Name,
			Hosts: []string{}, Gateways: []string{}, Backends: []flowBackend{},
		}
		for _, h := range hr.Spec.Hostnames {
			fr.Hosts = append(fr.Hosts, string(h))
		}
		for _, p := range hr.Spec.ParentRefs {
			if p.Kind != nil && *p.Kind != "Gateway" {
				continue
			}
			ns := hr.Namespace
			if p.Namespace != nil {
				ns = string(*p.Namespace)
			}
			fr.Gateways = append(fr.Gateways, ns+"/"+string(p.Name))
		}
		for _, rule := range hr.Spec.Rules {
			for _, b := range rule.BackendRefs {
				if b.Kind != nil && *b.Kind != "Service" {
					continue
				}
				ns := hr.Namespace
				if b.Namespace != nil {
					ns = string(*b.Namespace)
				}
				var port int32
				if b.Port != nil {
					port = int32(*b.Port)
				}
				fr.Backends = append(fr.Backends, flowBackend{Namespace: ns, Name: string(b.Name), Port: port})
			}
		}
		out.Routes = append(out.Routes, fr)
	}

	// VirtualService → Istio gateways (mesh-only VSs are skipped since they are not ingress flow)
	for _, vs := range st.vss {
		fr := flowRoute{
			Kind: "VirtualService", TypeID: "virtualservices.networking.istio.io",
			Namespace: vs.Namespace, Name: vs.Name,
			Hosts: append([]string{}, vs.Spec.Hosts...), Gateways: []string{}, Backends: []flowBackend{},
		}
		for _, g := range vs.Spec.Gateways {
			if g == "mesh" {
				continue
			}
			if !strings.Contains(g, "/") {
				g = vs.Namespace + "/" + g
			}
			fr.Gateways = append(fr.Gateways, g)
		}
		if len(fr.Gateways) == 0 {
			continue
		}
		for _, d := range vsDestinations(&vs.Spec) {
			var port int32
			if d.Port != nil {
				port = int32(d.Port.Number)
			}
			fr.Backends = append(fr.Backends, resolveDestination(d.Host, vs.Namespace, port))
		}
		out.Routes = append(out.Routes, fr)
	}

	// fill in backend existence + ready endpoint count
	for i := range out.Routes {
		for j := range out.Routes[i].Backends {
			b := &out.Routes[i].Backends[j]
			if b.External {
				b.ServiceEntry = serviceEntryFor(out.ServiceEntries, b.Name)
				continue
			}
			if _, b.Exists = st.services[b.Namespace+"/"+b.Name]; b.Exists {
				b.Endpoints = readyEndpoints(st, b.Namespace, b.Name)
			}
		}
	}

	sort.Slice(out.Gateways, func(i, j int) bool {
		return out.Gateways[i].Namespace+out.Gateways[i].Name < out.Gateways[j].Namespace+out.Gateways[j].Name
	})
	sort.Slice(out.Routes, func(i, j int) bool {
		return out.Routes[i].Namespace+out.Routes[i].Name < out.Routes[j].Namespace+out.Routes[j].Name
	})
	sort.Slice(out.ServiceEntries, func(i, j int) bool {
		return out.ServiceEntries[i].Namespace+out.ServiceEntries[i].Name < out.ServiceEntries[j].Namespace+out.ServiceEntries[j].Name
	})
	writeJSON(w, http.StatusOK, out)
}

// serviceEntryFor returns the SE covering an external host (exact match or
// "*.suffix" wildcard match).
func serviceEntryFor(ses []flowServiceEntry, host string) string {
	for _, se := range ses {
		for _, h := range se.Hosts {
			if h == host {
				return se.Name
			}
			if suffix, ok := strings.CutPrefix(h, "*"); ok && strings.HasSuffix(host, suffix) {
				return se.Name
			}
		}
	}
	return ""
}

// resolveDestination maps a VS destination host to a service or external host.
// "x.ns.svc.cluster.local" → ns/x, short name without dots → service in the same NS, anything else → external.
func resolveDestination(host, ns string, port int32) flowBackend {
	if name, rest, found := strings.Cut(host, "."); found {
		if ns2, ok := strings.CutSuffix(rest, ".svc.cluster.local"); ok {
			return flowBackend{Namespace: ns2, Name: name, Port: port}
		}
		return flowBackend{Name: host, Port: port, External: true}
	}
	return flowBackend{Namespace: ns, Name: host, Port: port}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
