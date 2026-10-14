import { allowed } from './httproute'

// Form-editable subset of a DestinationRule: host, subsets selected by labels and
// a top-level TLS mode / simple load balancer. Per-subset trafficPolicy,
// connectionPool, outlierDetection, portLevelSettings, … → YAML only.
export const TLS_MODES = ['DISABLE', 'SIMPLE', 'MUTUAL', 'ISTIO_MUTUAL'] as const
export const LB_SIMPLE = ['ROUND_ROBIN', 'LEAST_REQUEST', 'RANDOM', 'PASSTHROUGH'] as const

export interface DRSubsetModel { name: string; labels: { key: string; value: string }[] }
export interface DestinationRuleFormModel {
  name: string
  namespace: string
  host: string
  subsets: DRSubsetModel[]
  tlsMode?: (typeof TLS_MODES)[number]
  lbSimple?: (typeof LB_SIMPLE)[number]
}

/* eslint-disable @typescript-eslint/no-explicit-any */
type Any = any

// A value the selects can't show (e.g. the deprecated LEAST_CONN) locks the form too.
const inEnum = (v: Any, values: readonly string[]) => v == null || values.includes(v)

export function isDRRepresentable(obj: Any): boolean {
  const spec = obj?.spec ?? {}
  if (!allowed(spec, ['host', 'subsets', 'trafficPolicy', 'exportTo'])) return false
  if (!(spec.subsets ?? []).every((s: Any) => allowed(s, ['name', 'labels']))) return false
  const tp = spec.trafficPolicy
  return allowed(tp, ['tls', 'loadBalancer']) &&
    allowed(tp?.tls, ['mode']) && inEnum(tp?.tls?.mode, TLS_MODES) &&
    allowed(tp?.loadBalancer, ['simple']) && inEnum(tp?.loadBalancer?.simple, LB_SIMPLE)
}

export function toDRForm(obj: Any): DestinationRuleFormModel {
  const spec = obj?.spec ?? {}
  return {
    name: obj?.metadata?.name ?? '',
    namespace: obj?.metadata?.namespace ?? 'default',
    host: spec.host ?? '',
    subsets: (spec.subsets ?? []).map((s: Any) => ({
      name: s.name ?? '',
      labels: Object.entries(s.labels ?? {}).map(([key, value]) => ({ key, value })),
    })),
    tlsMode: spec.trafficPolicy?.tls?.mode,
    lbSimple: spec.trafficPolicy?.loadBalancer?.simple,
  }
}

// fromDRForm merges the model into the base object, keeping metadata,
// spec.exportTo and key order. Subset and label rows keep empty strings so a
// freshly added row stays visible until it is filled in.
export function fromDRForm(base: Any, m: DestinationRuleFormModel): Any {
  const obj = structuredClone(base ?? {})
  obj.apiVersion = obj.apiVersion ?? 'networking.istio.io/v1'
  obj.kind = 'DestinationRule'
  obj.metadata = { ...(obj.metadata ?? {}), name: m.name, namespace: m.namespace }
  const tp = Object.fromEntries(Object.entries({
    ...obj.spec?.trafficPolicy,
    tls: m.tlsMode && { mode: m.tlsMode },
    loadBalancer: m.lbSimple && { simple: m.lbSimple },
  }).filter(([, v]) => v))
  obj.spec = {
    ...(obj.spec ?? {}),
    host: m.host,
    subsets: m.subsets.map((s) => ({
      name: s.name,
      // Labels are a map: the first row with a key wins, later duplicates are dropped.
      ...(s.labels.length ? {
        labels: Object.fromEntries(s.labels
          .filter((l, i) => s.labels.findIndex((o) => o.key === l.key) === i)
          .map((l) => [l.key, l.value])),
      } : {}),
    })),
    trafficPolicy: tp,
  }
  if (!m.subsets.length) delete obj.spec.subsets
  if (!Object.keys(tp).length) delete obj.spec.trafficPolicy
  return obj
}
