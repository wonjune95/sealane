import type { GatewayRef } from '../api/gateways'
import type { ServiceRef } from '../api/services'
import { useSubsets, type SubsetRef } from '../api/subsets'
import { replace } from './httproute'
import type { VirtualServiceFormModel, VSRuleModel, VSMatchModel, VSDestinationModel } from './virtualservice'
import { Field, Section, Row, input } from './parts'
import { t } from '../i18n'

const del = 'text-sm text-red-600 hover:underline dark:text-red-400'
const add = 'text-sm text-accent hover:underline'
const label = 'text-xs text-muted'

export function VirtualServiceForm({
  model,
  onChange,
  gateways,
  services,
  lockIdentity,
}: {
  model: VirtualServiceFormModel
  onChange: (m: VirtualServiceFormModel) => void
  gateways: GatewayRef[]
  services: ServiceRef[]
  lockIdentity: boolean
}) {
  const set = (patch: Partial<VirtualServiceFormModel>) => onChange({ ...model, ...patch })
  const subsets = useSubsets(model.namespace).data ?? []
  const gwValues = gateways
    .filter((g) => g.type === 'istio')
    .map((g) => (g.namespace === model.namespace ? g.name : `${g.namespace}/${g.name}`))

  return (
    <div className="space-y-6 panel p-5 shadow-sm">
      <datalist id="vs-svc-list">
        {services.map((s) => <option key={s.name} value={s.name} />)}
      </datalist>

      {/* identity */}
      <div className="grid grid-cols-2 gap-4">
        <Field label="Name" required>
          <input className={input} value={model.name} disabled={lockIdentity}
            onChange={(e) => set({ name: e.target.value })} />
        </Field>
        <Field label="Namespace" required>
          <input className={input} value={model.namespace} disabled={lockIdentity}
            onChange={(e) => set({ namespace: e.target.value })} />
        </Field>
      </div>

      {/* hosts */}
      <Section title="Hosts" required onAdd={() => set({ hosts: [...model.hosts, ''] })}>
        {model.hosts.map((h, i) => (
          <Row key={i} onRemove={() => set({ hosts: model.hosts.filter((_, j) => j !== i) })}>
            <div className="flex items-center gap-2">
              <input className={input} list="vs-svc-list" value={h} placeholder="reviews / example.com"
                onChange={(e) => set({ hosts: replace(model.hosts, i, e.target.value) })} />
              <Missing host={h} services={services} />
            </div>
          </Row>
        ))}
      </Section>

      {/* gateways — empty means mesh only */}
      <Section title="Gateways"
        onAdd={() => set({ gateways: [...model.gateways, gwValues[0] ?? 'mesh'] })}>
        {model.gateways.map((g, i) => (
          <Row key={i} onRemove={() => set({ gateways: model.gateways.filter((_, j) => j !== i) })}>
            <select className={input} value={g}
              onChange={(e) => set({ gateways: replace(model.gateways, i, e.target.value) })}>
              <option value="mesh">{t('form.meshOption')}</option>
              {gwValues.map((v) => <option key={v} value={v}>{v}</option>)}
              {g !== 'mesh' && !gwValues.includes(g) && <option value={g}>{g}</option>}
            </select>
          </Row>
        ))}
      </Section>

      {/* http rules */}
      <Section title="HTTP rules" required
        onAdd={() => set({ http: [...model.http, { match: [], route: [{ host: '' }] }] })}>
        {model.http.map((rule, ri) => (
          <RuleCard key={ri} id={`r${ri}`} rule={rule} ns={model.namespace} services={services} subsets={subsets}
            onChange={(r) => set({ http: replace(model.http, ri, r) })}
            onRemove={() => set({ http: model.http.filter((_, j) => j !== ri) })} />
        ))}
      </Section>
    </div>
  )
}

// External hosts (with a dot or wildcard) are never flagged.
function Missing({ host, services }: { host: string; services: ServiceRef[] }) {
  if (host === '' || /[.*]/.test(host) || services.some((s) => s.name === host)) return null
  return (
    <span className="whitespace-nowrap rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-500/15 dark:text-amber-300" title={t('form.serviceNotInNs')}>⚠ {t('form.missing')}</span>
  )
}

function RuleCard({
  id, rule, ns, services, subsets, onChange, onRemove,
}: {
  id: string
  rule: VSRuleModel
  ns: string
  services: ServiceRef[]
  subsets: SubsetRef[]
  onChange: (r: VSRuleModel) => void
  onRemove: () => void
}) {
  const set = (patch: Partial<VSRuleModel>) => onChange({ ...rule, ...patch })
  const weightSum = rule.route.reduce((s, d) => s + (d.weight ?? 0), 0)
  const setRetries = (attempts?: number, perTryTimeout?: string) =>
    set({ retries: attempts == null && !perTryTimeout ? undefined : { attempts, perTryTimeout: perTryTimeout || undefined } })
  return (
    <div className="space-y-4 rounded border border-base p-3">
      <div className="flex items-center gap-2">
        <span className={label}>name<span className="text-faint"> {t('form.optionalWord')}</span></span>
        <input className={`${input} max-w-xs`} value={rule.name ?? ''} placeholder="reviews-v2"
          onChange={(e) => set({ name: e.target.value })} />
        <button onClick={onRemove} className={`ml-auto ${del}`}>{t('form.deleteRule')}</button>
      </div>

      {/* match */}
      <div className="space-y-2">
        <div className={label}>Match (uri / headers)<span className="text-faint"> {t('form.optionalWord')}</span></div>
        {rule.match.map((m, mi) => (
          <MatchBox key={mi} m={m}
            onChange={(nm) => set({ match: replace(rule.match, mi, nm) })}
            onRemove={() => set({ match: rule.match.filter((_, j) => j !== mi) })} />
        ))}
        <button onClick={() => set({ match: [...rule.match, { uriType: 'prefix', uriValue: '/', headers: [] }] })}
          className={add}>+ {t('form.addMatch')}</button>
      </div>

      {/* route */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className={label}>
            Route (host<span className="text-red-500">*</span> / subset / port / weight<span className="text-faint"> {t('form.optionalWord')}</span>)
          </span>
          {rule.route.length > 1 && <span className="text-xs text-faint">{t('form.weightSum', { n: weightSum })}</span>}
        </div>
        {rule.route.map((d, di) => (
          <RouteRow key={di} listId={`${id}-d${di}-subsets`} d={d} ns={ns} services={services} subsets={subsets}
            onChange={(nd) => set({ route: replace(rule.route, di, nd) })}
            onRemove={() => set({ route: rule.route.filter((_, j) => j !== di) })} />
        ))}
        <button onClick={() => set({ route: [...rule.route, { host: '' }] })} className={add}>+ {t('form.addRoute')}</button>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Field label="Timeout">
          <input className={input} value={rule.timeout ?? ''} placeholder="5s"
            onChange={(e) => set({ timeout: e.target.value })} />
        </Field>
        <Field label="Retries (attempts)">
          <input className={input} type="number" min={0} value={rule.retries?.attempts ?? ''} placeholder="3"
            onChange={(e) => setRetries(num(e.target.value), rule.retries?.perTryTimeout)} />
        </Field>
        <Field label="Per-try timeout">
          <input className={input} value={rule.retries?.perTryTimeout ?? ''} placeholder="2s"
            onChange={(e) => setRetries(rule.retries?.attempts, e.target.value)} />
        </Field>
        <Field label="Rewrite URI">
          <input className={input} value={rule.rewriteUri ?? ''} placeholder="/"
            onChange={(e) => set({ rewriteUri: e.target.value })} />
        </Field>
      </div>
    </div>
  )
}

function MatchBox({ m, onChange, onRemove }: { m: VSMatchModel; onChange: (m: VSMatchModel) => void; onRemove: () => void }) {
  const set = (patch: Partial<VSMatchModel>) => onChange({ ...m, ...patch })
  return (
    <div className="space-y-2 rounded border border-base p-2">
      <div className="flex items-center gap-2">
        <span className={label}>uri</span>
        <select className={`${input} w-28`} aria-label="URI match type" value={m.uriType ?? ''}
          onChange={(e) => set({ uriType: (e.target.value || undefined) as VSMatchModel['uriType'] })}>
          <option value="">—</option>
          <option value="prefix">prefix</option>
          <option value="exact">exact</option>
          <option value="regex">regex</option>
        </select>
        <input className={input} aria-label="URI" value={m.uriValue ?? ''} placeholder="/" disabled={!m.uriType}
          onChange={(e) => set({ uriValue: e.target.value })} />
        <button onClick={onRemove} className={del}>×</button>
      </div>
      {m.headers.map((h, hi) => (
        <div key={hi} className="flex items-center gap-2 pl-6">
          <input className={input} value={h.name} placeholder="header"
            onChange={(e) => set({ headers: replace(m.headers, hi, { ...h, name: e.target.value }) })} />
          <select className={`${input} w-28`} value={h.type}
            onChange={(e) => set({ headers: replace(m.headers, hi, { ...h, type: e.target.value as typeof h.type }) })}>
            <option value="exact">exact</option>
            <option value="prefix">prefix</option>
            <option value="regex">regex</option>
          </select>
          <input className={input} value={h.value} placeholder="value"
            onChange={(e) => set({ headers: replace(m.headers, hi, { ...h, value: e.target.value }) })} />
          <button onClick={() => set({ headers: m.headers.filter((_, j) => j !== hi) })} className={del}>×</button>
        </div>
      ))}
      <button onClick={() => set({ headers: [...m.headers, { name: '', type: 'exact', value: '' }] })}
        className={`pl-6 ${add}`}>+ {t('form.addHeader')}</button>
    </div>
  )
}

// Short and cluster-local names of the same Service compare equal.
const fqdn = (host: string, ns: string) => (host.includes('.') ? host : `${host}.${ns}.svc.cluster.local`)

function RouteRow({
  listId, d, ns, services, subsets, onChange, onRemove,
}: {
  listId: string
  d: VSDestinationModel
  ns: string
  services: ServiceRef[]
  subsets: SubsetRef[]
  onChange: (d: VSDestinationModel) => void
  onRemove: () => void
}) {
  const offered = d.host ? subsets.filter((s) => fqdn(s.host, s.namespace) === fqdn(d.host, ns)) : []
  return (
    <div className="flex items-center gap-2">
      <input className={`${input} flex-1`} list="vs-svc-list" value={d.host} placeholder="host"
        onChange={(e) => onChange({ ...d, host: e.target.value })} />
      <Missing host={d.host} services={services} />
      <input className={`${input} w-28`} list={listId} value={d.subset ?? ''} placeholder="subset"
        onChange={(e) => onChange({ ...d, subset: e.target.value })} />
      <datalist id={listId}>
        {offered.map((s) => <option key={`${s.dr}/${s.name}`} value={s.name} />)}
      </datalist>
      <input className={`${input} w-24`} type="number" value={d.port ?? ''} placeholder="port"
        onChange={(e) => onChange({ ...d, port: num(e.target.value) })} />
      <input className={`${input} w-24`} type="number" value={d.weight ?? ''} placeholder="weight"
        onChange={(e) => onChange({ ...d, weight: num(e.target.value) })} />
      <button onClick={onRemove} className={del}>×</button>
    </div>
  )
}

const num = (v: string) => (v === '' ? undefined : Number(v))
