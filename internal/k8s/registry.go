package k8s

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Category groups resource types for the UI navigation.
type Category string

const (
	CategoryTraffic    Category = "traffic"
	CategorySecurity   Category = "security"
	CategoryTelemetry  Category = "telemetry"
	CategoryGatewayAPI Category = "gateway-api"
)

// ResourceType describes a manageable CRD kind. Version is resolved at runtime via
// discovery (Istio versions differ), so it is not stored here.
type ResourceType struct {
	Kind       string   `json:"kind"`
	Group      string   `json:"group"`
	Resource   string   `json:"resource"` // plural, lowercase
	Namespaced bool     `json:"namespaced"`
	Category   Category `json:"category"`
}

// TypeID is the unique key (resource.group) used in API paths — disambiguates the
// "Gateway" kind which exists in both Istio and Gateway API.
func (rt ResourceType) TypeID() string { return rt.Resource + "." + rt.Group }

// Registry is the curated, full Istio + Gateway API resource set (user chose full
// scope). The generic engine gives every entry CRUD/YAML; forms are added à la carte.
var Registry = []ResourceType{
	// Istio — traffic management
	{"VirtualService", "networking.istio.io", "virtualservices", true, CategoryTraffic},
	{"DestinationRule", "networking.istio.io", "destinationrules", true, CategoryTraffic},
	{"Gateway", "networking.istio.io", "gateways", true, CategoryTraffic},
	{"ServiceEntry", "networking.istio.io", "serviceentries", true, CategoryTraffic},
	{"Sidecar", "networking.istio.io", "sidecars", true, CategoryTraffic},
	{"WorkloadEntry", "networking.istio.io", "workloadentries", true, CategoryTraffic},
	{"WorkloadGroup", "networking.istio.io", "workloadgroups", true, CategoryTraffic},
	{"EnvoyFilter", "networking.istio.io", "envoyfilters", true, CategoryTraffic},
	// Istio — security
	{"AuthorizationPolicy", "security.istio.io", "authorizationpolicies", true, CategorySecurity},
	{"PeerAuthentication", "security.istio.io", "peerauthentications", true, CategorySecurity},
	{"RequestAuthentication", "security.istio.io", "requestauthentications", true, CategorySecurity},
	// Istio — telemetry
	{"Telemetry", "telemetry.istio.io", "telemetries", true, CategoryTelemetry},
	// Gateway API
	{"GatewayClass", "gateway.networking.k8s.io", "gatewayclasses", false, CategoryGatewayAPI},
	{"Gateway", "gateway.networking.k8s.io", "gateways", true, CategoryGatewayAPI},
	{"HTTPRoute", "gateway.networking.k8s.io", "httproutes", true, CategoryGatewayAPI},
	{"GRPCRoute", "gateway.networking.k8s.io", "grpcroutes", true, CategoryGatewayAPI},
	{"TCPRoute", "gateway.networking.k8s.io", "tcproutes", true, CategoryGatewayAPI},
	{"TLSRoute", "gateway.networking.k8s.io", "tlsroutes", true, CategoryGatewayAPI},
	{"ReferenceGrant", "gateway.networking.k8s.io", "referencegrants", true, CategoryGatewayAPI},
}

// dangerous marks high-risk kinds where a typo can break the whole mesh data
// plane or security posture — the UI requires typed confirmation and audit logs
// flag them (Gemini review ⑦).
var dangerous = map[string]bool{
	"envoyfilters.networking.istio.io":         true,
	"authorizationpolicies.security.istio.io":  true,
	"peerauthentications.security.istio.io":    true,
	"requestauthentications.security.istio.io": true,
	"sidecars.networking.istio.io":             true,
}

// IsDangerous reports whether a TypeID is high-risk.
func IsDangerous(typeID string) bool { return dangerous[typeID] }

// noAutoForm marks kinds whose schema is effectively unbounded (arbitrary
// proto/JSON), so an auto-generated form is meaningless — YAML only (Gemini ⑤).
var noAutoForm = map[string]bool{
	"envoyfilters.networking.istio.io": true,
}

// ResolvedType is a registry entry with its discovered served version + install state.
type ResolvedType struct {
	ResourceType
	TypeID      string                      `json:"typeId"`
	Version     string                      `json:"version,omitempty"`
	Installed   bool                        `json:"installed"`
	Dangerous   bool                        `json:"dangerous"`
	FormCapable bool                        `json:"formCapable"` // schema-driven auto-form possible
	GVR         schema.GroupVersionResource `json:"-"`
}

// CatalogCached returns the discovery-resolved registry, computing it once.
func (f *ClientFactory) CatalogCached() ([]ResolvedType, error) {
	f.catMu.Lock()
	defer f.catMu.Unlock()
	if f.catalog == nil {
		c, err := f.Catalog()
		if err != nil {
			return nil, err
		}
		f.catalog = c
	}
	return f.catalog, nil
}

// ResolveType looks up an installed resource type by its TypeID (resource.group).
func (f *ClientFactory) ResolveType(typeID string) (ResolvedType, error) {
	cat, err := f.CatalogCached()
	if err != nil {
		return ResolvedType{}, err
	}
	for _, rt := range cat {
		if rt.TypeID == typeID {
			if !rt.Installed {
				return ResolvedType{}, fmt.Errorf("resource type not installed: %s", typeID)
			}
			return rt, nil
		}
	}
	return ResolvedType{}, fmt.Errorf("unknown resource type: %s", typeID)
}

// Catalog discovers which registered resources are installed and at what version
// (preferring GA). Uses base/SA discovery — cluster-scoped, not user data.
func (f *ClientFactory) Catalog() ([]ResolvedType, error) {
	d, err := f.Discovery()
	if err != nil {
		return nil, err
	}
	_, lists, err := d.ServerGroupsAndResources()
	if err != nil && lists == nil {
		return nil, err
	}
	served := map[string]map[string]string{} // group -> resource -> best version
	for _, rl := range lists {
		gv, perr := schema.ParseGroupVersion(rl.GroupVersion)
		if perr != nil {
			continue
		}
		for _, r := range rl.APIResources {
			if strings.Contains(r.Name, "/") {
				continue // subresource (e.g. .../status)
			}
			m := served[gv.Group]
			if m == nil {
				m = map[string]string{}
				served[gv.Group] = m
			}
			m[r.Name] = preferVersion(m[r.Name], gv.Version)
		}
	}
	out := make([]ResolvedType, 0, len(Registry))
	for _, rt := range Registry {
		ver := served[rt.Group][rt.Resource]
		out = append(out, ResolvedType{
			ResourceType: rt,
			TypeID:       rt.TypeID(),
			Version:      ver,
			Installed:    ver != "",
			Dangerous:    dangerous[rt.TypeID()],
			FormCapable:  ver != "" && !noAutoForm[rt.TypeID()],
			GVR:          schema.GroupVersionResource{Group: rt.Group, Version: ver, Resource: rt.Resource},
		})
	}
	return out, nil
}

// ManagedResources returns the plural resource names of Registry, sorted and without duplicates:
// what Sealane's RBAC grants (the chart's sealane.managedResources lists the same names).
func ManagedResources() []string {
	seen := map[string]bool{}
	var out []string
	for _, rt := range Registry {
		if !seen[rt.Resource] {
			seen[rt.Resource] = true
			out = append(out, rt.Resource)
		}
	}
	sort.Strings(out)
	return out
}
