package api

import (
	"context"
	"net/http"
	"sort"
	"strings"

	istioapi "istio.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/wonjune95/sealane/internal/k8s"
)

// 설정 기반 인그레스 트래픽 흐름도: Gateway → Route(VS/HTTPRoute) → Service.
// 메트릭 없이 라우팅 리소스만으로 "이 호스트로 들어온 트래픽이 어디로 가나"를 그린다.

type flowBackend struct {
	Namespace    string `json:"namespace,omitempty"`
	Name         string `json:"name"`
	Port         int32  `json:"port,omitempty"`
	External     bool   `json:"external,omitempty"` // 클러스터 서비스로 안 풀리는 호스트
	Exists       bool   `json:"exists"`
	Endpoints    int    `json:"endpoints"`              // ready 엔드포인트(대략 파드) 수
	ServiceEntry string `json:"serviceEntry,omitempty"` // 외부 호스트를 커버하는 SE 이름
}

type flowRoute struct {
	Kind      string        `json:"kind"` // VirtualService | HTTPRoute
	TypeID    string        `json:"typeId"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Hosts     []string      `json:"hosts"`
	Gateways  []string      `json:"gateways"` // 게이트웨이 노드 id ("ns/name")
	Backends  []flowBackend `json:"backends"`
}

type flowGateway struct {
	Kind      string   `json:"kind"` // Gateway(API) | IstioGateway
	TypeID    string   `json:"typeId"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Hosts     []string `json:"hosts"`
	Egress    bool     `json:"egress,omitempty"` // 이그레스 게이트웨이 (selector istio=egressgateway 휴리스틱)
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

// handleFlowMap builds the graph from live resources. CRD가 없는 소스는 조용히
// 건너뛴다(멀티클러스터: 클러스터마다 설치가 다르다).
func (s *Server) handleFlowMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := clientFrom(ctx)
	out := flowMap{Gateways: []flowGateway{}, Routes: []flowRoute{}, ServiceEntries: []flowServiceEntry{}}

	// 게이트웨이 (Gateway API + Istio)
	if gws, err := c.Gateway.GatewayV1().Gateways("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range gws.Items {
			g := &gws.Items[i]
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
	}
	if gws, err := c.Istio.NetworkingV1().Gateways("").List(ctx, metav1.ListOptions{}); err == nil {
		for _, g := range gws.Items {
			hosts := []string{}
			for _, srv := range g.Spec.Servers {
				for _, h := range srv.Hosts {
					// "ns/host" 형식이면 host만
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
	}

	// ServiceEntry — 메시에서 나가는 외부 목적지 등록부 (이그레스 흐름)
	if ses, err := c.Istio.NetworkingV1().ServiceEntries("").List(ctx, metav1.ListOptions{}); err == nil {
		for _, se := range ses.Items {
			out.ServiceEntries = append(out.ServiceEntries, flowServiceEntry{
				TypeID:    "serviceentries.networking.istio.io",
				Namespace: se.Namespace, Name: se.Name,
				Hosts: append([]string{}, se.Spec.Hosts...),
			})
		}
	}

	// 서비스 존재 확인용 인덱스
	svcExists := map[string]bool{}
	if svcs, err := c.Kube.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range svcs.Items {
			svcExists[svcs.Items[i].Namespace+"/"+svcs.Items[i].Name] = true
		}
	}

	// HTTPRoute → Gateway API 게이트웨이
	if hrs, err := c.Gateway.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range hrs.Items {
			hr := &hrs.Items[i]
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
	}

	// VirtualService → Istio 게이트웨이 (mesh 전용 VS는 인그레스 흐름이 아니므로 제외)
	if vss, err := c.Istio.NetworkingV1().VirtualServices("").List(ctx, metav1.ListOptions{}); err == nil {
		for _, vs := range vss.Items {
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
			addDest := func(d *istioapi.Destination) {
				if d == nil {
					return
				}
				var port int32
				if d.Port != nil {
					port = int32(d.Port.Number)
				}
				fr.Backends = append(fr.Backends, resolveDestination(d.Host, vs.Namespace, port))
			}
			for _, h := range vs.Spec.Http {
				for _, rt := range h.Route {
					addDest(rt.Destination)
				}
			}
			for _, t := range vs.Spec.Tls {
				for _, rt := range t.Route {
					addDest(rt.Destination)
				}
			}
			for _, t := range vs.Spec.Tcp {
				for _, rt := range t.Route {
					addDest(rt.Destination)
				}
			}
			out.Routes = append(out.Routes, fr)
		}
	}

	// 백엔드 존재 여부 + ready 엔드포인트 수 채우기 (서비스당 1회)
	epCount := map[string]int{}
	for i := range out.Routes {
		for j := range out.Routes[i].Backends {
			b := &out.Routes[i].Backends[j]
			if b.External {
				b.ServiceEntry = serviceEntryFor(out.ServiceEntries, b.Name)
				continue
			}
			key := b.Namespace + "/" + b.Name
			b.Exists = svcExists[key]
			if !b.Exists {
				continue
			}
			if n, ok := epCount[key]; ok {
				b.Endpoints = n
				continue
			}
			b.Endpoints = readyEndpoints(ctx, c, b.Namespace, b.Name)
			epCount[key] = b.Endpoints
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

// serviceEntryFor returns the SE covering an external host (정확히 일치 또는
// "*.suffix" 와일드카드 매치).
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
// "x.ns.svc.cluster.local" → ns/x, 점 없는 짧은 이름 → 같은 NS 서비스, 그 외 → 외부.
func resolveDestination(host, ns string, port int32) flowBackend {
	if name, rest, found := strings.Cut(host, "."); found {
		if ns2, ok := strings.CutSuffix(rest, ".svc.cluster.local"); ok {
			return flowBackend{Namespace: ns2, Name: name, Port: port}
		}
		return flowBackend{Name: host, Port: port, External: true}
	}
	return flowBackend{Namespace: ns, Name: host, Port: port}
}

func readyEndpoints(ctx context.Context, c *k8s.Clients, ns, name string) int {
	ep, err := c.Kube.CoreV1().Endpoints(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return 0
	}
	n := 0
	for _, sub := range ep.Subsets {
		n += len(sub.Addresses)
	}
	return n
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
