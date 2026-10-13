package api

import (
	"context"

	"github.com/wonjune95/sealane/internal/auth"
	"github.com/wonjune95/sealane/internal/k8s"
)

// Per-request values injected by withK8s so handlers never touch auth/client
// construction directly (keeps P2/P3 handlers decoupled — see DESIGN.md §12 P1).
type ctxKey int

const (
	clientKey ctxKey = iota
	identityKey
	factoryKey
	clusterKey
)

func withClient(ctx context.Context, c *k8s.Clients) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

func clientFrom(ctx context.Context) *k8s.Clients {
	c, _ := ctx.Value(clientKey).(*k8s.Clients)
	return c
}

func withIdentity(ctx context.Context, id auth.Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

func identityFrom(ctx context.Context) auth.Identity {
	id, _ := ctx.Value(identityKey).(auth.Identity)
	return id
}

// withFactory carries the request's per-cluster factory (?cluster= 해석 결과) so
// 카탈로그/스키마/CRD 조회도 대상 클러스터를 향한다.
func withFactory(ctx context.Context, f ClientSource) context.Context {
	return context.WithValue(ctx, factoryKey, f)
}

func factoryFrom(ctx context.Context) ClientSource {
	f, _ := ctx.Value(factoryKey).(ClientSource)
	return f
}

// withCluster records the non-local cluster name for audit entries.
func withCluster(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, clusterKey, name)
}

func clusterFrom(ctx context.Context) string {
	name, _ := ctx.Value(clusterKey).(string)
	return name
}
