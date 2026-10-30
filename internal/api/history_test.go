package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func historyServer(t *testing.T) (*http.ServeMux, *Server, *kubefake.Clientset) {
	t.Helper()
	clients, _ := fakeDynamicClients()
	kube := kubefake.NewClientset()
	clients.Kube = kube
	s := NewServer(false, &stubSource{clients: clients}, testStore(t), testSessions, AccountsCMRef{}, ClustersSecretRef{})
	s.SetHistory(HistoryRef{Namespace: "sealane", Name: "sealane-history"})
	mux := http.NewServeMux()
	s.Routes(mux)
	return mux, s, kube
}

func serve(t *testing.T, mux *http.ServeMux, method, path, body string, want int) []byte {
	t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(method, path, rd))
	if rec.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func TestHistory_RecordsChangesWithObjects(t *testing.T) {
	mux, _, kube := historyServer(t)
	base := "/api/resources/" + vsTypeID

	serve(t, mux, "POST", base, vsBody, http.StatusCreated)
	var cur struct{ Raw map[string]any }
	_ = json.Unmarshal(serve(t, mux, "GET", base+"/default/app", "", http.StatusOK), &cur)
	cur.Raw["spec"] = map[string]any{"hosts": []string{"new.example.com"}}
	cur.Raw["metadata"].(map[string]any)["resourceVersion"] = "1" // the fake client sets none; updates need one
	upd, _ := json.Marshal(cur.Raw)
	serve(t, mux, "PUT", base+"/default/app", string(upd), http.StatusOK)
	serve(t, mux, "DELETE", base+"/default/app", "", http.StatusNoContent)

	var list []Revision
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history?type="+vsTypeID+"&ns=default&name=app", "", http.StatusOK), &list)
	if len(list) != 3 || list[0].Verb != "delete" || list[1].Verb != "update" || list[2].Verb != "create" {
		t.Fatalf("history = %+v, want delete, update, create", list)
	}
	if list[0].User != "tester" || list[0].ID == "" {
		t.Errorf("newest = %+v, want an ID and user tester", list[0])
	}
	var audit []Revision // the home page list reads the same store
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/audit", "", http.StatusOK), &audit)
	if len(audit) != 3 || audit[0].ID != list[0].ID {
		t.Errorf("audit = %+v, want the stored history", audit)
	}

	var upRev RevisionDetail
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history/"+list[1].ID, "", http.StatusOK), &upRev)
	if !strings.Contains(string(upRev.Before), "app.example.com") || !strings.Contains(string(upRev.After), "new.example.com") {
		t.Errorf("update before/after = %s / %s", upRev.Before, upRev.After)
	}
	if strings.Contains(string(upRev.After), "resourceVersion") || strings.Contains(string(upRev.After), "managedFields") {
		t.Errorf("stored object keeps server fields: %s", upRev.After)
	}
	var delRev RevisionDetail
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history/"+list[0].ID, "", http.StatusOK), &delRev)
	if !strings.Contains(string(delRev.Before), "new.example.com") || !isNull(delRev.After) {
		t.Errorf("delete before/after = %s / %s, want the last version and no after", delRev.Before, delRev.After)
	}
	var createRev RevisionDetail
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history/"+list[2].ID, "", http.StatusOK), &createRev)
	if !isNull(createRev.Before) || !strings.Contains(string(createRev.After), "app.example.com") {
		t.Errorf("create before/after = %s / %s", createRev.Before, createRev.After)
	}

	// The list ConfigMap and the objects Secret live next to Sealane and carry the history labels.
	cm, err := kube.CoreV1().ConfigMaps("sealane").Get(context.Background(), "sealane-history", metav1.GetOptions{})
	if err != nil || cm.Labels["app.kubernetes.io/component"] != "history" {
		t.Errorf("list: %v %v", err, cm.GetLabels())
	}
	sec, err := kube.CoreV1().Secrets("sealane").Get(context.Background(), "sealane-history-objects", metav1.GetOptions{})
	if err != nil || sec.Labels["app.kubernetes.io/component"] != "history" || len(sec.Data) != 3 {
		t.Errorf("objects: %v %v, %d keys", err, sec.GetLabels(), len(sec.Data))
	}
	// Viewers see who changed what, not the objects.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/history/"+list[1].ID, nil)
	req.AddCookie(&http.Cookie{Name: "sealane_session", Value: testCookie("ro", "viewer")})
	mux.ServeHTTP(rec, req)
	var viewerRev RevisionDetail
	_ = json.Unmarshal(rec.Body.Bytes(), &viewerRev)
	if rec.Code != http.StatusOK || viewerRev.Verb != "update" || !isNull(viewerRev.Before) || !isNull(viewerRev.After) {
		t.Errorf("viewer revision = %d %s, want metadata only", rec.Code, rec.Body.String())
	}
	serve(t, mux, "GET", "/api/history/123", "", http.StatusNotFound)
	// Other resources and clusters are filtered out.
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history?type="+vsTypeID+"&ns=default&name=other", "", http.StatusOK), &list)
	if len(list) != 0 {
		t.Errorf("other resource history = %+v, want none", list)
	}
}

func TestHistory_KeepsNewestEntriesAndBoundsObjects(t *testing.T) {
	_, s, kube := historyServer(t)
	ctx := context.Background()
	start := time.Unix(1_700_000_000, 0)
	big := json.RawMessage(`{"metadata":{"name":"x"},"spec":{"pad":"` + strings.Repeat("a", 10<<10) + `"}}`)
	for i := range historyKeep + 5 {
		s.recordRevision(ctx, AuditEntry{Time: start.Add(time.Duration(i) * time.Second), Verb: "update", TypeID: vsTypeID, Name: "x", OK: true}, big, big)
	}
	list := s.historyList(ctx)
	if len(list) != historyKeep || list[0].Time.Before(list[1].Time) {
		t.Fatalf("kept %d entries (newest first: %v), want %d", len(list), list[0].Time.After(list[1].Time), historyKeep)
	}
	sec, _ := kube.CoreV1().Secrets("sealane").Get(ctx, "sealane-history-objects", metav1.GetOptions{})
	size := 0
	for k, v := range sec.Data {
		size += len(k) + len(v)
	}
	if size > historyObjectsMax || len(sec.Data[list[0].ID]) == 0 {
		t.Errorf("objects = %d bytes in %d keys, newest kept: %v", size, len(sec.Data), len(sec.Data[list[0].ID]) > 0)
	}
	if _, ok := sec.Data[list[len(list)-1].ID]; ok {
		t.Error("oldest revision's objects should have been dropped to stay under the size limit")
	}
}

func TestHistory_OffFallsBackToMemory(t *testing.T) {
	clients, _ := fakeDynamicClients()
	mux := newTestMux(t, &stubSource{clients: clients})
	serve(t, mux, "POST", "/api/resources/"+vsTypeID, vsBody, http.StatusCreated)
	var list []Revision
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/audit", "", http.StatusOK), &list)
	if len(list) != 1 || list[0].ID != "" || list[0].Verb != "create" {
		t.Errorf("memory history = %+v, want one create without an ID", list)
	}
}

// Revisions recorded by 1.15 to 1.18 kept their objects in a ConfigMap; they still open.
func TestHistory_ReadsObjectsFromTheOldConfigMap(t *testing.T) {
	mux, _, kube := historyServer(t)
	ctx := context.Background()
	old := Revision{ID: "1700000000000000000", AuditEntry: AuditEntry{Verb: "update", TypeID: vsTypeID, Name: "x", OK: true}}
	if err := updateHistoryCM(ctx, kube, "sealane", "sealane-history", func(d map[string]string) {
		b, _ := json.Marshal([]Revision{old})
		d[historyListKey] = string(b)
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateHistoryCM(ctx, kube, "sealane", "sealane-history-objects", func(d map[string]string) {
		d[old.ID] = `{"before":{"spec":{"hosts":["old.example.com"]}},"after":null}`
	}); err != nil {
		t.Fatal(err)
	}
	var d RevisionDetail
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history/"+old.ID, "", http.StatusOK), &d)
	if !strings.Contains(string(d.Before), "old.example.com") || !isNull(d.After) {
		t.Errorf("old revision = %s / %s", d.Before, d.After)
	}
}

// Startup moves the old plaintext objects into the Secret and deletes the ConfigMap.
func TestHistory_MigratesLegacyObjects(t *testing.T) {
	mux, s, kube := historyServer(t)
	ctx := context.Background()
	old := Revision{ID: "1700000000000000000", AuditEntry: AuditEntry{Verb: "update", TypeID: vsTypeID, Name: "x", OK: true}}
	_ = updateHistoryCM(ctx, kube, "sealane", "sealane-history", func(d map[string]string) {
		b, _ := json.Marshal([]Revision{old})
		d[historyListKey] = string(b)
	})
	_ = updateHistoryCM(ctx, kube, "sealane", "sealane-history-objects", func(d map[string]string) {
		d[old.ID] = `{"before":{"spec":{"hosts":["old.example.com"]}},"after":null}`
	})
	s.MigrateLegacyHistoryObjects(ctx)
	if _, err := kube.CoreV1().ConfigMaps("sealane").Get(ctx, "sealane-history-objects", metav1.GetOptions{}); err == nil {
		t.Error("the old ConfigMap is still there")
	}
	var d RevisionDetail
	_ = json.Unmarshal(serve(t, mux, "GET", "/api/history/"+old.ID, "", http.StatusOK), &d)
	if !strings.Contains(string(d.Before), "old.example.com") {
		t.Errorf("migrated revision = %s", d.Before)
	}
	s.MigrateLegacyHistoryObjects(ctx) // nothing left to do
}

// A client that hangs up right after its change does not lose the record.
func TestHistory_RecordSurvivesCanceledRequest(t *testing.T) {
	_, s, _ := historyServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.recordRevision(ctx, AuditEntry{Time: time.Now(), Verb: "create", TypeID: vsTypeID, Name: "x", OK: true}, nil, json.RawMessage(`{"a":1}`))
	if list := s.historyList(context.Background()); len(list) != 1 || !strings.Contains(list[0].ID, "-") {
		t.Fatalf("history after a canceled request = %+v, want one entry with a suffixed ID", list)
	}
}

func TestOlderID(t *testing.T) {
	if !olderID("999", "1000") || olderID("1000", "999") || !olderID("1000", "1001") {
		t.Error("IDs must order by numeric value")
	}
}

func isNull(raw json.RawMessage) bool { return raw == nil || string(raw) == "null" }
