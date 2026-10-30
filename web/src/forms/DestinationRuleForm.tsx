import type { ServiceRef } from '../api/services'
import { replace } from './httproute'
import { LB_SIMPLE, TLS_MODES, type DestinationRuleFormModel, type DRSubsetModel } from './destinationrule'
import { Field, Section, Missing, input } from './parts'
import { t } from '../i18n'

const del = 'text-sm text-red-600 hover:underline dark:text-red-400'
const add = 'text-sm text-accent hover:underline'

export function DestinationRuleForm({
  model,
  onChange,
  services,
  lockIdentity,
}: {
  model: DestinationRuleFormModel
  onChange: (m: DestinationRuleFormModel) => void
  services: ServiceRef[]
  lockIdentity: boolean
}) {
  const set = (patch: Partial<DestinationRuleFormModel>) => onChange({ ...model, ...patch })

  return (
    <div className="space-y-6 panel p-5 shadow-sm">
      <datalist id="dr-svc-list">
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

      <Field label={t('form.host')} required>
        <div className="flex items-center gap-2">
          <input className={input} list="dr-svc-list" value={model.host} placeholder="reviews / reviews.shop.svc.cluster.local"
            onChange={(e) => set({ host: e.target.value })} />
          <Missing host={model.host} services={services} />
        </div>
      </Field>

      <Section title={t('form.subsets')} onAdd={() => set({ subsets: [...model.subsets, { name: '', labels: [] }] })}>
        {model.subsets.map((s, si) => (
          <SubsetCard key={si} s={s}
            onChange={(ns) => set({ subsets: replace(model.subsets, si, ns) })}
            onRemove={() => set({ subsets: model.subsets.filter((_, j) => j !== si) })} />
        ))}
      </Section>

      <Section title={t('form.trafficPolicy')}>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t('form.tlsMode')}>
            <select className={input} value={model.tlsMode ?? ''}
              onChange={(e) => set({ tlsMode: (e.target.value || undefined) as DestinationRuleFormModel['tlsMode'] })}>
              <option value="">{t('form.unset')}</option>
              {TLS_MODES.map((v) => <option key={v} value={v}>{v}</option>)}
            </select>
          </Field>
          <Field label={t('form.loadBalancer')}>
            <select className={input} value={model.lbSimple ?? ''}
              onChange={(e) => set({ lbSimple: (e.target.value || undefined) as DestinationRuleFormModel['lbSimple'] })}>
              <option value="">{t('form.unset')}</option>
              {LB_SIMPLE.map((v) => <option key={v} value={v}>{v}</option>)}
            </select>
          </Field>
        </div>
      </Section>
    </div>
  )
}

function SubsetCard({ s, onChange, onRemove }: { s: DRSubsetModel; onChange: (s: DRSubsetModel) => void; onRemove: () => void }) {
  const set = (patch: Partial<DRSubsetModel>) => onChange({ ...s, ...patch })
  return (
    <div className="space-y-2 rounded border border-base p-3">
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted">name<span className="text-red-500">*</span></span>
        <input className={`${input} max-w-xs`} value={s.name} placeholder="v1"
          onChange={(e) => set({ name: e.target.value })} />
        <button onClick={onRemove} className={`ml-auto ${del}`}>{t('form.deleteSubset')}</button>
      </div>
      {s.labels.map((l, li) => (
        <div key={li} className="flex items-center gap-2 pl-6">
          {/* Committed on blur: a half-typed key (app on the way to app-tier) must not
              collide with an existing label key mid-word. */}
          <input key={`${li}:${l.key}`} className={input} defaultValue={l.key} placeholder={t('form.ph.label')}
            onBlur={(e) => {
              const v = e.target.value
              if (v === l.key) return
              // Labels are a map: committing a key another row already has would drop a row.
              if (s.labels.some((o, j) => j !== li && o.key === v)) e.target.value = l.key
              else set({ labels: replace(s.labels, li, { ...l, key: v }) })
            }} />
          <span className="text-faint">=</span>
          <input className={input} value={l.value} placeholder={t('form.ph.value')}
            onChange={(e) => set({ labels: replace(s.labels, li, { ...l, value: e.target.value }) })} />
          <button onClick={() => set({ labels: s.labels.filter((_, j) => j !== li) })} className={del}>×</button>
        </div>
      ))}
      <button onClick={() => set({ labels: [...s.labels, { key: '', value: '' }] })}
        className={`pl-6 ${add}`}>+ {t('form.addLabel')}</button>
    </div>
  )
}
