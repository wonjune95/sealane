package api

import (
	"encoding/json"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type errEnvelope struct {
	Error errBody `json:"error"`
}

type errBody struct {
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"httpStatus"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, reason, msg string) {
	writeJSON(w, status, errEnvelope{errBody{Reason: reason, Message: msg, HTTPStatus: status}})
}

// writeK8sError maps a Kubernetes API error to the Sealane error envelope.
func writeK8sError(w http.ResponseWriter, err error) {
	switch {
	case apierrors.IsConflict(err):
		writeError(w, http.StatusConflict, "Conflict", "the resource was modified; refresh and retry")
	case apierrors.IsForbidden(err):
		writeError(w, http.StatusForbidden, "Forbidden", err.Error())
	case apierrors.IsUnauthorized(err):
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
	case apierrors.IsNotFound(err):
		writeError(w, http.StatusNotFound, "NotFound", err.Error())
	case apierrors.IsInvalid(err):
		writeError(w, http.StatusUnprocessableEntity, "Invalid", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
	}
}
