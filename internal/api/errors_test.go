package api

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWriteK8sError_StatusMapping(t *testing.T) {
	gr := schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"conflict", apierrors.NewConflict(gr, "app", errors.New("changed")), 409},
		{"forbidden", apierrors.NewForbidden(gr, "app", errors.New("nope")), 403},
		{"notfound", apierrors.NewNotFound(gr, "app"), 404},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Kind: "HTTPRoute"}, "app", nil), 422},
		{"other", errors.New("boom"), 500},
		{"unauthorized", apierrors.NewUnauthorized("no token"), 502}, // the cluster refused Sealane, not the user's session
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeK8sError(rec, c.err)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
			want := "" // only the Conflict and cluster-auth messages are Sealane's own
			switch c.want {
			case 409:
				want = "conflict"
			case 502:
				want = "cluster-auth-failed"
			}
			if got := errCode(t, rec); got != want {
				t.Fatalf("code = %q, want %q", got, want)
			}
		})
	}
}

// errCode returns the error envelope's code.
func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env errEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v: %s", err, rec.Body.String())
	}
	return env.Error.Code
}
