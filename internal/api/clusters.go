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
// (key = 클러스터 이름, value = kubeconfig 원문). 빈 Name이면 멀티클러스터 비활성
// — "local"만 존재한다. 자격증명은 대시보드가 있는 클러스터의 Secret에만 저장한다.
type ClustersSecretRef struct{ Namespace, Name string }

// clusterSource resolves the ?cluster= parameter to a per-cluster factory.
// ""/"local"은 항상 파드의 SA(또는 dev kubeconfig) 팩토리다.
// ponytail: 캐시는 PUT/DELETE에서만 무효화 — kubectl로 Secret을 직접 고치면 재시작 필요.
func (s *Server) clusterSource(ctx context.Context, name string) (ClientSource, error) {
	if name == "" || name == "local" {
		return s.factory, nil
	}
	if s.clustersSecret.Name == "" {
		return nil, fmt.Errorf("멀티클러스터가 설정되지 않았습니다 (CLUSTERS_SECRET)")
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
		return nil, fmt.Errorf("클러스터 Secret 조회 실패: %w", err)
	}
	kc, ok := sec.Data[name]
	if !ok {
		return nil, fmt.Errorf("등록되지 않은 클러스터: %s", name)
	}
	f, err := k8s.NewClientFactoryFromKubeconfig(kc)
	if err != nil {
		return nil, fmt.Errorf("클러스터 %s: %w", name, err)
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

// handleClustersList returns "local" + registered clusters. 모든 로그인 사용자가
// 호출한다(헤더 드롭다운). Secret 조회 실패는 local만 반환으로 degrade한다.
// ?status=true면 각 클러스터에 병렬 ping해 연결 상태·버전을 채운다(설정 패널용).
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
		t := true // local은 이 응답을 서빙 중이라는 것 자체가 연결 증명
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
		writeError(w, http.StatusInternalServerError, "Internal", "클러스터 Secret 위치가 설정되지 않았습니다 (CLUSTERS_SECRET/POD_NAMESPACE)")
		return "", false
	}
	name := r.PathValue("name")
	if name == "local" {
		writeError(w, http.StatusBadRequest, "BadRequest", "local 클러스터는 변경할 수 없습니다")
		return "", false
	}
	if !validUsername.MatchString(name) {
		writeError(w, http.StatusBadRequest, "BadRequest", "클러스터 이름은 영숫자로 시작하는 영숫자·-._ 64자 이내여야 합니다")
		return "", false
	}
	return name, true
}

// handleClusterUpsert registers/updates a cluster from pasted kubeconfig.
// 연결 실패해도 저장은 한다(일시적 다운일 수 있음) — 응답에 연결 결과를 담는다.
func (s *Server) handleClusterUpsert(w http.ResponseWriter, r *http.Request) {
	name, ok := s.clustersGuard(w, r)
	if !ok {
		return
	}
	var in struct {
		Kubeconfig string `json:"kubeconfig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "잘못된 요청 형식입니다")
		return
	}
	if in.Kubeconfig == "" {
		writeError(w, http.StatusBadRequest, "BadRequest", "kubeconfig가 비어 있습니다")
		return
	}
	f, err := k8s.NewClientFactoryFromKubeconfig([]byte(in.Kubeconfig))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}
	if err := s.patchClusterSecret(r.Context(), name, []byte(in.Kubeconfig)); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "클러스터 Secret 갱신 실패: "+err.Error())
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
		writeError(w, http.StatusInternalServerError, "Internal", "클러스터 Secret 갱신 실패: "+err.Error())
		return
	}
	s.dropClusterCache(name)
	slog.Info("cluster deleted", "cluster", name)
	w.WriteHeader(http.StatusNoContent)
}

// patchClusterSecret merge-patches one Secret key; kubeconfig nil deletes it.
// ([]byte은 JSON 마샬 시 base64가 되어 Secret data 규약과 일치한다)
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
