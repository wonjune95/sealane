import { allowed } from './httproute'

// Form-editable subset of a VirtualService (HTTP routing only). Anything else
// (tls, tcp, fault, mirror, corsPolicy, header manipulation, delegate, …) makes
// the object "not representable" → the Form tab locks and the user edits YAML.
export interface VSMatchModel {
  uriType?: 'prefix' | 'exact' | 'regex'
  uriValue?: string
  headers: { name: string; type: 'exact' | 'prefix' | 'regex'; value: string }[]
}
export interface VSDestinationModel { host: string; subset?: string; port?: number; weight?: number }
export interface VSRuleModel {
  name?: string
  match: VSMatchModel[]
  route: VSDestinationModel[]
  timeout?: string
  // attempts is optional so `retries: { perTryTimeout }` round-trips unchanged.
  retries?: { attempts?: number; perTryTimeout?: string }
  rewriteUri?: string
}
export interface VirtualServiceFormModel {
  name: string
  namespace: string
  hosts: string[]
  gateways: string[]
  http: VSRuleModel[]
}

/* eslint-disable @typescript-eslint/no-explicit-any */
type Any = any

const STRING_MATCH = ['exact', 'prefix', 'regex']
const oneOf = (v: Any) => allowed(v, STRING_MATCH) && Object.keys(v ?? {}).length === 1

export function isVSRepresentable(obj: Any): boolean {
  const spec = obj?.spec ?? {}
  if (!allowed(spec, ['hosts', 'gateways', 'http', 'exportTo'])) return false
  for (const r of spec.http ?? []) {
    if (!allowed(r, ['name', 'match', 'route', 'timeout', 'retries', 'rewrite'])) return false
    for (const m of r.match ?? []) {
      if (!allowed(m, ['uri', 'headers'])) return false
      if (m.uri != null && !oneOf(m.uri)) return false
      if (!Object.values(m.headers ?? {}).every(oneOf)) return false
    }
    for (const d of r.route ?? []) {
      if (!allowed(d, ['destination', 'weight'])) return false
      if (!allowed(d.destination, ['host', 'subset', 'port'])) return false
      if (!allowed(d.destination?.port, ['number'])) return false
    }
    if (!allowed(r.retries, ['attempts', 'perTryTimeout'])) return false
    if (!allowed(r.rewrite, ['uri'])) return false
  }
  return true
}

const first = (v: Any) => Object.keys(v ?? {})[0]

export function toVSForm(obj: Any): VirtualServiceFormModel {
  const spec = obj?.spec ?? {}
  return {
    name: obj?.metadata?.name ?? '',
    namespace: obj?.metadata?.namespace ?? 'default',
    hosts: spec.hosts ?? [],
    gateways: spec.gateways ?? [],
    http: (spec.http ?? []).map((r: Any) => ({
      name: r.name,
      match: (r.match ?? []).map((m: Any) => ({
        uriType: first(m.uri),
        uriValue: m.uri?.[first(m.uri)],
        headers: Object.entries(m.headers ?? {}).map(([name, v]: [string, Any]) => ({
          name, type: first(v), value: v[first(v)],
        })),
      })),
      route: (r.route ?? []).map((d: Any) => ({
        host: d.destination?.host ?? '',
        subset: d.destination?.subset,
        port: d.destination?.port?.number,
        weight: d.weight,
      })),
      timeout: r.timeout,
      retries: r.retries && { attempts: r.retries.attempts, perTryTimeout: r.retries.perTryTimeout },
      rewriteUri: r.rewrite?.uri,
    })),
  }
}

// Spreads `{ [k]: v }` only when v is set; empty strings count as unset.
const opt = (k: string, v: unknown) => (v == null || v === '' ? {} : { [k]: v })

// fromVSForm merges the model into the base object, keeping metadata and
// spec.exportTo. List rows (hosts, header names, destination hosts) keep empty
// strings so a freshly added row stays visible until it is filled in.
export function fromVSForm(base: Any, m: VirtualServiceFormModel): Any {
  const obj = structuredClone(base ?? {})
  obj.apiVersion = obj.apiVersion ?? 'networking.istio.io/v1'
  obj.kind = 'VirtualService'
  obj.metadata = { ...(obj.metadata ?? {}), name: m.name, namespace: m.namespace }
  // Spread keeps exportTo and the original key order.
  obj.spec = {
    ...(obj.spec ?? {}),
    hosts: m.hosts,
    gateways: m.gateways,
    http: m.http.map((r) => ({
      ...opt('name', r.name),
      ...(r.match.length ? {
        match: r.match.map((x) => ({
          ...(x.uriType ? { uri: { [x.uriType]: x.uriValue ?? '' } } : {}),
          // Headers are a map: the first row with a name wins, later duplicates are
          // dropped rather than overwriting its value.
          ...(x.headers.length ? {
            headers: Object.fromEntries(x.headers
              .filter((h, i) => x.headers.findIndex((o) => o.name === h.name) === i)
              .map((h) => [h.name, { [h.type]: h.value }])),
          } : {}),
        })),
      } : {}),
      ...(r.route.length ? {
        route: r.route.map((d) => ({
          destination: {
            host: d.host,
            ...opt('subset', d.subset),
            ...(d.port != null ? { port: { number: d.port } } : {}),
          },
          ...opt('weight', d.weight),
        })),
      } : {}),
      ...opt('timeout', r.timeout),
      ...(r.retries ? { retries: { ...opt('attempts', r.retries.attempts), ...opt('perTryTimeout', r.retries.perTryTimeout) } } : {}),
      ...(r.rewriteUri ? { rewrite: { uri: r.rewriteUri } } : {}),
    })),
  }
  if (!m.gateways.length) delete obj.spec.gateways
  return obj
}
