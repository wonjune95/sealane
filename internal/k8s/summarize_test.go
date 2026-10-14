package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStatusOf(t *testing.T) {
	cond := func(typ, st, reason string) map[string]any {
		return map[string]any{"type": typ, "status": st, "reason": reason}
	}
	route := func(c ...any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{
			"parents": []any{map[string]any{"conditions": c}}}}}
	}
	msg := func(level string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{
			"validationMessages": []any{map[string]any{"level": level, "type": map[string]any{"code": "IST0101"}}}}}}
	}
	const hr, gw, vs = "httproutes.gateway.networking.k8s.io", "gateways.gateway.networking.k8s.io", "virtualservices.networking.istio.io"
	tests := []struct {
		name, typ, want string
		u               *unstructured.Unstructured
	}{
		{"route false", hr, "error:NoMatchingParent", route(cond("Accepted", "False", "NoMatchingParent"))},
		{"route ok", hr, "ok", route(cond("Accepted", "True", "Accepted"), cond("ResolvedRefs", "True", "ResolvedRefs"))},
		{"route no status", hr, "", &unstructured.Unstructured{Object: map[string]any{}}},
		{"gateway false", gw, "error:Invalid", &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{
			"conditions": []any{cond("Programmed", "False", "Invalid")}}}}},
		{"istio warning", vs, "warning:IST0101", msg("WARNING")},
		{"istio error", vs, "error:IST0101", msg("ERROR")},
		{"istio no status", vs, "", &unstructured.Unstructured{Object: map[string]any{}}},
		{"odd shape", hr, "", &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"parents": "x"}}}},
	}
	for _, tt := range tests {
		if got := statusOf(tt.typ, tt.u); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	obj := func(spec map[string]any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	}
	m := map[string]any{"a": "b"}
	sl := func(v ...any) []any { return v }
	tests := []struct {
		typ, want string
		u         *unstructured.Unstructured
	}{
		{"gateways.networking.istio.io", "hosts: shop.example.com, *.api.example.com", obj(map[string]any{"servers": sl(
			map[string]any{"hosts": sl("ns/shop.example.com", "*.api.example.com")},
			map[string]any{"hosts": sl("./shop.example.com", "shop.example.com")})})},
		{"gateways.networking.istio.io", "", obj(nil)},
		{"peerauthentications.security.istio.io", "mtls: STRICT · selector", obj(map[string]any{
			"mtls": map[string]any{"mode": "STRICT"}, "selector": map[string]any{"matchLabels": m}})},
		{"peerauthentications.security.istio.io", "mtls: PERMISSIVE", obj(map[string]any{"mtls": map[string]any{"mode": "PERMISSIVE"}})},
		{"peerauthentications.security.istio.io", "mtls: (unset)", obj(nil)},
		{"requestauthentications.security.istio.io", "issuers: 2", obj(map[string]any{"jwtRules": sl(m, m)})},
		{"requestauthentications.security.istio.io", "issuer: https://i", obj(map[string]any{"jwtRules": sl(map[string]any{"issuer": "https://i"})})},
		{"requestauthentications.security.istio.io", "", obj(nil)},
		{"sidecars.networking.istio.io", "egress hosts: 3 · selector", obj(map[string]any{
			"egress":           sl(map[string]any{"hosts": sl("./*", "istio-system/*")}, map[string]any{"hosts": sl("a/b")}),
			"workloadSelector": map[string]any{"labels": m}})},
		{"sidecars.networking.istio.io", "", obj(nil)},
		{"workloadentries.networking.istio.io", "address: 10.0.0.1", obj(map[string]any{"address": "10.0.0.1"})},
		{"workloadentries.networking.istio.io", "", obj(nil)},
		{"workloadgroups.networking.istio.io", "labels: 2", obj(map[string]any{"template": map[string]any{"labels": map[string]any{"a": "1", "b": "2"}}})},
		{"workloadgroups.networking.istio.io", "labels: 0", obj(nil)},
		{"envoyfilters.networking.istio.io", "patches: 2 · selector", obj(map[string]any{
			"configPatches": sl(m, m), "workloadSelector": map[string]any{"labels": m}})},
		{"envoyfilters.networking.istio.io", "patches: 1", obj(map[string]any{"configPatches": sl(m)})},
		{"envoyfilters.networking.istio.io", "", obj(nil)},
		{"telemetries.telemetry.istio.io", "tracing, metrics", obj(map[string]any{"metrics": sl(m), "tracing": sl(m), "other": 1})},
		{"telemetries.telemetry.istio.io", "", obj(nil)},
		{"gatewayclasses.gateway.networking.k8s.io", "controller: istio.io/gateway-controller", obj(map[string]any{"controllerName": "istio.io/gateway-controller"})},
		{"gatewayclasses.gateway.networking.k8s.io", "", obj(nil)},
		{"gateways.gateway.networking.k8s.io", "listeners: 2 · a.example.com, b.example.com", obj(map[string]any{"listeners": sl(
			map[string]any{"hostname": "a.example.com"}, map[string]any{"hostname": "b.example.com"})})},
		{"gateways.gateway.networking.k8s.io", "listeners: 1", obj(map[string]any{"listeners": sl(map[string]any{"port": int64(80)})})},
		{"gateways.gateway.networking.k8s.io", "", obj(nil)},
		{"grpcroutes.gateway.networking.k8s.io", "a.example.com", obj(map[string]any{"hostnames": sl("a.example.com")})},
		{"tcproutes.gateway.networking.k8s.io", "parents: 2", obj(map[string]any{"parentRefs": sl(m, m)})},
		{"tlsroutes.gateway.networking.k8s.io", "", obj(nil)},
		{"referencegrants.gateway.networking.k8s.io", "from: HTTPRoute/web → to: Secret", obj(map[string]any{
			"from": sl(map[string]any{"kind": "HTTPRoute", "namespace": "web"}), "to": sl(map[string]any{"kind": "Secret"})})},
		{"referencegrants.gateway.networking.k8s.io", "from: HTTPRoute/web → to: Secret +2", obj(map[string]any{
			"from": sl(map[string]any{"kind": "HTTPRoute", "namespace": "web"}, m),
			"to":   sl(map[string]any{"kind": "Secret"}, m)})},
		{"referencegrants.gateway.networking.k8s.io", "", obj(nil)},
		{"gateways.networking.istio.io", "", obj(map[string]any{"servers": "x"})},
		{"sidecars.networking.istio.io", "", obj(map[string]any{"egress": "x", "workloadSelector": "x"})},
	}
	for _, tt := range tests {
		if got := summarize(tt.typ, tt.u); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.typ, got, tt.want)
		}
	}
}
