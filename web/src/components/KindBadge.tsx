import type { Category } from '../api/resourceTypes'
import { accentFor, categoryOf } from '../ui/categories'

const ABBR: Record<string, string> = {
  VirtualService: 'VS', DestinationRule: 'DR', Gateway: 'GW', ServiceEntry: 'SE', Sidecar: 'SC',
  WorkloadEntry: 'WE', WorkloadGroup: 'WG', EnvoyFilter: 'EF', ProxyConfig: 'PC', WasmPlugin: 'WP',
  AuthorizationPolicy: 'AP', PeerAuthentication: 'PA', RequestAuthentication: 'RA', Telemetry: 'TM',
  GatewayClass: 'GC', HTTPRoute: 'HR', GRPCRoute: 'GR', ReferenceGrant: 'RG', TLSRoute: 'TLS',
  TCPRoute: 'TCP', UDPRoute: 'UDP', BackendTLSPolicy: 'BTP', Service: 'SVC', Namespace: 'NS',
}

// A coloured abbreviation per kind (VS, DR, HR …), tinted by category; showName adds the kind.
export function KindBadge({
  kind,
  typeId,
  category,
  showName = false,
}: {
  kind: string
  typeId?: string
  category?: Category
  showName?: boolean
}) {
  const a = accentFor(category ?? (typeId ? categoryOf(typeId) : undefined))
  const abbr = ABBR[kind] ?? (kind.replace(/[^A-Z]/g, '') || kind).slice(0, 3)
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5" title={kind}>
      <span
        aria-hidden
        className={`inline-block min-w-7 rounded-sm px-1 py-px text-center text-[10px] font-bold leading-4 tracking-wide ${a.badge}`}
      >
        {abbr}
      </span>
      {showName ? <span className="text-xs font-medium text-strong">{kind}</span> : <span className="sr-only">{kind}</span>}
    </span>
  )
}
