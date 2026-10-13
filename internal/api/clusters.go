package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/wonjune95/sealane/internal/k8s"
)

// ClustersSecretRef locates the Secret holding remote-cluster kubeconfigs
// (key = cluster name, value = raw kubeconfig). An empty Name disables multi-cluster
// — only "local" exists. Credentials are stored only in a Secret on the cluster
// Sealane runs in.
type ClustersSecretRef struct{ Namespace, Name string }

// clusterSource resolves the ?cluster= parameter to a per-cluster factory.
// ""/"local" is always the pod's SA (or dev kubeconfig) factory.
// ponytail: the cache is invalidated only on PUT/DELETE — editing the Secret directly with kubectl needs a restart.
func (s *Server) clusterSource(ctx context.Context, name string) (ClientSource, error) {
	if name == "" || name == "local" {
		return s.factory, nil
	}
	if s.clustersSecret.Name == "" {
		return nil, fmt.Errorf("multi-cluster is not configured (CLUSTERS_SECRET)")
	}
	s.clusterMu.Lock()
	defer s.clusterMu.Unlock()
	if f, ok := s.clusterCache[name]; ok {
		return f, nil
	}
	client, err := s.factory.Base()
	if err != nil {
		return nil, err
	}
	sec, err := client.Kube.CoreV1().Secrets(s.clustersSecret.Namespace).
		Get(ctx, s.clustersSecret.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to read clusters Secret: %w", err)
	}
	kc, ok := sec.Data[name]
	if !ok {
		return nil, fmt.Errorf("unknown cluster: %s", name)
	}
	f, err := k8s.NewClientFactoryFromKubeconfig(kc)
	if err != nil {
		return nil, fmt.Errorf("cluster %s: %w", name, err)
	}
	if s.clusterCache == nil {
		s.clusterCache = map[string]ClientSource{}
	}
	s.clusterCache[name] = f
	return f, nil
}

func (s *Server) dropClusterCache(name string) {
	s.clusterMu.Lock()
	delete(s.clusterCache, name)
	s.clusterMu.Unlock()
}

// handleClustersList returns "local" + registered clusters. Every logged-in user
// calls it (header dropdown). A failed Secret read degrades to returning only local.
// With ?status=true it pings each cluster in parallel to fill in connectivity and
// version (for the settings panel).
func (s *Server) handleClustersList(w http.ResponseWriter, r *http.Request) {
	if _, err := s.sessionIdentity(r); err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
		return
	}
	type cluster struct {
		Name      string `json:"name"`
		Connected *bool  `json:"connected,omitempty"`
		Version   string `json:"version,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	withStatus := r.URL.Query().Get("status") == "true"
	list := []cluster{{Name: "local"}}
	if withStatus {
		t := true // for local, serving this response is itself proof of connectivity
		list[0].Connected = &t
	}
	var kubeconfigs map[string][]byte
	if s.clustersSecret.Name != "" {
		if client, err := s.factory.Base(); err == nil {
			sec, err := client.Kube.CoreV1().Secrets(s.clustersSecret.Namespace).
				Get(r.Context(), s.clustersSecret.Name, metav1.GetOptions{})
			if err == nil {
				kubeconfigs = sec.Data
				names := make([]string, 0, len(sec.Data))
				for name := range sec.Data {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, n := range names {
					list = append(list, cluster{Name: n})
				}
			}
		}
	}
	if withStatus {
		var wg sync.WaitGroup
		for i := range list[1:] {
			c := &list[i+1]
			wg.Add(1)
			go func() {
				defer wg.Done()
				connected := false
				c.Connected = &connected
				f, err := k8s.NewClientFactoryFromKubeconfig(kubeconfigs[c.Name])
				if err == nil {
					c.Version, err = f.Ping()
				}
				if err != nil {
					c.Error = err.Error()
					return
				}
				connected = true
			}()
		}
		wg.Wait()
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) clustersGuard(w http.ResponseWriter, r *http.Request) (string, bool) {
	if _, ok := s.adminOnly(w, r); !ok {
		return "", false
	}
	if s.clustersSecret.Name == "" {
		writeError(w, http.StatusInternalServerError, "Internal", "clusters Secret location is not configured (CLUSTERS_SECRET/POD_NAMESPACE)")
		return "", false
	}
	name := r.PathValue("name")
	if name == "local" {
		writeError(w, http.StatusBadRequest, "BadRequest", "the local cluster cannot be modified")
		return "", false
	}
	if !validUsername.MatchString(name) {
		writeError(w, http.StatusBadRequest, "BadRequest", "cluster name must start with a letter or digit and contain only letters, digits, '-', '.', '_' (max 64 characters)")
		return "", false
	}
	return name, true
}

// handleClusterUpsert registers/updates a cluster from pasted kubeconfig.
// It saves even if the connection fails (the cluster may be briefly down) — the response carries the connection result.
func (s *Server) handleClusterUpsert(w http.ResponseWriter, r *http.Request) {
	name, ok := s.clustersGuard(w, r)
	if !ok {
		return
	}
	var in struct {
		Kubeconfig string `json:"kubeconfig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "invalid request body")
		return
	}
	if in.Kubeconfig == "" {
		writeError(w, http.StatusBadRequest, "BadRequest", "kubeconfig is empty")
		return
	}
	f, err := k8s.NewClientFactoryFromKubeconfig([]byte(in.Kubeconfig))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	if err := s.patchClusterSecret(r.Context(), name, []byte(in.Kubeconfig)); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update clusters Secret: "+err.Error())
		return
	}
	s.dropClusterCache(name)
	version, perr := f.Ping()
	out := map[string]any{"connected": perr == nil, "version": version}
	if perr != nil {
		out["error"] = perr.Error()
	}
	slog.Info("cluster upserted", "cluster", name, "connected", perr == nil)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleClusterDelete(w http.ResponseWriter, r *http.Request) {
	name, ok := s.clustersGuard(w, r)
	if !ok {
		return
	}
	if err := s.patchClusterSecret(r.Context(), name, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "failed to update clusters Secret: "+err.Error())
		return
	}
	s.dropClusterCache(name)
	slog.Info("cluster deleted", "cluster", name)
	w.WriteHeader(http.StatusNoContent)
}

// patchClusterSecret merge-patches one Secret key; kubeconfig nil deletes it.
// ([]byte marshals to base64 in JSON, matching the Secret data convention)
func (s *Server) patchClusterSecret(ctx context.Context, name string, kubeconfig []byte) error {
	client, err := s.factory.Base()
	if err != nil {
		return err
	}
	secrets := client.Kube.CoreV1().Secrets(s.clustersSecret.Namespace)
	if kubeconfig != nil {
		if _, err := secrets.Get(ctx, s.clustersSecret.Name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			_, err = secrets.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: s.clustersSecret.Name, Namespace: s.clustersSecret.Namespace},
				Data:       map[string][]byte{name: kubeconfig},
			}, metav1.CreateOptions{})
			return err
		}
	}
	var value any
	if kubeconfig != nil {
		value = kubeconfig
	}
	b, err := json.Marshal(map[string]any{"data": map[string]any{name: value}})
	if err != nil {
		return err
	}
	_, err = secrets.Patch(ctx, s.clustersSecret.Name, types.MergePatchType, b, metav1.PatchOptions{})
	return err
}
