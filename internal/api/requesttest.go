package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Live request tester: runs curl inside a pod the user picks to send a real HTTP request.
// The request passes through that pod's sidecar, so you can see how VirtualService
// routing, AuthorizationPolicy and mTLS actually behave.
//
// It is a dangerous feature, so it has three locks: (1) the REQUEST_TESTER env var (Helm opt-in)
// (2) the admin role (3) argv built directly (no shell) + input validation + timeout and size caps.

const (
	reqTestMaxBody     = 64 << 10  // request body cap
	reqTestMaxResponse = 256 << 10 // response cap (excess is truncated)
	reqTestMaxTimeout  = 30
)

var reqTestMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true,
}

type podRef struct {
	Name       string   `json:"name"`
	Containers []string `json:"containers"`
	Mesh       bool     `json:"mesh"` // has an istio-proxy sidecar = routing can be verified
	Ready      bool     `json:"ready"`
}

// handlePods lists pods for the source-pod picker. It responds only when the request tester is enabled.
func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	if !s.requestTesterAllowed(w, r) {
		return
	}
	ctx := r.Context()
	list, err := clientFrom(ctx).Kube.CoreV1().Pods(r.URL.Query().Get("ns")).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	out := make([]podRef, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		if p.Status.Phase != "Running" {
			continue
		}
		ref := podRef{Name: p.Name, Ready: true}
		// Istio 1.29+ injects the sidecar as a native sidecar (an initContainer with
		// restartPolicy=Always) — both lists must be checked to tell mesh membership.
		for _, c := range p.Spec.InitContainers {
			if c.RestartPolicy == nil || *c.RestartPolicy != corev1.ContainerRestartPolicyAlways {
				continue // real init containers have already exited and cannot be exec'd into
			}
			ref.Containers = append(ref.Containers, c.Name)
			if c.Name == "istio-proxy" {
				ref.Mesh = true
			}
		}
		for _, c := range p.Spec.Containers {
			ref.Containers = append(ref.Containers, c.Name)
			if c.Name == "istio-proxy" {
				ref.Mesh = true
			}
		}
		for _, cs := range p.Status.ContainerStatuses {
			if !cs.Ready {
				ref.Ready = false
			}
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// requestTesterAllowed enforces the opt-in flag and the admin role.
func (s *Server) requestTesterAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !s.requestTester {
		writeError(w, http.StatusForbidden, "Forbidden",
			"request tester is disabled (set Helm value requestTester.enabled=true)")
		return false
	}
	if identityFrom(r.Context()).Role != "admin" {
		writeError(w, http.StatusForbidden, "Forbidden", "request tester is admin only")
		return false
	}
	return true
}

type reqTestInput struct {
	Namespace string            `json:"namespace"`
	Pod       string            `json:"pod"`
	Container string            `json:"container"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	TimeoutS  int               `json:"timeoutSeconds"`
}

type reqTestResult struct {
	Status     int               `json:"status,omitempty"`
	StatusText string            `json:"statusText,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	DurationMs int64             `json:"durationMs"`
	Truncated  bool              `json:"truncated,omitempty"`
	Error      string            `json:"error,omitempty"`
	Hint       string            `json:"hint,omitempty"` // interpretation in Istio terms
}

// handleRequestTest sends one HTTP request from inside the chosen pod.
func (s *Server) handleRequestTest(w http.ResponseWriter, r *http.Request) {
	if !s.requestTesterAllowed(w, r) {
		return
	}
	id := identityFrom(r.Context())
	var in reqTestInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, reqTestMaxBody+8<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "invalid request body")
		return
	}
	argv, err := buildCurlArgv(&in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(in.TimeoutS+5)*time.Second)
	defer cancel()
	start := time.Now()
	stdout, stderr, execErr := factoryFrom(r.Context()).
		ExecInPod(ctx, in.Namespace, in.Pod, in.Container, argv, reqTestMaxResponse)
	out := reqTestResult{DurationMs: time.Since(start).Milliseconds()}
	if stdout != "" {
		parseCurlOutput(stdout, &out)
	}
	if execErr != nil && out.Status == 0 {
		out.Error = summarizeExecError(execErr, stderr)
	}
	out.Hint = istioHint(out.Status, out.Body, out.Headers)
	out.Truncated = len(stdout) >= reqTestMaxResponse

	slog.Info("request test", slog.Bool("audit", true), slog.String("user", id.Name),
		slog.String("cluster", clusterFrom(r.Context())), slog.String("pod", in.Namespace+"/"+in.Pod),
		slog.String("method", in.Method), slog.String("url", in.URL), slog.Int("status", out.Status))
	writeJSON(w, http.StatusOK, out)
}

// buildCurlArgv validates the input and builds argv — no shell is involved, so
// values become arguments as-is, and once validated there is no room for injection.
func buildCurlArgv(in *reqTestInput) ([]string, error) {
	if in.Namespace == "" || in.Pod == "" {
		return nil, fmt.Errorf("select a source pod")
	}
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	if in.Method == "" {
		in.Method = "GET"
	}
	if !reqTestMethods[in.Method] {
		return nil, fmt.Errorf("unsupported method: %s", in.Method)
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid URL (example: http://svc.ns:8080/path)")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https are supported")
	}
	if in.TimeoutS <= 0 {
		in.TimeoutS = 10
	}
	if in.TimeoutS > reqTestMaxTimeout {
		in.TimeoutS = reqTestMaxTimeout
	}
	if len(in.Body) > reqTestMaxBody {
		return nil, fmt.Errorf("request body too large (max %dKB)", reqTestMaxBody>>10)
	}
	argv := []string{"curl", "-sS", "-i", "--max-time", fmt.Sprint(in.TimeoutS), "-X", in.Method}
	for _, k := range sortedHeaderKeys(in.Headers) {
		v := in.Headers[k]
		if strings.ContainsAny(k, "\r\n: ") || strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("header name or value contains invalid characters: %s", k)
		}
		argv = append(argv, "-H", k+": "+v)
	}
	if in.Body != "" {
		argv = append(argv, "--data-binary", in.Body)
	}
	argv = append(argv, "--", u.String()) // nothing after "--" is parsed as an option
	return argv, nil
}

func sortedHeaderKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// parseCurlOutput splits "curl -i" output into status line, headers and body.
func parseCurlOutput(raw string, out *reqTestResult) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	head, body, found := strings.Cut(raw, "\n\n")
	if !found {
		out.Body = raw
		return
	}
	lines := strings.Split(head, "\n")
	if len(lines) == 0 {
		out.Body = body
		return
	}
	if parts := strings.SplitN(lines[0], " ", 3); len(parts) >= 2 && strings.HasPrefix(parts[0], "HTTP/") {
		fmt.Sscanf(parts[1], "%d", &out.Status)
		if len(parts) == 3 {
			out.StatusText = parts[2]
		}
	}
	out.Headers = map[string]string{}
	for _, l := range lines[1:] {
		if k, v, ok := strings.Cut(l, ":"); ok {
			out.Headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	out.Body = body
}

// summarizeExecError looks at both stderr and the exec error — a missing curl
// surfaces as kubelet's exec error, not in stderr.
func summarizeExecError(err error, stderr string) string {
	msg := strings.TrimSpace(stderr)
	all := msg + " " + err.Error()
	switch {
	case strings.Contains(all, "executable file not found"), strings.Contains(all, "no such file or directory"):
		return "curl is not available in this container (e.g. distroless image). Pick another pod or container that has curl."
	case strings.Contains(all, "container not found"):
		return "no container with that name."
	case msg != "":
		return msg
	default:
		return err.Error()
	}
}

// istioHint translates common mesh failures into an actionable sentence.
func istioHint(status int, body string, headers map[string]string) string {
	switch {
	case status == 403 && strings.Contains(body, "RBAC: access denied"):
		return "AuthorizationPolicy denied the request."
	case status == 503 && strings.Contains(body, "upstream connect error"):
		return "the sidecar could not connect upstream — mTLS mismatch or no healthy endpoints."
	case status == 404 && viaEnvoy(headers):
		return "the request reached Envoy but no route matched — check host/path (compare with the route check)."
	case status == 0:
		return ""
	case !viaEnvoy(headers):
		return "no Envoy trace in the response — the request may have bypassed the sidecar (pod outside the mesh or direct access)."
	}
	return ""
}

// viaEnvoy reports whether the response came through a sidecar/gateway.
// The server header arrives as "istio-envoy" or "envoy" depending on version and config.
func viaEnvoy(headers map[string]string) bool {
	return strings.Contains(headers["server"], "envoy") || headers["x-envoy-upstream-service-time"] != ""
}
