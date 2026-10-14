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
