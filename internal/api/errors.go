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
	Code       string `json:"code,omitempty"` // set only on Sealane-authored messages, for UI translation
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, reason, msg string) {
	writeCoded(w, status, reason, "", msg)
}

// writeCoded is writeError for a fixed Sealane-authored message; code is its
// stable slug, which the UI looks up as err.<code> to show a translation.
func writeCoded(w http.ResponseWriter, status int, reason, code, msg string) {
	writeJSON(w, status, errEnvelope{errBody{Reason: reason, Message: msg, HTTPStatus: status, Code: code}})
}

// writeK8sError maps a Kubernetes API error to the Sealane error envelope.
func writeK8sError(w http.ResponseWriter, err error) {
	switch {
	case apierrors.IsConflict(err):
		writeCoded(w, http.StatusConflict, "Conflict", "conflict", "the resource was modified; refresh and retry")
	case apierrors.IsForbidden(err):
		writeError(w, http.StatusForbidden, "Forbidden", err.Error())
	case apierrors.IsUnauthorized(err):
		// The cluster refused Sealane's own credentials (an expired remote token, say). That is not the
		// user's session, so it must not send them to the sign-in page.
		writeCoded(w, http.StatusBadGateway, "ClusterAuthFailed", "cluster-auth-failed", "the cluster rejected Sealane's credentials: "+err.Error())
	case apierrors.IsNotFound(err):
		writeError(w, http.StatusNotFound, "NotFound", err.Error())
	case apierrors.IsInvalid(err):
		writeError(w, http.StatusUnprocessableEntity, "Invalid", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
	}
}
