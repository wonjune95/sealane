import type { GatewayRef } from '../api/gateways'
import type { ServiceRef } from '../api/services'
import { useSubsets, type SubsetRef } from '../api/subsets'
import { replace } from './httproute'
import type { VirtualServiceFormModel, VSRuleModel, VSMatchModel, VSDestinationModel } from './virtualservice'
import { Field, Section, Row, Missing, input } from './parts'
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
        <Field label={t('common.name')} required>
          <input className={input} value={model.name} disabled={lockIdentity}
            onChange={(e) => set({ name: e.target.value })} />
        </Field>
        <Field label={t('common.namespace')} required>
          <input className={input} value={model.namespace} disabled={lockIdentity}
            onChange={(e) => set({ namespace: e.target.value })} />
        </Field>
      </div>

      {/* hosts */}
      <Section title={t('form.hosts')} required onAdd={() => set({ hosts: [...model.hosts, ''] })}>
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
      <Section title={t('form.gateways')}
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
      <Section title={t('form.httpRules')} required
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
        <button onClick={() => set({ match: [...rule.match, { headers: [] }] })}
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
            showWeight={rule.route.length > 1}
            onRemove={() => set({ route: rule.route.filter((_, j) => j !== di) })} />
        ))}
        <button onClick={() => set({ route: [...rule.route, { host: '' }] })} className={add}>+ {t('form.addRoute')}</button>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Field label={t('form.timeout')}>
          <input className={input} value={rule.timeout ?? ''} placeholder="5s"
            onChange={(e) => set({ timeout: e.target.value })} />
        </Field>
        <Field label={t('form.retries')}>
          <input className={input} type="number" min={0} value={rule.retries?.attempts ?? ''} placeholder="3"
            onChange={(e) => setRetries(num(e.target.value), rule.retries?.perTryTimeout)} />
        </Field>
        <Field label={t('form.perTryTimeout')}>
          <input className={input} value={rule.retries?.perTryTimeout ?? ''} placeholder="2s"
            onChange={(e) => setRetries(rule.retries?.attempts, e.target.value)} />
        </Field>
        <Field label={t('form.rewriteUri')}>
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
      {m.uriType && (
        <div className="flex items-center gap-2">
          <span className={label}>uri</span>
          <select className={`${input} w-28`} aria-label={t('vs.uriMatchType')} value={m.uriType}
            onChange={(e) => set({ uriType: e.target.value as VSMatchModel['uriType'] })}>
            <option value="prefix">prefix</option>
            <option value="exact">exact</option>
            <option value="regex">regex</option>
          </select>
          <input className={input} aria-label={t('vs.uri')} value={m.uriValue ?? ''} placeholder="/"
            onChange={(e) => set({ uriValue: e.target.value })} />
          <button onClick={() => set({ uriType: undefined, uriValue: undefined })} className={del}>×</button>
        </div>
      )}
      {m.headers.map((h, hi) => (
        <div key={hi} className="flex items-center gap-2 pl-6">
          {/* Committed on blur: a half-typed name (x-user on the way to x-user-id)
              must not collide with an existing header key mid-word. */}
          <input key={`${hi}:${h.name}`} className={input} defaultValue={h.name} placeholder={t('form.ph.header')}
            onBlur={(e) => {
              const v = e.target.value
              if (v === h.name) return
              // Headers are a map: committing a name another row already has would drop a row.
              if (m.headers.some((o, j) => j !== hi && o.name === v)) e.target.value = h.name
              else set({ headers: replace(m.headers, hi, { ...h, name: v }) })
            }} />
          <select className={`${input} w-28`} value={h.type}
            onChange={(e) => set({ headers: replace(m.headers, hi, { ...h, type: e.target.value as typeof h.type }) })}>
            <option value="exact">exact</option>
            <option value="prefix">prefix</option>
            <option value="regex">regex</option>
          </select>
          <input className={input} value={h.value} placeholder={t('form.ph.value')}
            onChange={(e) => set({ headers: replace(m.headers, hi, { ...h, value: e.target.value }) })} />
          <button onClick={() => set({ headers: m.headers.filter((_, j) => j !== hi) })} className={del}>×</button>
        </div>
      ))}
      <div className="flex items-center gap-3 pl-6">
        {!m.uriType && (
          <button onClick={() => set({ uriType: 'prefix', uriValue: '/' })} className={add}>+ URI</button>
        )}
        <button onClick={() => set({ headers: [...m.headers, { name: '', type: 'exact', value: '' }] })}
          className={add}>+ {t('form.addHeader')}</button>
        <button onClick={onRemove} className={`ml-auto ${del}`} aria-label={t('form.deleteMatch')}
          title={t('form.deleteMatch')}>×</button>
      </div>
    </div>
  )
}

// Short and cluster-local names of the same Service compare equal.
const fqdn = (host: string, ns: string) => (host.includes('.') ? host : `${host}.${ns}.svc.cluster.local`)

function RouteRow({
  listId, d, ns, services, subsets, showWeight, onChange, onRemove,
}: {
  listId: string
  d: VSDestinationModel
  ns: string
  services: ServiceRef[]
  subsets: SubsetRef[]
  showWeight: boolean // a single destination takes 100% in Istio, so its weight is noise
  onChange: (d: VSDestinationModel) => void
  onRemove: () => void
}) {
  const offered = d.host ? subsets.filter((s) => fqdn(s.host, s.namespace) === fqdn(d.host, ns)) : []
  return (
    <div className="flex items-center gap-2">
      <input className={`${input} flex-1`} list="vs-svc-list" value={d.host} placeholder={t('form.ph.host')}
        onChange={(e) => onChange({ ...d, host: e.target.value })} />
      <Missing host={d.host} services={services} />
      <input className={`${input} w-28`} list={listId} value={d.subset ?? ''} placeholder={t('form.ph.subset')}
        onChange={(e) => onChange({ ...d, subset: e.target.value })} />
      <datalist id={listId}>
        {offered.map((s) => <option key={`${s.dr}/${s.name}`} value={s.name} />)}
      </datalist>
      <input className={`${input} w-24`} type="number" value={d.port ?? ''} placeholder={t('form.ph.port')}
        onChange={(e) => onChange({ ...d, port: num(e.target.value) })} />
      {showWeight && (
        <input className={`${input} w-24`} type="number" value={d.weight ?? ''} placeholder={t('form.ph.weight')}
          onChange={(e) => onChange({ ...d, weight: num(e.target.value) })} />
      )}
      <button onClick={onRemove} className={del}>×</button>
    </div>
  )
}

const num = (v: string) => (v === '' ? undefined : Number(v))
