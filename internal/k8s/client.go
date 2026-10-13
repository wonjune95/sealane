// Package k8s builds Kubernetes clients for Sealane.
//
// Auth model: users log in with app-local accounts (internal/auth) and
// the app's role decides what they may do. Every Kubernetes call runs as one
// identity — the pod's ServiceAccount in prod, the local kubeconfig in dev.
package k8s

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	istio "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	gateway "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// Clients bundles the clients a request needs, all built from the same config.
// Dynamic drives the generic resource engine (any registered CRD); the typed
// clients back specific helpers.
type Clients struct {
	Kube    kubernetes.Interface
	Gateway gateway.Interface
	Istio   istio.Interface
	Dynamic dynamic.Interface
}

type ClientFactory struct {
	dev     bool
	baseCfg *rest.Config // dev: local kubeconfig; prod: in-cluster SA
	baseDyn dynamic.Interface

	baseMu      sync.Mutex
	baseClients *Clients // lazily built once from baseCfg

	newDiscovery func() (discovery.DiscoveryInterface, error) // swappable in tests

	catMu       sync.Mutex
	catalog     []ResolvedType // lazily cached; restart to pick up newly installed CRDs
	schemaMu    sync.Mutex
	schemaCache map[string]json.RawMessage // typeID -> spec schema
	istiodMu    sync.Mutex
	istiodVer   *string // lazily cached control-plane version ("" = not detected)
}

func NewClientFactory(dev bool, kubeconfig string) (*ClientFactory, error) {
	var cfg *rest.Config
	var err error
	if dev {
		// Default rules honor $KUBECONFIG and ~/.kube/config; ExplicitPath overrides.
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfig != "" {
			rules.ExplicitPath = kubeconfig
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			rules, &clientcmd.ConfigOverrides{},
		).ClientConfig()
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("load base kube config: %w", err)
	}
	f, err := newFactory(cfg)
	if err != nil {
		return nil, err
	}
	f.dev = dev
	return f, nil
}

// NewClientFactoryFromKubeconfig builds a factory for a remote cluster from raw
// kubeconfig bytes (multi-cluster: the value stored in the clusters Secret).
func NewClientFactoryFromKubeconfig(data []byte) (*ClientFactory, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	return newFactory(cfg)
}

func newFactory(cfg *rest.Config) (*ClientFactory, error) {
	baseDyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("base dynamic client: %w", err)
	}
	f := &ClientFactory{baseCfg: cfg, baseDyn: baseDyn}
	f.newDiscovery = func() (discovery.DiscoveryInterface, error) {
		return discovery.NewDiscoveryClientForConfig(f.baseCfg)
	}
	return f, nil
}

// Ping checks API-server reachability (connection test when registering a cluster) and returns
// the server's Kubernetes version.
func (f *ClientFactory) Ping() (string, error) {
	cfg := rest.CopyConfig(f.baseCfg)
	cfg.Timeout = 5 * time.Second
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return "", err
	}
	v, err := disc.ServerVersion()
	if err != nil {
		return "", err
	}
	return v.GitVersion, nil
}

// Base returns the shared clients built from the base config (SA in prod,
// kubeconfig in dev). App-level roles authorize users; this single identity
// performs all cluster operations.
func (f *ClientFactory) Base() (*Clients, error) {
	f.baseMu.Lock()
	defer f.baseMu.Unlock()
	if f.baseClients != nil {
		return f.baseClients, nil
	}
	kube, err := kubernetes.NewForConfig(f.baseCfg)
	if err != nil {
		return nil, err
	}
	gw, err := gateway.NewForConfig(f.baseCfg)
	if err != nil {
		return nil, err
	}
	is, err := istio.NewForConfig(f.baseCfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(f.baseCfg)
	if err != nil {
		return nil, err
	}
	f.baseClients = &Clients{Kube: kube, Gateway: gw, Istio: is, Dynamic: dyn}
	return f.baseClients, nil
}

// Discovery returns a discovery client on the base config — cluster-scoped CRD
// presence/version checks only.
func (f *ClientFactory) Discovery() (discovery.DiscoveryInterface, error) {
	return f.newDiscovery()
}
