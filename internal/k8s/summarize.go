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
	"gateways.networking.istio.io": func(u *unstructured.Unstructured) string {
		var hosts []string
		seen := map[string]bool{}
		for _, h := range itemStrings(u, "servers", "hosts") {
			h = h[strings.LastIndex(h, "/")+1:] // ns/host and ./host -> host
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
		return prefix("hosts", joinHosts(hosts))
	},
	"peerauthentications.security.istio.io": func(u *unstructured.Unstructured) string {
		mode, _, _ := unstructured.NestedString(u.Object, "spec", "mtls", "mode")
		if mode == "" {
			mode = "(unset)"
		}
		return "mtls: " + mode + selectorSuffix(u, "spec", "selector", "matchLabels")
	},
	"requestauthentications.security.istio.io": func(u *unstructured.Unstructured) string {
		rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "jwtRules")
		if len(rules) == 1 {
			if r, ok := rules[0].(map[string]any); ok {
				if iss, _, _ := unstructured.NestedString(r, "issuer"); iss != "" {
					return "issuer: " + iss
				}
			}
		}
		return countSummary("issuers", len(rules))
	},
	"sidecars.networking.istio.io": func(u *unstructured.Unstructured) string {
		s := countSummary("egress hosts", len(itemStrings(u, "egress", "hosts")))
		if s == "" {
			return ""
		}
		return s + selectorSuffix(u, "spec", "workloadSelector", "labels")
	},
	"workloadentries.networking.istio.io": func(u *unstructured.Unstructured) string {
		a, _, _ := unstructured.NestedString(u.Object, "spec", "address")
		return prefix("address", a)
	},
	"workloadgroups.networking.istio.io": func(u *unstructured.Unstructured) string {
		l, _, _ := unstructured.NestedMap(u.Object, "spec", "template", "labels")
		return "labels: " + strconv.Itoa(len(l))
	},
	"envoyfilters.networking.istio.io": func(u *unstructured.Unstructured) string {
		p, _, _ := unstructured.NestedSlice(u.Object, "spec", "configPatches")
		s := countSummary("patches", len(p))
		if s == "" {
			return ""
		}
		return s + selectorSuffix(u, "spec", "workloadSelector", "labels")
	},
	"telemetries.telemetry.istio.io": func(u *unstructured.Unstructured) string {
		var keys []string
		for _, k := range []string{"tracing", "metrics", "accessLogging"} {
			if _, ok, _ := unstructured.NestedFieldNoCopy(u.Object, "spec", k); ok {
				keys = append(keys, k)
			}
		}
		return strings.Join(keys, ", ")
	},
	"gatewayclasses.gateway.networking.k8s.io": func(u *unstructured.Unstructured) string {
		c, _, _ := unstructured.NestedString(u.Object, "spec", "controllerName")
		return prefix("controller", c)
	},
	"gateways.gateway.networking.k8s.io": func(u *unstructured.Unstructured) string {
		ls, _, _ := unstructured.NestedSlice(u.Object, "spec", "listeners")
		s := countSummary("listeners", len(ls))
		var hosts []string
		for _, l := range ls {
			if m, ok := l.(map[string]any); ok {
				if h, _, _ := unstructured.NestedString(m, "hostname"); h != "" {
					hosts = append(hosts, h)
				}
			}
		}
		if h := joinHosts(hosts); h != "" {
			s += " · " + h
		}
		return s
	},
	"grpcroutes.gateway.networking.k8s.io": routeSummary,
	"tcproutes.gateway.networking.k8s.io":  routeSummary,
	"tlsroutes.gateway.networking.k8s.io":  routeSummary,
	"referencegrants.gateway.networking.k8s.io": func(u *unstructured.Unstructured) string {
		from, _, _ := unstructured.NestedSlice(u.Object, "spec", "from")
		to, _, _ := unstructured.NestedSlice(u.Object, "spec", "to")
		if len(from) == 0 || len(to) == 0 {
			return ""
		}
		f, _ := from[0].(map[string]any)
		t, _ := to[0].(map[string]any)
		fk, _, _ := unstructured.NestedString(f, "kind")
		fn, _, _ := unstructured.NestedString(f, "namespace")
		tk, _, _ := unstructured.NestedString(t, "kind")
		s := "from: " + fk + "/" + fn + " → to: " + tk
		if more := len(from) + len(to) - 2; more > 0 { // extra entries on either side
			s += " +" + strconv.Itoa(more)
		}
		return s
	},
}

// routeSummary: hostnames when present, else the parentRefs count.
func routeSummary(u *unstructured.Unstructured) string {
	if h := hostsSummary(u, "spec", "hostnames"); h != "" {
		return h
	}
	p, _, _ := unstructured.NestedSlice(u.Object, "spec", "parentRefs")
	return countSummary("parents", len(p))
}

// itemStrings flattens spec.<list>[].<key> string slices.
func itemStrings(u *unstructured.Unstructured, list, key string) []string {
	items, _, _ := unstructured.NestedSlice(u.Object, "spec", list)
	var out []string
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			v, _, _ := unstructured.NestedStringSlice(m, key)
			out = append(out, v...)
		}
	}
	return out
}

func countSummary(label string, n int) string {
	if n == 0 {
		return ""
	}
	return label + ": " + strconv.Itoa(n)
}

func selectorSuffix(u *unstructured.Unstructured, fields ...string) string {
	if l, _, _ := unstructured.NestedMap(u.Object, fields...); len(l) > 0 {
		return " · selector"
	}
	return ""
}

func hostsSummary(u *unstructured.Unstructured, fields ...string) string {
	hosts, _, _ := unstructured.NestedStringSlice(u.Object, fields...)
	return joinHosts(hosts)
}

func joinHosts(hosts []string) string {
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
