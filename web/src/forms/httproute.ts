import { dump } from 'js-yaml'

// Form-editable subset of an HTTPRoute. Anything outside this subset makes the
// route "not representable" → the Form tab locks and the user edits via YAML.
export interface BackendRefModel { name: string; port: number; weight?: number }
export interface RuleModel {
  pathType: 'PathPrefix' | 'Exact'
  pathValue: string
  backendRefs: BackendRefModel[]
}
export interface ParentRefModel { name: string; namespace?: string; sectionName?: string }
export interface HTTPRouteFormModel {
  name: string
  namespace: string
  parentRefs: ParentRefModel[]
  hostnames: string[]
  rules: RuleModel[]
}

/* eslint-disable @typescript-eslint/no-explicit-any */
type Any = any

export const allowed = (obj: Any, keys: string[]) =>
  Object.keys(obj ?? {}).every((k) => keys.includes(k))

// isHTTPRepresentable reports whether the form can losslessly edit this object's
// spec. Fields the form doesn't model (filters, header matches, multiple matches,
// timeouts, non-Service backends) force YAML-only editing.
export function isHTTPRepresentable(obj: Any): boolean {
  const spec = obj?.spec ?? {}
  for (const p of spec.parentRefs ?? []) {
    // group/kind are API-server defaults on stored objects — representable
    // as long as they still point at a Gateway.
    if (!allowed(p, ['group', 'kind', 'name', 'namespace', 'sectionName'])) return false
    if (p.group != null && p.group !== 'gateway.networking.k8s.io') return false
    if (p.kind != null && p.kind !== 'Gateway') return false
  }
  for (const rule of spec.rules ?? []) {
    if (!allowed(rule, ['matches', 'backendRefs'])) return false
    const matches = rule.matches ?? []
    if (matches.length > 1) return false
    if (matches.length === 1) {
      if (!allowed(matches[0], ['path'])) return false
      if (!allowed(matches[0].path ?? {}, ['type', 'value'])) return false
      const t = matches[0].path?.type
      if (t && t !== 'PathPrefix' && t !== 'Exact') return false
    }
    for (const b of rule.backendRefs ?? []) {
      // Same: tolerate defaulted group/kind, but only plain Service backends.
      if (!allowed(b, ['group', 'kind', 'name', 'port', 'weight'])) return false
      if (b.group != null && b.group !== '') return false
      if (b.kind != null && b.kind !== 'Service') return false
    }
  }
  return true
}

export function toHTTPForm(obj: Any): HTTPRouteFormModel {
  const spec = obj?.spec ?? {}
  return {
    name: obj?.metadata?.name ?? '',
    namespace: obj?.metadata?.namespace ?? 'default',
    parentRefs: (spec.parentRefs ?? []).map((p: Any) => ({
      name: p.name ?? '', namespace: p.namespace, sectionName: p.sectionName,
    })),
    hostnames: spec.hostnames ?? [],
    rules: (spec.rules ?? []).map((r: Any) => ({
      pathType: r.matches?.[0]?.path?.type ?? 'PathPrefix',
      pathValue: r.matches?.[0]?.path?.value ?? '/',
      backendRefs: (r.backendRefs ?? []).map((b: Any) => ({
        name: b.name ?? '', port: b.port ?? 80, weight: b.weight,
      })),
    })),
  }
}

// fromHTTPForm merges the form model back into the base object, preserving fields
// the form doesn't touch (labels, annotations, resourceVersion, …).
export function fromHTTPForm(base: Any, m: HTTPRouteFormModel): Any {
  const obj = structuredClone(base ?? {})
  obj.apiVersion = obj.apiVersion ?? 'gateway.networking.k8s.io/v1'
  obj.kind = 'HTTPRoute'
  obj.metadata = { ...(obj.metadata ?? {}), name: m.name, namespace: m.namespace }
  obj.spec = { ...(obj.spec ?? {}) }
  obj.spec.parentRefs = m.parentRefs.map((p) => ({
    name: p.name,
    ...(p.namespace ? { namespace: p.namespace } : {}),
    ...(p.sectionName ? { sectionName: p.sectionName } : {}),
  }))
  obj.spec.hostnames = m.hostnames.filter((h) => h.trim() !== '')
  obj.spec.rules = m.rules.map((r) => ({
    matches: [{ path: { type: r.pathType, value: r.pathValue } }],
    backendRefs: r.backendRefs.map((b) => ({
      name: b.name, port: b.port, ...(b.weight != null ? { weight: b.weight } : {}),
    })),
  }))
  return obj
}

export function replace<T>(arr: T[], i: number, v: T): T[] {
  return arr.map((x, j) => (j === i ? v : x))
}

export const toYaml = (obj: unknown): string => dump(obj)
