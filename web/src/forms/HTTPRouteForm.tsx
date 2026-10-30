import type { GatewayRef } from '../api/gateways'
import type { ServiceRef } from '../api/services'
import { replace, type HTTPRouteFormModel, type RuleModel, type BackendRefModel } from './httproute'
import { Field, Section, Row, input } from './parts'
import { t } from '../i18n'

export function HTTPRouteForm({
  model,
  onChange,
  gateways,
  services,
  lockIdentity,
}: {
  model: HTTPRouteFormModel
  onChange: (m: HTTPRouteFormModel) => void
  gateways: GatewayRef[]
  services: ServiceRef[]
  lockIdentity: boolean
}) {
  const set = (patch: Partial<HTTPRouteFormModel>) => onChange({ ...model, ...patch })
  const apiGateways = gateways.filter((g) => g.type === 'gateway-api')

  return (
    <div className="space-y-6 panel p-5 shadow-sm">
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

      {/* parentRefs */}
      <Section title={t('form.parentGateways')} required
        onAdd={() => set({ parentRefs: [...model.parentRefs, { name: apiGateways[0]?.name ?? '' }] })}>
        {model.parentRefs.map((p, i) => (
          <Row key={i} onRemove={() => set({ parentRefs: model.parentRefs.filter((_, j) => j !== i) })}>
            <select className={input} value={p.name}
              onChange={(e) => set({ parentRefs: replace(model.parentRefs, i, { ...p, name: e.target.value }) })}>
              {apiGateways.length === 0 && <option value={p.name}>{p.name || t('form.noGateways')}</option>}
              {apiGateways.map((g) => <option key={g.name} value={g.name}>{g.name}</option>)}
            </select>
          </Row>
        ))}
      </Section>

      {/* hostnames */}
      <Section title={t('form.hostnames')}
        onAdd={() => set({ hostnames: [...model.hostnames, ''] })}>
        {model.hostnames.map((h, i) => (
          <Row key={i} onRemove={() => set({ hostnames: model.hostnames.filter((_, j) => j !== i) })}>
            <input className={input} value={h} placeholder="example.com"
              onChange={(e) => set({ hostnames: replace(model.hostnames, i, e.target.value) })} />
          </Row>
        ))}
      </Section>

      {/* rules */}
      <Section title={t('form.rules')} required
        onAdd={() => set({ rules: [...model.rules, { pathType: 'PathPrefix', pathValue: '/', backendRefs: [{ name: '', port: 80 }] }] })}>
        {model.rules.map((rule, ri) => (
          <RuleCard key={ri} rule={rule} services={services}
            onChange={(r) => set({ rules: replace(model.rules, ri, r) })}
            onRemove={() => set({ rules: model.rules.filter((_, j) => j !== ri) })} />
        ))}
      </Section>
    </div>
  )
}

function RuleCard({
  rule, services, onChange, onRemove,
}: {
  rule: RuleModel
  services: ServiceRef[]
  onChange: (r: RuleModel) => void
  onRemove: () => void
}) {
  const weightSum = rule.backendRefs.reduce((s, b) => s + (b.weight ?? 0), 0)
  return (
    <div className="rounded border border-base p-3">
      <div className="mb-3 flex items-center gap-2">
        <span className="text-xs text-muted">{t('form.path')}<span className="text-red-500">*</span></span>
        <select className={input} value={rule.pathType}
          onChange={(e) => onChange({ ...rule, pathType: e.target.value as RuleModel['pathType'] })}>
          <option value="PathPrefix">PathPrefix</option>
          <option value="Exact">Exact</option>
        </select>
        <input className={input} value={rule.pathValue} placeholder="/"
          onChange={(e) => onChange({ ...rule, pathValue: e.target.value })} />
        <button onClick={onRemove} className="ml-auto text-sm text-red-600 hover:underline dark:text-red-400">{t('form.deleteRule')}</button>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted">
            {t('form.backends')} ({t('form.ph.service')}<span className="text-red-500">*</span> / {t('form.ph.port')}<span className="text-red-500">*</span> / {t('form.ph.weight')}<span className="text-faint"> {t('form.optionalWord')}</span>)
          </span>
          {rule.backendRefs.length > 1 && (
            <span className="text-xs text-faint">{t('form.weightSum', { n: weightSum })}</span>
          )}
        </div>
        {rule.backendRefs.map((b, bi) => (
          <BackendRow key={bi} b={b} services={services}
            onChange={(nb) => onChange({ ...rule, backendRefs: replace(rule.backendRefs, bi, nb) })}
            onRemove={() => onChange({ ...rule, backendRefs: rule.backendRefs.filter((_, j) => j !== bi) })} />
        ))}
        <button
          onClick={() => onChange({ ...rule, backendRefs: [...rule.backendRefs, { name: '', port: 80 }] })}
          className="text-sm text-accent hover:underline">+ {t('form.backend')}</button>
      </div>
    </div>
  )
}

function BackendRow({
  b, services, onChange, onRemove,
}: {
  b: BackendRefModel
  services: ServiceRef[]
  onChange: (b: BackendRefModel) => void
  onRemove: () => void
}) {
  const known = services.some((s) => s.name === b.name)
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1">
        <input className={input} list="svc-list" value={b.name} placeholder={t('form.ph.service')}
          onChange={(e) => onChange({ ...b, name: e.target.value })} />
        <datalist id="svc-list">
          {services.map((s) => <option key={s.name} value={s.name} />)}
        </datalist>
      </div>
      {b.name !== '' && !known && (
        <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-500/15 dark:text-amber-300" title={t('form.serviceNotInNs')}>⚠ {t('form.missing')}</span>
      )}
      <input className={`${input} w-24`} type="number" value={b.port} placeholder={t('form.ph.port')}
        onChange={(e) => onChange({ ...b, port: Number(e.target.value) })} />
      <input className={`${input} w-24`} type="number" value={b.weight ?? ''} placeholder={t('form.ph.weight')}
        onChange={(e) => onChange({ ...b, weight: e.target.value === '' ? undefined : Number(e.target.value) })} />
      <button onClick={onRemove} className="text-sm text-red-600 hover:underline dark:text-red-400">×</button>
    </div>
  )
}
