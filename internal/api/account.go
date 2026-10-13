package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"golang.org/x/crypto/bcrypt"

	"github.com/wonjune95/sealane/internal/auth"
)

// EnsureInitialAdmin은 계정 ConfigMap에 admin이 없으면(또는 CM 자체가 없으면)
// 초기 계정 admin/admin을 생성한다. 계정 추가·변경은 이 ConfigMap 편집으로 한다.
// 부팅을 막을 일은 아니므로 실패는 경고 로그로만 남긴다.
func (s *Server) EnsureInitialAdmin(ctx context.Context) {
	if s.dev || s.accountsCM.Name == "" {
		return
	}
	client, err := s.factory.Base()
	if err != nil {
		slog.Warn("initial admin: k8s client unavailable", "err", err)
		return
	}
	cms := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace)
	cm, err := cms.Get(ctx, s.accountsCM.Name, metav1.GetOptions{})
	cmMissing := apierrors.IsNotFound(err)
	switch {
	case err == nil && cm.Data["admin"] != "":
		return // admin 존재 — 할 일 없음
	case err != nil && !cmMissing:
		slog.Warn("initial admin: read accounts configmap failed", "err", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		slog.Warn("initial admin: hash failed", "err", err)
		return
	}
	entry := "admin:" + string(hash)
	if cmMissing {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: s.accountsCM.Name, Namespace: s.accountsCM.Namespace},
			Data:       map[string]string{"admin": entry},
		}, metav1.CreateOptions{})
	} else {
		err = s.patchAccount(ctx, "admin", entry)
	}
	if err != nil {
		slog.Warn("initial admin: write accounts configmap failed", "err", err)
		return
	}
	slog.Info("initial admin account created (admin/admin) — 로그인 후 비밀번호를 변경하세요")
}

// AccountsCMRef locates the accounts ConfigMap so password changes can be
// written back (the mounted dir is read-only from the pod's perspective).
type AccountsCMRef struct{ Namespace, Name string }

// handlePasswordChange lets any logged-in user (viewer 포함) change their own
// password: verify the current one, bcrypt the new one, patch the ConfigMap key.
// The mounted dir syncs within ~1min, so the old password works until then.
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if s.dev {
		writeError(w, http.StatusBadRequest, "BadRequest", "dev 모드에는 계정이 없습니다")
		return
	}
	id, err := s.sessionIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "잘못된 요청 형식입니다")
		return
	}
	if len(in.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "BadRequest", "새 비밀번호는 8자 이상이어야 합니다")
		return
	}
	role, err := s.accounts.Authenticate(id.Name, in.CurrentPassword)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "현재 비밀번호가 일치하지 않습니다")
		return
	}
	if s.accountsCM.Name == "" {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 위치가 설정되지 않았습니다 (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	if err := s.patchAccount(r.Context(), id.Name, role+":"+string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 갱신 실패: "+err.Error())
		return
	}
	s.markPasswordChanged(id.Name)
	slog.Info("password changed", "user", id.Name)
	w.WriteHeader(http.StatusNoContent)
}

// 초기 비밀번호("admin")를 아직 쓰는 계정이면 true — UI가 변경 화면에 고정한다.
// 방금 바꾼 사용자는 마운트 동기화(~1분)를 기다리지 않도록 인메모리로 즉시 풀어준다.
func (s *Server) mustChangePassword(user string) bool {
	if s.dev || user == "" {
		return false
	}
	s.pwChangedMu.Lock()
	changed := s.pwChanged[user]
	s.pwChangedMu.Unlock()
	return !changed && s.accounts.PasswordIs(user, "admin")
}

func (s *Server) markPasswordChanged(user string) {
	s.pwChangedMu.Lock()
	if s.pwChanged == nil {
		s.pwChanged = map[string]bool{}
	}
	s.pwChanged[user] = true
	s.pwChangedMu.Unlock()
}

// patchAccount merge-patches one ConfigMap key; entry nil deletes the key.
func (s *Server) patchAccount(ctx context.Context, user string, entry any) error {
	client, err := s.factory.Base()
	if err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{"data": map[string]any{user: entry}})
	if err != nil {
		return err
	}
	_, err = client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace).
		Patch(ctx, s.accountsCM.Name, types.MergePatchType, b, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("%s/%s: %w", s.accountsCM.Namespace, s.accountsCM.Name, err)
	}
	return nil
}

// --- 계정 관리 (admin 전용): ConfigMap을 API로 노출해 UI에서 편집한다 ---

// ConfigMap 키 제약(-._a-zA-Z0-9)이자 Store의 파일명 제약을 만족하는 사용자명.
var validUsername = regexp.MustCompile(`^[a-zA-Z0-9][-._a-zA-Z0-9]{0,63}$`)

// adminOnly checks dev mode, session and admin role (계정·클러스터 관리 공용).
// 실패 시 이미 응답을 썼으므로 ok=false면 그대로 return.
func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	if s.dev {
		writeError(w, http.StatusBadRequest, "BadRequest", "dev 모드에는 계정이 없습니다")
		return auth.Identity{}, false
	}
	id, err := s.sessionIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", err.Error())
		return auth.Identity{}, false
	}
	if id.Role != "admin" {
		writeError(w, http.StatusForbidden, "Forbidden", "admin만 할 수 있습니다")
		return auth.Identity{}, false
	}
	return id, true
}

// adminGuard = adminOnly + 계정 ConfigMap 설정 확인.
func (s *Server) adminGuard(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, ok := s.adminOnly(w, r)
	if !ok {
		return auth.Identity{}, false
	}
	if s.accountsCM.Name == "" {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 위치가 설정되지 않았습니다 (ACCOUNTS_CONFIGMAP/POD_NAMESPACE)")
		return auth.Identity{}, false
	}
	return id, true
}

func (s *Server) handleAccountsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminGuard(w, r); !ok {
		return
	}
	client, err := s.factory.Base()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", err.Error())
		return
	}
	cm, err := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace).Get(r.Context(), s.accountsCM.Name, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 조회 실패: "+err.Error())
		return
	}
	type account struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	list := make([]account, 0, len(cm.Data))
	for name, entry := range cm.Data {
		role, _, _ := strings.Cut(entry, ":")
		list = append(list, account{Name: name, Role: role})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	writeJSON(w, http.StatusOK, list)
}

// handleAccountUpsert creates or updates an account. 비밀번호를 비우면 기존
// 해시를 유지한 채 역할만 바꾼다. 본인 역할 변경은 잠금 방지를 위해 막는다.
func (s *Server) handleAccountUpsert(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminGuard(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !validUsername.MatchString(name) {
		writeError(w, http.StatusBadRequest, "BadRequest", "사용자명은 영숫자로 시작하는 영숫자·-._ 64자 이내여야 합니다")
		return
	}
	var in struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "BadRequest", "잘못된 요청 형식입니다")
		return
	}
	if !auth.ValidRole(in.Role) {
		writeError(w, http.StatusBadRequest, "BadRequest", "역할은 admin/editor/viewer 중 하나여야 합니다")
		return
	}
	if name == id.Name && in.Role != id.Role {
		writeError(w, http.StatusBadRequest, "BadRequest", "본인 역할은 변경할 수 없습니다")
		return
	}
	var entry string
	if in.Password != "" {
		if len(in.Password) < 8 {
			writeError(w, http.StatusBadRequest, "BadRequest", "비밀번호는 8자 이상이어야 합니다")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", err.Error())
			return
		}
		entry = in.Role + ":" + string(hash)
	} else {
		// 역할만 변경: 기존 해시를 읽어 유지한다.
		client, err := s.factory.Base()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", err.Error())
			return
		}
		cm, err := client.Kube.CoreV1().ConfigMaps(s.accountsCM.Namespace).Get(r.Context(), s.accountsCM.Name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 조회 실패: "+err.Error())
			return
		}
		_, hash, found := strings.Cut(cm.Data[name], ":")
		if !found {
			writeError(w, http.StatusBadRequest, "BadRequest", "새 계정은 비밀번호가 필요합니다")
			return
		}
		entry = in.Role + ":" + hash
	}
	if err := s.patchAccount(r.Context(), name, entry); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 갱신 실패: "+err.Error())
		return
	}
	slog.Info("account upserted", "by", id.Name, "user", name, "role", in.Role)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.adminGuard(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if name == "admin" {
		writeError(w, http.StatusBadRequest, "BadRequest", "admin 계정은 삭제할 수 없습니다")
		return
	}
	if name == id.Name {
		writeError(w, http.StatusBadRequest, "BadRequest", "본인 계정은 삭제할 수 없습니다")
		return
	}
	if err := s.patchAccount(r.Context(), name, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal", "계정 ConfigMap 갱신 실패: "+err.Error())
		return
	}
	slog.Info("account deleted", "by", id.Name, "user", name)
	w.WriteHeader(http.StatusNoContent)
}
