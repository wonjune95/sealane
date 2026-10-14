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

// statusOf condenses a resource's status into "", "ok", "error:<Reason>" or
// "warning:<Reason>" for the list pill. Gateway API: parents[].conditions /
// conditions; Istio: status.validationMessages (present when istiod analysis is on).
func statusOf(typeID string, u *unstructured.Unstructured) string {
	switch {
	case strings.HasSuffix(typeID, ".gateway.networking.k8s.io"):
		var conds []any
		if typeID == "gateways.gateway.networking.k8s.io" || typeID == "gatewayclasses.gateway.networking.k8s.io" {
			conds, _, _ = unstructured.NestedSlice(u.Object, "status", "conditions")
		} else {
			parents, _, _ := unstructured.NestedSlice(u.Object, "status", "parents")
			for _, p := range parents {
				if pm, ok := p.(map[string]any); ok {
					c, _, _ := unstructured.NestedSlice(pm, "conditions")
					conds = append(conds, c...)
				}
			}
		}
		if len(conds) == 0 {
			return ""
		}
		for _, c := range conds {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			typ, _, _ := unstructured.NestedString(cm, "type")
			st, _, _ := unstructured.NestedString(cm, "status")
			if (typ == "Accepted" || typ == "ResolvedRefs" || typ == "Programmed") && st == "False" {
				reason, _, _ := unstructured.NestedString(cm, "reason")
				return "error:" + reason
			}
		}
		return "ok"
	case strings.HasSuffix(typeID, ".istio.io"):
		msgs, _, _ := unstructured.NestedSlice(u.Object, "status", "validationMessages")
		worst := ""
		for _, m := range msgs {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			level, _, _ := unstructured.NestedString(mm, "level")
			code, _, _ := unstructured.NestedString(mm, "type", "code")
			if level == "ERROR" {
				return "error:" + code
			}
			if level == "WARNING" && worst == "" {
				worst = "warning:" + code
			}
		}
		return worst
	}
	return ""
}
