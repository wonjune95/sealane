import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'
import { t } from '../i18n'

// Reference resolution for auto-form fields (host/gateway/subset): turns opaque
// text fields into suggest-as-you-type dropdowns linked to real cluster resources,
// with existence warnings — without forcing the value (free text stays valid, so
// external hosts / cross-ns refs don't break). Datalist, not <select>.
// 'gateway'     — Istio-style string ref (VS spec.gateways): cross-ns uses "ns/name"
// 'gatewayName' — Gateway API parentRefs[].name: bare name only (ns is a sibling field)
export type RefKind = 'service' | 'gateway' | 'gatewayName' | 'subset'

export interface RefOption {
  value: string // canonical value inserted/matched
  label: string // shown in the datalist
  aliases?: string[] // other accepted spellings (FQDN variants, ns/name)
  navTypeId?: string // resource type to "go to" (none for core Services)
  navNs?: string
  navName?: string
  host?: string // subset scoping
}

interface ServiceRef { namespace: string; name: string; ports: number[] }
interface GatewayRef { type: 'gateway-api' | 'istio'; namespace: string; name: string }
interface SubsetRef { namespace: string; dr: string; host: string; name: string }

const ENDPOINT: Record<RefKind, string> = {
  service: 'services',
  gateway: 'gateways',
  gatewayName: 'gateways',
  subset: 'subsets',
}

const ISTIO_GW = 'gateways.networking.istio.io'
const GWAPI_GW = 'gateways.gateway.networking.k8s.io'
const DR_TYPE = 'destinationrules.networking.istio.io'

function normalize(kind: RefKind, raw: unknown, ns: string): RefOption[] {
  if (!Array.isArray(raw)) return []
  if (kind === 'service') {
    return (raw as ServiceRef[]).map((s) => ({
      value: s.name,
      label: `${s.name} (${s.namespace})`,
      aliases: [`${s.name}.${s.namespace}`, `${s.name}.${s.namespace}.svc`, `${s.name}.${s.namespace}.svc.cluster.local`],
    }))
  }
  if (kind === 'gateway' || kind === 'gatewayName') {
    const gws: RefOption[] = (raw as GatewayRef[]).map((g) => ({
      // Istio string refs need "ns/name" for cross-namespace gateways; parentRefs
      // carry the namespace in a sibling field so only the bare name goes in.
      value: kind === 'gatewayName' || g.namespace === ns ? g.name : `${g.namespace}/${g.name}`,
      label: `${g.namespace}/${g.name} (${g.type})`,
      aliases: [g.name, `${g.namespace}/${g.name}`],
      navTypeId: g.type === 'istio' ? ISTIO_GW : GWAPI_GW,
      navNs: g.namespace,
      navName: g.name,
    }))
    if (kind === 'gateway') gws.unshift({ value: 'mesh', label: t('form.meshOption') })
    return gws
  }
  return (raw as SubsetRef[]).map((s) => ({
    value: s.name,
    label: `${s.name} → ${s.host}`,
    navTypeId: DR_TYPE,
    navNs: s.namespace,
    navName: s.dr,
    host: s.host,
  }))
}

export function useReferenceOptions(kind: RefKind, ns: string) {
  // Gateways are few and routinely live in another namespace (e.g. a shared
  // traefik/istio-ingress ns) — scoping them to the form's ns yields an empty
  // dropdown, so list cluster-wide. Services/subsets stay namespace-scoped.
  const allNs = kind === 'gateway' || kind === 'gatewayName'
  return useQuery({
    queryKey: ['ref', kind, allNs ? '' : ns],
    queryFn: () => apiGet<unknown>(`/api/${ENDPOINT[kind]}?ns=${allNs ? '' : ns}`),
    enabled: allNs || !!ns,
    staleTime: 30_000,
    select: (raw) => normalize(kind, raw, ns),
  })
}
