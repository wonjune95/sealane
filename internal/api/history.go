package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// Change history kept in the cluster, so it is shared by replicas and survives restarts.
// Two ConfigMaps next to Sealane, both created on first use:
//   - <name>          key "revisions": JSON list of the newest historyKeep entries (who, when, what)
//   - <name>-objects  one key per revision ID: the object before and after the change
//
// The list stays small, so the home page reads it cheaply; objects are only read when someone
// opens a revision and are dropped oldest first once they pass historyObjectsMax, because a
// ConfigMap holds at most 1 MiB. Changes made outside Sealane (kubectl, GitOps) are not recorded.
type HistoryRef struct{ Namespace, Name string }

// Revision is one entry of the change history; ID orders entries (Unix nanoseconds).
type Revision struct {
	ID string `json:"id"`
	AuditEntry
}

// RevisionDetail is a revision with the objects it changed (nil when not kept or not applicable:
// no Before for a create, no After for a delete).
type RevisionDetail struct {
	Revision
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

const (
	historyKeep       = 200
	historyObjectsMax = 768 << 10
	historyListKey    = "revisions"
)

// SetHistory turns on the in-cluster change history (empty Name = in-memory list only, as in dev mode).
func (s *Server) SetHistory(ref HistoryRef) { s.history = ref }

// recordRevision stores one audited change. Failures are logged and never fail the request:
// the change itself already happened.
func (s *Server) recordRevision(ctx context.Context, e AuditEntry, before, after json.RawMessage) {
	if s.history.Name == "" {
		return
	}
	c, err := s.factory.Base()
	if err != nil {
		slog.Warn("change history not saved", "err", err)
		return
	}
	rev := Revision{ID: strconv.FormatInt(e.Time.UnixNano(), 10), AuditEntry: e}
	if e.OK && (before != nil || after != nil) {
		blob, _ := json.Marshal(map[string]json.RawMessage{"before": historyObject(before), "after": historyObject(after)})
		if len(blob) <= historyObjectsMax {
			err := updateHistoryCM(ctx, c.Kube, s.history.Namespace, s.history.Name+"-objects", func(data map[string]string) {
				data[rev.ID] = string(blob)
				pruneHistoryObjects(data)
			})
			if err != nil {
				slog.Warn("change history: objects not saved", "id", rev.ID, "err", err)
			}
		}
	}
	err = updateHistoryCM(ctx, c.Kube, s.history.Namespace, s.history.Name, func(data map[string]string) {
		list := decodeRevisions(data[historyListKey])
		list = append([]Revision{rev}, list...)
		if len(list) > historyKeep {
			list = list[:historyKeep]
		}
		b, _ := json.Marshal(list)
		data[historyListKey] = string(b)
	})
	if err != nil {
		slog.Warn("change history not saved", "id", rev.ID, "err", err)
	}
}

// historyObject drops what the server sets on every write, so a stored version can be loaded
// back into the editor and applied. Annotations stay, kubectl's last-applied included.
func historyObject(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var o map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber() // keep integers exact
	if d.Decode(&o) != nil {
		return nil
	}
	delete(o, "status")
	if md, ok := o["metadata"].(map[string]any); ok {
		for _, k := range []string{"managedFields", "uid", "resourceVersion", "generation", "creationTimestamp", "selfLink"} {
			delete(md, k)
		}
	}
	b, _ := json.Marshal(o)
	return b
}

// pruneHistoryObjects drops the oldest revisions' objects until the rest fit historyObjectsMax.
func pruneHistoryObjects(data map[string]string) {
	ids := make([]string, 0, len(data))
	total := 0
	for id, v := range data {
		ids = append(ids, id)
		total += len(id) + len(v)
	}
	sort.Slice(ids, func(i, j int) bool { return olderID(ids[i], ids[j]) })
	for _, id := range ids {
		if total <= historyObjectsMax {
			return
		}
		total -= len(id) + len(data[id])
		delete(data, id)
	}
}

// IDs are decimal Unix nanoseconds: a shorter one is older, equal lengths compare as text.
func olderID(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func decodeRevisions(s string) []Revision {
	var list []Revision
	_ = json.Unmarshal([]byte(s), &list)
	return list
}

// updateHistoryCM read-modify-writes one ConfigMap, creating it when missing; replicas writing at
// the same time retry on the resourceVersion conflict.
func updateHistoryCM(ctx context.Context, kube kubernetes.Interface, ns, name string, mutate func(map[string]string)) error {
	cms := kube.CoreV1().ConfigMaps(ns)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm, err := cms.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{
					"app.kubernetes.io/name": "sealane", "app.kubernetes.io/component": "history",
				}},
				Data: map[string]string{},
			}
			mutate(cm.Data)
			_, err = cms.Create(ctx, cm, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) { // another replica created it first: retry as an update
				return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, name, err)
			}
			return err
		}
		if err != nil {
			return err
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		mutate(cm.Data)
		_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
		return err
	})
}

// historyList returns the stored history (newest first), or the in-memory one when the store is
// off or cannot be read.
func (s *Server) historyList(ctx context.Context) []Revision {
	if s.history.Name != "" {
		if c, err := s.factory.Base(); err == nil {
			cm, err := c.Kube.CoreV1().ConfigMaps(s.history.Namespace).Get(ctx, s.history.Name, metav1.GetOptions{})
			switch {
			case err == nil:
				return decodeRevisions(cm.Data[historyListKey])
			case apierrors.IsNotFound(err):
				return []Revision{}
			default:
				slog.Warn("change history unreadable, showing this pod's memory", "err", err)
			}
		}
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	out := make([]Revision, len(s.auditLog))
	for i, e := range s.auditLog {
		out[i] = Revision{AuditEntry: e}
	}
	return out
}

// GET /api/history?type=&ns=&name= — revisions of one resource in the current cluster (newest first).
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cluster := clusterFrom(r.Context())
	out := []Revision{}
	for _, rev := range s.historyList(r.Context()) {
		if rev.Cluster == cluster && rev.TypeID == q.Get("type") && rev.Namespace == q.Get("ns") && rev.Name == q.Get("name") {
			out = append(out, rev)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/history/{id} — one revision with its objects (null when no longer kept).
func (s *Server) handleRevision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var d *RevisionDetail
	for _, rev := range s.historyList(r.Context()) {
		if rev.ID == id && id != "" {
			d = &RevisionDetail{Revision: rev}
			break
		}
	}
	if d == nil {
		writeCoded(w, http.StatusNotFound, "NotFound", "revision-not-found", "no such revision: "+id)
		return
	}
	if c, err := s.factory.Base(); err == nil && s.history.Name != "" {
		cm, err := c.Kube.CoreV1().ConfigMaps(s.history.Namespace).Get(r.Context(), s.history.Name+"-objects", metav1.GetOptions{})
		if err == nil && cm.Data[id] != "" {
			var o struct{ Before, After json.RawMessage }
			if json.Unmarshal([]byte(cm.Data[id]), &o) == nil {
				d.Before, d.After = nullToNil(o.Before), nullToNil(o.After)
			}
		}
	}
	writeJSON(w, http.StatusOK, d)
}

func nullToNil(raw json.RawMessage) json.RawMessage {
	if string(raw) == "null" {
		return nil
	}
	return raw
}
