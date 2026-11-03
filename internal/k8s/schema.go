package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var crdGVR = schema.GroupVersionResource{
	Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
}

// SpecSchema returns the openAPIV3Schema for a kind's spec, used to auto-generate
// a form (Gemini ③: spec only — metadata/status excluded so round-trip preserves
// resourceVersion). Read from the CRD object via the base/SA client and cached.
// CRD name == TypeID (resource.group).
func (f *ClientFactory) SpecSchema(typeID string) (json.RawMessage, error) {
	rt, err := f.ResolveType(typeID)
	if err != nil {
		return nil, err
	}
	if noAutoForm[typeID] {
		return nil, fmt.Errorf("resource is YAML-only: %s", typeID)
	}

	f.schemaMu.Lock()
	defer f.schemaMu.Unlock()
	if f.schemaCache == nil {
		f.schemaCache = map[string]json.RawMessage{}
	}
	key := typeID + "@" + rt.Version // a CRD upgrade that changes the served version gets its new schema
	if s, ok := f.schemaCache[key]; ok {
		return s, nil
	}

	u, err := f.baseDyn.Resource(crdGVR).Get(context.Background(), typeID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	versions, found, err := unstructured.NestedSlice(u.Object, "spec", "versions")
	if err != nil || !found {
		return nil, fmt.Errorf("crd %s: no versions", typeID)
	}
	for _, v := range versions {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _, _ := unstructured.NestedString(vm, "name"); name != rt.Version {
			continue
		}
		spec, found, err := unstructured.NestedMap(vm, "schema", "openAPIV3Schema", "properties", "spec")
		if err != nil || !found {
			return nil, fmt.Errorf("crd %s/%s: no spec schema", typeID, rt.Version)
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, err
		}
		f.schemaCache[key] = raw
		return raw, nil
	}
	return nil, fmt.Errorf("crd %s: version %s not found", typeID, rt.Version)
}
