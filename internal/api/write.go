package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wonjune95/sealane/internal/k8s"
)

const maxBodyBytes = 1 << 20 // 1 MiB; route objects are small.

func readBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// writeWriteError maps client-input errors to 400 and otherwise defers to the
// Kubernetes error mapping (409 conflict, 422 invalid, 403 forbidden, …).
func writeWriteError(w http.ResponseWriter, err error) {
	var bre *k8s.BadRequestError
	switch {
	case errors.Is(err, k8s.ErrResourceVersionRequired):
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
	case errors.As(err, &bre):
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
	default:
		writeK8sError(w, err)
	}
}
