package k8s

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// summarize returns a short per-kind description for the list view. All field
// access goes through unstructured.Nested* helpers (never raw type assertions)
// so a missing/odd field can't panic (Gemini review ⑤).
func summarize(typeID string, u *unstructured.Unstructured) string {
	if f := summarizers[typeID]; f != nil {
		return f(u)
	}
	return ""
}

var summarizers = map[string]func(*unstructured.Unstructured) string{
	"httproutes.gateway.networking.k8s.io": func(u *unstructured.Unstructured) string { return hostsSummary(u, "spec", "hostnames") },
	"virtualservices.networking.istio.io":  func(u *unstructured.Unstructured) string { return hostsSummary(u, "spec", "hosts") },
	"serviceentries.networking.istio.io":   func(u *unstructured.Unstructured) string { return hostsSummary(u, "spec", "hosts") },
	"destinationrules.networking.istio.io": func(u *unstructured.Unstructured) string {
		h, _, _ := unstructured.NestedString(u.Object, "spec", "host")
		return prefix("host", h)
	},
	"authorizationpolicies.security.istio.io": func(u *unstructured.Unstructured) string {
		a, _, _ := unstructured.NestedString(u.Object, "spec", "action")
		return prefix("action", a)
	},
}

func hostsSummary(u *unstructured.Unstructured, fields ...string) string {
	hosts, _, _ := unstructured.NestedStringSlice(u.Object, fields...)
	if len(hosts) == 0 {
		return ""
	}
	if len(hosts) > 3 {
		return strings.Join(hosts[:3], ", ") + " +" + strconv.Itoa(len(hosts)-3)
	}
	return strings.Join(hosts, ", ")
}

func prefix(label, v string) string {
	if v == "" {
		return ""
	}
	return label + ": " + v
}
