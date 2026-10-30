package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/wonjune95/sealane/internal/k8s"
)

// AuditEntry is one non-dry-run mutation performed through Sealane. Entries go to the
// structured log, to an in-memory ring (auditKeep; what dev mode and a failing store fall
// back to) and, when configured, to the in-cluster change history (history.go).
type AuditEntry struct {
	Time      time.Time `json:"time"`
	Cluster   string    `json:"cluster,omitempty"` // empty means local
	User      string    `json:"user,omitempty"`
	Provider  string    `json:"provider,omitempty"` // local | oidc; same name from both is two people
	Verb      string    `json:"verb"`               // create | update | delete
	TypeID    string    `json:"typeId"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
}

const auditKeep = 200

// handleAudit returns the recent mutation history of every cluster (newest first).
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.historyList(r.Context()))
}

// auditResource audits a generic-engine mutation, flagging high-risk kinds so
// monitoring can alert on them (Gemini review ⑦), and records it in the change
// history with the object before and after (nil when there is none).
func (s *Server) auditResource(ctx context.Context, verb, typeID, ns, name string, dryRun bool, err error, before, after json.RawMessage) {
	if dryRun {
		return
	}
	danger := k8s.IsDangerous(typeID)
	attrs := []any{
		slog.Bool("audit", true),
		slog.Bool("dangerous", danger),
		slog.String("cluster", clusterFrom(ctx)),
		slog.String("user", identityFrom(ctx).User()),
		slog.String("provider", identityFrom(ctx).Provider),
		slog.String("verb", verb),
		slog.String("type", typeID),
		slog.String("namespace", ns),
		slog.String("name", name),
	}
	switch {
	case err != nil:
		slog.Error("resource mutation failed", append(attrs, slog.String("err", err.Error()))...)
	case danger:
		slog.Warn("HIGH-RISK resource mutated", attrs...)
	default:
		slog.Info("resource mutated", attrs...)
	}

	e := AuditEntry{
		Time:      time.Now(),
		Cluster:   clusterFrom(ctx),
		User:      identityFrom(ctx).User(),
		Provider:  identityFrom(ctx).Provider,
		Verb:      verb,
		TypeID:    typeID,
		Namespace: ns,
		Name:      name,
		OK:        err == nil,
	}
	s.auditMu.Lock()
	s.auditLog = append([]AuditEntry{e}, s.auditLog...)
	if len(s.auditLog) > auditKeep {
		s.auditLog = s.auditLog[:auditKeep]
	}
	s.auditMu.Unlock()
	s.recordRevision(ctx, e, before, after)
}
