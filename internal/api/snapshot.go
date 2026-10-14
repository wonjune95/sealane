package api

import (
	"context"
	"sync"

	istionet "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/wonjune95/sealane/internal/k8s"
)

// meshSnapshot is every cluster-wide list the flow map, route check, analysis and
// service view read, loaded concurrently once per request. A list that fails (CRD
// not installed, RBAC, throttling) leaves its field nil — that nil is the field's
// ok flag — and the callers skip that kind; a list that succeeds is non-nil even
// when empty. A kind the caller did not request (snapshotOpts) is nil as well.
type meshSnapshot struct {
	services   map[string]*corev1.Service // ns/name
	endpoints  map[string]int             // ns/name → ready addresses
	pods       []podInfo
	namespaces []nsInfo
	ses        []flowServiceEntry
	drs        []*istionet.DestinationRule
	istioGWs   []*istionet.Gateway
	vss        []*istionet.VirtualService
	hrs        []gwapi.HTTPRoute
	gws        []gwapi.Gateway
}

type podInfo struct {
	ns, name     string
	labels       labels.Set
	sidecar      bool // an istio-proxy container or native-sidecar init container
	injectOptOut bool // sidecar.istio.io/inject: "false" label or annotation
	phase        string
}

type nsInfo struct {
	name      string
	injection bool // istio-injection=enabled, or an istio.io/rev label with no istio-injection label (Istio webhook selectors)
}

func hasProxy(cs []corev1.Container) bool {
	for _, c := range cs {
		if c.Name == "istio-proxy" {
			return true
		}
	}
	return false
}

// snapshotOpts picks the optional kinds; Services, Endpoints, VirtualServices and
// HTTPRoutes are always loaded.
type snapshotOpts struct {
	pods       bool // subset / gateway selectors that match nothing, pods without a sidecar (analysis)
	namespaces bool // sidecar injection labels (analysis)
	drs        bool // DestinationRules
	ses        bool // ServiceEntries
	gateways   bool // Istio and Gateway API Gateways
}

// fullSnapshot is what the analysis needs.
var fullSnapshot = snapshotOpts{pods: true, namespaces: true, drs: true, ses: true, gateways: true}

// loadMeshSnapshot runs every list in its own goroutine; each goroutine writes only its own field.
func loadMeshSnapshot(ctx context.Context, c *k8s.Clients, opts snapshotOpts) *meshSnapshot {
	st := &meshSnapshot{}
	all := metav1.ListOptions{}
	var wg sync.WaitGroup
	wg.Go(func() {
		if l, err := c.Kube.CoreV1().Services("").List(ctx, all); err == nil {
			st.services = make(map[string]*corev1.Service, len(l.Items))
			for i := range l.Items {
				st.services[l.Items[i].Namespace+"/"+l.Items[i].Name] = &l.Items[i]
			}
		}
	})
	wg.Go(func() {
		if l, err := c.Kube.CoreV1().Endpoints("").List(ctx, all); err == nil {
			st.endpoints = make(map[string]int, len(l.Items))
			for _, ep := range l.Items {
				n := 0
				for _, sub := range ep.Subsets {
					n += len(sub.Addresses)
				}
				st.endpoints[ep.Namespace+"/"+ep.Name] = n
			}
		}
	})
	if opts.pods {
		// ponytail: lists every Pod object (full specs) on every refresh; a metadata-only
		// list cannot see container names, so a pod informer is the upgrade if large clusters feel it.
		wg.Go(func() {
			if l, err := c.Kube.CoreV1().Pods("").List(ctx, all); err == nil {
				st.pods = make([]podInfo, 0, len(l.Items))
				for i := range l.Items {
					p := &l.Items[i]
					st.pods = append(st.pods, podInfo{ns: p.Namespace, name: p.Name, labels: labels.Set(p.Labels),
						sidecar:      hasProxy(p.Spec.Containers) || hasProxy(p.Spec.InitContainers), // native sidecars are init containers
						injectOptOut: p.Labels["sidecar.istio.io/inject"] == "false" || p.Annotations["sidecar.istio.io/inject"] == "false",
						phase:        string(p.Status.Phase)})
				}
			}
		})
	}
	if opts.namespaces {
		wg.Go(func() {
			if l, err := c.Kube.CoreV1().Namespaces().List(ctx, all); err == nil {
				st.namespaces = make([]nsInfo, 0, len(l.Items))
				for _, n := range l.Items {
					_, rev := n.Labels["istio.io/rev"]
					inj, hasInj := n.Labels["istio-injection"]
					st.namespaces = append(st.namespaces, nsInfo{name: n.Name, injection: inj == "enabled" || (!hasInj && rev)})
				}
			}
		})
	}
	if opts.ses {
		wg.Go(func() {
			if l, err := c.Istio.NetworkingV1().ServiceEntries("").List(ctx, all); err == nil {
				st.ses = []flowServiceEntry{}
				for _, se := range l.Items {
					st.ses = append(st.ses, flowServiceEntry{TypeID: "serviceentries.networking.istio.io",
						Namespace: se.Namespace, Name: se.Name, Hosts: append([]string{}, se.Spec.Hosts...)})
				}
			}
		})
	}
	if opts.drs {
		wg.Go(func() {
			if l, err := c.Istio.NetworkingV1().DestinationRules("").List(ctx, all); err == nil {
				st.drs = append([]*istionet.DestinationRule{}, l.Items...)
			}
		})
	}
	if opts.gateways {
		wg.Go(func() {
			if l, err := c.Istio.NetworkingV1().Gateways("").List(ctx, all); err == nil {
				st.istioGWs = append([]*istionet.Gateway{}, l.Items...)
			}
		})
	}
	wg.Go(func() {
		if l, err := c.Istio.NetworkingV1().VirtualServices("").List(ctx, all); err == nil {
			st.vss = append([]*istionet.VirtualService{}, l.Items...)
		}
	})
	wg.Go(func() {
		if l, err := c.Gateway.GatewayV1().HTTPRoutes("").List(ctx, all); err == nil {
			st.hrs = append([]gwapi.HTTPRoute{}, l.Items...)
		}
	})
	if opts.gateways {
		wg.Go(func() {
			if l, err := c.Gateway.GatewayV1().Gateways("").List(ctx, all); err == nil {
				st.gws = append([]gwapi.Gateway{}, l.Items...)
			}
		})
	}
	wg.Wait()
	return st
}

// readyEndpoints is the ready address count of Service ns/name (0 when unknown).
func readyEndpoints(st *meshSnapshot, ns, name string) int {
	return st.endpoints[ns+"/"+name]
}
