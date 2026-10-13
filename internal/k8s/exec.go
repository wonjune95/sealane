package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod runs argv inside a container and returns its stdout/stderr.
// argv runs as-is without a shell, so argument injection is impossible.
// Request tester only — the caller builds argv programmatically.
func (f *ClientFactory) ExecInPod(ctx context.Context, ns, pod, container string, argv []string, maxBytes int) (string, string, error) {
	clients, err := f.Base()
	if err != nil {
		return "", "", err
	}
	rest := clients.Kube.CoreV1().RESTClient()
	req := rest.Post().Resource("pods").Namespace(ns).Name(pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   argv,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(f.baseCfg, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to prepare exec: %w", err)
	}
	var out, errOut bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: limitWriter(&out, maxBytes),
		Stderr: limitWriter(&errOut, 8<<10),
	})
	return out.String(), errOut.String(), err
}

// limitWriter drops everything past n bytes (to protect memory even when the response is large).
func limitWriter(w io.Writer, n int) io.Writer { return &capped{w: w, left: n} }

type capped struct {
	w    io.Writer
	left int
}

func (c *capped) Write(p []byte) (int, error) {
	if c.left <= 0 {
		return len(p), nil // drop silently — the caller detects truncation from the size
	}
	if len(p) > c.left {
		p = p[:c.left]
	}
	n, err := c.w.Write(p)
	c.left -= n
	return len(p), err
}
