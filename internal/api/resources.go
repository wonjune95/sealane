package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/wonjune95/sealane/internal/k8s"
)

// resourceProvider resolves the typeID (resource.group) to an installed type and
// builds a user-scoped generic provider.
func (s *Server) resourceProvider(ctx context.Context, typeID string) (*k8s.ResourceProvider, error) {
	rt, err := factoryFrom(ctx).ResolveType(typeID)
	if err != nil {
		return nil, err
	}
	return k8s.NewResourceProvider(clientFrom(ctx), rt), nil
}

// nsParam normalizes the path namespace. Cluster-scoped resources carry "-" as a
// placeholder; convert it to "" so the dynamic client doesn't send "-" as a real
// namespace (Gemini review ④).
func nsParam(r *http.Request) string {
	ns := r.PathValue("ns")
	if ns == "-" {
		return ""
	}
	return ns
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typeID := r.PathValue("type")
	p, err := s.resourceProvider(ctx, typeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	items, err := p.List(ctx, r.URL.Query().Get("ns"))
	if err != nil {
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.resourceProvider(ctx, r.PathValue("type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	detail, err := p.Get(ctx, nsParam(r), r.PathValue("name"))
	if err != nil {
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typeID := r.PathValue("type")
	p, err := s.resourceProvider(ctx, typeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == "true"
	// Target from the body, so a rejected create is still audited with the
	// intended namespace/name; on success the server's canonical values win.
	ns, name := rawMeta(raw)
	detail, err := p.Create(ctx, raw, dryRun)
	if err == nil {
		ns, name = detail.Namespace, detail.Name
	}
	s.auditResource(ctx, "create", typeID, ns, name, dryRun, err)
	if err != nil {
		writeWriteError(w, err)
		return
	}
	status := http.StatusCreated
	if dryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, detail)
}

// rawMeta extracts metadata.namespace/name from a request body. Malformed JSON
// yields empty strings; the write path reports the parse error itself.
func rawMeta(raw json.RawMessage) (ns, name string) {
	var m struct {
		Metadata struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Metadata.Namespace, m.Metadata.Name
}

func (s *Server) handleUpdateResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typeID, ns, name := r.PathValue("type"), nsParam(r), r.PathValue("name")
	p, err := s.resourceProvider(ctx, typeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == "true"
	detail, err := p.Update(ctx, ns, name, raw, dryRun)
	s.auditResource(ctx, "update", typeID, ns, name, dryRun, err)
	if err != nil {
		writeWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typeID, ns, name := r.PathValue("type"), nsParam(r), r.PathValue("name")
	p, err := s.resourceProvider(ctx, typeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	err = p.Delete(ctx, ns, name)
	s.auditResource(ctx, "delete", typeID, ns, name, false, err)
	if err != nil {
		writeK8sError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
