package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/wonjune95/sealane/internal/k8s"
)

// AuditEntry is one non-dry-run mutation performed through Sealane.
// Entries live in an in-memory ring (auditKeep) shared by all users — the app is
// deliberately store-less, so this history resets on pod restart;
// the structured audit log below is the durable record.
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

// handleAudit returns the recent mutation history (newest first).
func (s *Server) handleAudit(w http.ResponseWriter, _ *http.Request) {
	s.auditMu.Lock()
	entries := make([]AuditEntry, len(s.auditLog))
	copy(entries, s.auditLog)
	s.auditMu.Unlock()
	writeJSON(w, http.StatusOK, entries)
}

// auditResource audits a generic-engine mutation, flagging high-risk kinds so
// monitoring can alert on them (Gemini review ⑦), and records it in the
// in-memory history served at /api/audit.
func (s *Server) auditResource(ctx context.Context, verb, typeID, ns, name string, dryRun bool, err error) {
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

	s.auditMu.Lock()
	s.auditLog = append([]AuditEntry{{
		Time:      time.Now(),
		Cluster:   clusterFrom(ctx),
		User:      identityFrom(ctx).User(),
		Provider:  identityFrom(ctx).Provider,
		Verb:      verb,
		TypeID:    typeID,
		Namespace: ns,
		Name:      name,
		OK:        err == nil,
	}}, s.auditLog...)
	if len(s.auditLog) > auditKeep {
		s.auditLog = s.auditLog[:auditKeep]
	}
	s.auditMu.Unlock()
}
