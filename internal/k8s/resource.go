package k8s

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// ResourceSummary is the generic list row for any kind.
type ResourceSummary struct {
	TypeID          string `json:"typeId"`
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	Summary         string `json:"summary"`          // per-kind short description (may be empty)
	Status          string `json:"status,omitempty"` // "", "ok", "error:<Reason>", "warning:<Code>"
	Age             string `json:"age"`
	ResourceVersion string `json:"resourceVersion"`
}

type ResourceDetail struct {
	ResourceSummary
	Raw json.RawMessage `json:"raw"`
}

// ResourceProvider does CRUD for any registered kind via the dynamic client.
// Using unstructured (vs typed) preserves every field the user sends — no silent
// drops — and scales to all kinds with one code path.
type ResourceProvider struct {
	dyn dynamic.Interface
	rt  ResolvedType
}

func NewResourceProvider(c *Clients, rt ResolvedType) *ResourceProvider {
	return &ResourceProvider{dyn: c.Dynamic, rt: rt}
}

func (p *ResourceProvider) ri(ns string) dynamic.ResourceInterface {
	if p.rt.Namespaced && ns != "" {
		return p.dyn.Resource(p.rt.GVR).Namespace(ns)
	}
	return p.dyn.Resource(p.rt.GVR)
}

func (p *ResourceProvider) summary(u *unstructured.Unstructured) ResourceSummary {
	return ResourceSummary{
		TypeID:          p.rt.TypeID,
		Kind:            p.rt.Kind,
		Namespace:       u.GetNamespace(),
		Name:            u.GetName(),
		Summary:         summarize(p.rt.TypeID, u),
		Status:          statusOf(p.rt.TypeID, u),
		Age:             age(u.GetCreationTimestamp()),
		ResourceVersion: u.GetResourceVersion(),
	}
}

func (p *ResourceProvider) detail(u *unstructured.Unstructured) (ResourceDetail, error) {
	raw, err := u.MarshalJSON()
	if err != nil {
		return ResourceDetail{}, err
	}
	return ResourceDetail{ResourceSummary: p.summary(u), Raw: raw}, nil
}

func (p *ResourceProvider) List(ctx context.Context, ns string) ([]ResourceSummary, error) {
	list, err := p.ri(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ResourceSummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, p.summary(&list.Items[i]))
	}
	return out, nil
}

func (p *ResourceProvider) Get(ctx context.Context, ns, name string) (ResourceDetail, error) {
	u, err := p.ri(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ResourceDetail{}, err
	}
	return p.detail(u)
}

// fromRaw decodes request bytes via apimachinery's JSON (UseNumber → int64), so
// integer fields like ports/weights are NOT widened to float64 and rejected as
// 422 by the API server (Gemini review ①). Never round-trip through stdlib
// interface{} decoding here.
func fromRaw(raw json.RawMessage) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	if err := u.UnmarshalJSON(raw); err != nil {
		return nil, badRequest(err)
	}
	return u, nil
}

func (p *ResourceProvider) Create(ctx context.Context, raw json.RawMessage, dryRun bool) (ResourceDetail, error) {
	u, err := fromRaw(raw)
	if err != nil {
		return ResourceDetail{}, err
	}
	created, err := p.ri(u.GetNamespace()).Create(ctx, u, createOpts(dryRun))
	if err != nil {
		return ResourceDetail{}, err
	}
	return p.detail(created)
}

func (p *ResourceProvider) Update(ctx context.Context, ns, name string, raw json.RawMessage, dryRun bool) (ResourceDetail, error) {
	u, err := fromRaw(raw)
	if err != nil {
		return ResourceDetail{}, err
	}
	u.SetNamespace(ns)
	u.SetName(name)
	if u.GetResourceVersion() == "" {
		return ResourceDetail{}, ErrResourceVersionRequired
	}
	updated, err := p.ri(ns).Update(ctx, u, updateOpts(dryRun))
	if err != nil {
		return ResourceDetail{}, err
	}
	return p.detail(updated)
}

func (p *ResourceProvider) Delete(ctx context.Context, ns, name string) error {
	return p.ri(ns).Delete(ctx, name, metav1.DeleteOptions{})
}
