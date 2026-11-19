import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { apiGet, getCluster } from '../api/client'
import { useCapabilities } from '../api/capabilities'
import { useResourceTypes, CATEGORY_LABELS, type Category, type ResourceType } from '../api/resourceTypes'
import { CATEGORY_ORDER } from '../ui/categories'
import { useAudit, type AuditEntry } from '../api/audit'
import { VERB_LABEL, VERB_TINT, formatTime } from '../lib/verbs'
import { useAnalysis, type Finding } from '../api/analyze'
import { Icon } from '../components/icons'
import { KindBadge } from '../components/KindBadge'
import { EmptyState } from '../components/EmptyState'
import type { ResourceSummary } from '../api/resources'
import { t, type Key } from '../i18n'

const QUICK: { type: string; label: string; hint: Key }[] = [
  { type: 'virtualservices.networking.istio.io', label: 'VirtualService', hint: 'home.hint.virtualService' },
  { type: 'destinationrules.networking.istio.io', label: 'DestinationRule', hint: 'home.hint.destinationRule' },
  { type: 'gateways.networking.istio.io', label: 'Gateway', hint: 'home.hint.gateway' },
  { type: 'httproutes.gateway.networking.k8s.io', label: 'HTTPRoute', hint: 'home.hint.httpRoute' },
]
const ROWS = 10

type Health = 'error' | 'warning' | 'ok'
type Row = ResourceSummary & { type: ResourceType; health: Health }
const RANK: Record<Health, number> = { error: 0, warning: 1, ok: 2 }
const refKey = (typeId: string, ns: string, name: string) => `${typeId}/${ns}/${name}`

export function Home() {
  const caps = useCapabilities()
  const types = useResourceTypes()
  const installed = (types.data ?? []).filter((rt) => rt.installed)
  const audit = useAudit()
  const analysis = useAnalysis('', false)
  const findings = analysis.data?.findings
  const [cat, setCat] = useState<Category | ''>('')
  const results = useQueries({
    queries: installed.map((rt) => ({
      queryKey: ['resources', rt.typeId, ''],
      queryFn: () => apiGet<ResourceSummary[]>(`/api/resources/${rt.typeId}`),
    })),
  })
  const loading = types.isLoading || results.some((r) => r.isLoading)
  const failed = types.isError || results.some((r) => r.isError)

  // A resource's health: the worst analysis finding on it, else its own status condition.
  const worst = new Map<string, Health>()
  for (const f of findings ?? []) {
    const k = refKey(f.resource.typeId, f.resource.namespace, f.resource.name)
    if (worst.get(k) !== 'error') worst.set(k, f.severity)
  }
  const rows: Row[] = installed.flatMap((rt, i) =>
    (results[i]?.data ?? []).map((r) => {
      const status = r.status?.split(':')[0]
      const health =
        worst.get(refKey(rt.typeId, r.namespace, r.name)) ??
        (status === 'error' ? 'error' : status === 'warning' ? 'warning' : 'ok')
      return { ...r, type: rt, health }
    }),
  )
  rows.sort(
    (a, b) => RANK[a.health] - RANK[b.health] || a.namespace.localeCompare(b.namespace) || a.name.localeCompare(b.name),
  )
  const count = (h: Health) => rows.filter((r) => r.health === h).length
  const shown = cat ? rows.filter((r) => r.type.category === cat) : rows
  const canWrite = caps.data?.role === 'admin' || caps.data?.role === 'editor'
  const quick = QUICK.filter((q) => installed.some((rt) => rt.typeId === q.type))
  const kindOf = (typeId: string) => types.data?.find((rt) => rt.typeId === typeId)?.kind ?? typeId
  const dash = loading || failed
  const sortedFindings = [...(findings ?? [])].sort((a, b) =>
    a.severity === b.severity ? 0 : a.severity === 'error' ? -1 : 1,
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <h1 className="text-xl font-semibold text-strong">{t('home.title')}</h1>
        <span className="text-[13px] text-muted">
          Istio{' '}
          <b className="font-semibold text-strong">
            {caps.data?.istiodVersion || caps.data?.istioApiVersion || t('home.notInstalled')}
          </b>
        </span>
        <span className="text-[13px] text-muted">
          Gateway API <b className="font-semibold text-strong">{caps.data?.gatewayAPIVersion || t('home.notInstalled')}</b>
        </span>
        <span className="text-[13px] text-muted">{t('home.kindCount', { n: installed.length })}</span>
        <span className="inline-flex items-center gap-1.5 text-[13px] font-medium text-emerald-700 dark:text-emerald-400">
          <span className="h-2 w-2 rounded-full bg-emerald-500" />
          {t('home.apiConnected')}
        </span>
        {canWrite && quick.length > 0 && (
          <details className="create-menu ml-auto">
            <summary className="btn-primary">
              <Icon name="plus" className="h-4 w-4" />
              {t('home.createResource')}
              <Icon name="down" className="h-3.5 w-3.5" />
            </summary>
            <div className="menu-panel">
              {quick.map((q) => (
                <Link
                  key={q.type}
                  to={`/resources/${q.type}/new`}
                  className="flex items-center gap-3 rounded px-3 py-2 hover:bg-slate-50 dark:hover:bg-slate-800"
                >
                  <KindBadge kind={q.label} typeId={q.type} />
                  <span>
                    <span className="block text-xs font-medium text-strong">{q.label}</span>
                    <span className="text-[11px] text-muted">{t(q.hint)}</span>
                  </span>
                </Link>
              ))}
            </div>
          </details>
        )}
      </div>
      {failed && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
        >
          <Icon name="warning" className="h-4 w-4" />
          {t('home.partialFailure')}
        </div>
      )}
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <Stat label={t('home.statResources')} value={dash ? '—' : rows.length} bar="border-l-[rgb(var(--accent))]" />
        <Stat
          label={t('home.statHealthy')}
          title={t('home.statHealthyHint')}
          value={dash || !findings ? '—' : count('ok')}
          bar="border-l-emerald-500"
          icon={<Icon name="ok" className="h-4 w-4 text-emerald-600" />}
        />
        <Stat
          label={t('home.statWarnings')}
          value={dash || !findings ? '—' : count('warning')}
          bar="border-l-amber-500"
          icon={<Icon name="warning" className="h-4 w-4 text-amber-600" />}
        />
        <Stat
          label={t('home.statErrors')}
          value={dash || !findings ? '—' : count('error')}
          bar="border-l-red-500"
          icon={<Icon name="error" className="h-4 w-4 text-red-600" />}
        />
      </div>
      <div className="flex flex-wrap items-start gap-4">
        <section className="panel min-w-0 flex-[999_1_560px] overflow-hidden" aria-labelledby="home-resources">
          <div className="flex flex-wrap items-center gap-1.5 border-b border-base px-4 py-2.5">
            <h2 id="home-resources" className="section-title mr-2">
              {t('home.resourcesTitle')}
            </h2>
            {(['', ...CATEGORY_ORDER] as const).map((c) => {
              if (c && !installed.some((rt) => rt.category === c)) return null
              const n = c ? rows.filter((r) => r.type.category === c).length : rows.length
              return (
                <button
                  key={c || 'all'}
                  type="button"
                  aria-pressed={cat === c}
                  onClick={() => setCat(c)}
                  className={`h-6 rounded-full border px-2.5 text-xs ${
                    cat === c
                      ? 'border-[rgb(var(--accent))] bg-[rgb(var(--accent)/0.08)] font-medium text-accent'
                      : 'border-base text-slate-600 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800'
                  }`}
                >
                  {c ? CATEGORY_LABELS[c] : t('home.filterAll')} {dash ? '' : n}
                </button>
              )
            })}
          </div>
          <div className="table-scroll">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t('home.col.status')}</th>
                  <th>{t('home.col.kind')}</th>
                  <th>{t('home.col.name')}</th>
                  <th>{t('home.col.namespace')}</th>
                  <th>{t('home.col.summary')}</th>
                  <th>{t('home.col.age')}</th>
                </tr>
              </thead>
              <tbody>
                {shown.slice(0, ROWS).map((r) => (
                  <tr key={refKey(r.typeId, r.namespace, r.name)} className="border-t border-slate-100 dark:border-slate-800">
                    <td className="!py-2">
                      <HealthLabel health={r.health} />
                    </td>
                    <td className="!py-2">
                      <KindBadge kind={r.type.kind} category={r.type.category} />
                    </td>
                    <td className="!py-2">
                      <Link
                        to={`/resources/${r.typeId}/${r.namespace || '-'}/${r.name}`}
                        className="font-medium text-accent hover:underline"
                      >
                        {r.name}
                      </Link>
                    </td>
                    <td className="!py-2 text-muted">{r.namespace || '—'}</td>
                    <td className="max-w-[320px] truncate !py-2 font-mono text-xs text-muted">{r.summary || '—'}</td>
                    <td className="!py-2 text-muted">{r.age}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!dash && shown.length === 0 && <p className="px-4 py-6 text-sm text-muted">{t('home.noResources')}</p>}
            {loading && <p className="px-4 py-6 text-sm text-muted">{t('common.loading')}</p>}
          </div>
          <div className="flex items-center justify-between border-t border-base px-4 py-2.5 text-xs text-muted">
            <span>{dash ? '' : t('home.shown', { shown: Math.min(ROWS, shown.length), total: shown.length })}</span>
            <Link to="/overview" className="inline-flex items-center gap-1 font-medium text-accent hover:underline">
              {t('home.allResources')}
              <Icon name="arrow" className="h-3.5 w-3.5" />
            </Link>
          </div>
        </section>
        <div className="flex min-w-0 flex-[1_1_300px] flex-col gap-4">
          <section className="panel overflow-hidden" aria-labelledby="home-analysis">
            <div className="flex items-center border-b border-base px-4 py-2.5">
              <h2 id="home-analysis" className="section-title">
                {t('home.analysisTitle')}
              </h2>
              <Link to="/analysis" className="ml-auto text-xs font-medium text-accent hover:underline">
                {t('home.open')}
              </Link>
            </div>
            {analysis.isLoading ? (
              <p className="px-4 py-4 text-sm text-muted">{t('common.loading')}</p>
            ) : analysis.isError ? (
              <p role="alert" className="px-4 py-4 text-sm text-red-600 dark:text-red-400">
                {t('home.analysisFailed')}
              </p>
            ) : sortedFindings.length === 0 ? (
              <p className="flex items-center gap-2 px-4 py-4 text-sm text-muted">
                <Icon name="ok" className="h-4 w-4 text-emerald-600" />
                {t('home.noProblems')}
              </p>
            ) : (
              <ul>
                {sortedFindings.slice(0, 4).map((f, i) => (
                  <FindingRow key={i} f={f} />
                ))}
              </ul>
            )}
          </section>
          <section className="panel overflow-hidden" aria-labelledby="audit-title">
            <div className="flex items-center border-b border-base px-4 py-2.5">
              <h2 id="audit-title" className="section-title" title={t('home.recentChangesScope')}>
                {t('home.recentChanges')}
              </h2>
            </div>
            {audit.isLoading ? (
              <p className="px-4 py-4 text-sm text-muted">{t('home.auditLoading')}</p>
            ) : audit.isError ? (
              <p role="alert" className="px-4 py-4 text-sm text-red-600 dark:text-red-400">
                {t('home.auditFailed')}
              </p>
            ) : !audit.data?.length ? (
              <EmptyState icon="clock" title={t('home.auditEmptyTitle')} description={t('home.auditEmptyDescription')} />
            ) : (
              <ul className="max-h-80 divide-y divide-slate-100 overflow-y-auto px-4 dark:divide-slate-800">
                {audit.data.map((a, i) => (
                  <ActionRow key={a.id || i} a={a} kind={kindOf(a.typeId)} canRestore={canWrite} />
                ))}
              </ul>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}

function Stat({
  label,
  value,
  bar,
  icon,
  title,
}: {
  label: string
  value: number | string
  bar: string
  icon?: ReactNode
  title?: string
}) {
  return (
    <div className={`panel border-l-4 px-4 py-3 ${bar}`} title={title}>
      <div className="flex items-center gap-1.5 text-[13px] text-muted">
        {icon}
        {label}
      </div>
      <div className="mt-1 text-[26px] font-semibold tabular-nums text-strong">{value}</div>
    </div>
  )
}

const HEALTH: Record<Health, { icon: string; cls: string; label: Key }> = {
  error: { icon: 'error', cls: 'text-red-700 dark:text-red-400', label: 'home.status.error' },
  warning: { icon: 'warning', cls: 'text-amber-700 dark:text-amber-400', label: 'home.status.warning' },
  ok: { icon: 'ok', cls: 'text-emerald-700 dark:text-emerald-400', label: 'home.status.ok' },
}

function HealthLabel({ health }: { health: Health }) {
  const h = HEALTH[health]
  return (
    <span className={`inline-flex items-center gap-1.5 whitespace-nowrap text-xs font-medium ${h.cls}`}>
      <Icon name={h.icon} className="h-3.5 w-3.5" />
      {t(h.label)}
    </span>
  )
}

function FindingRow({ f }: { f: Finding }) {
  const r = f.resource
  const to = r.typeId === 'namespaces' ? '/analysis' : `/resources/${r.typeId}/${r.namespace || '-'}/${r.name}`
  return (
    <li className="flex gap-2.5 border-b border-slate-100 px-4 py-2.5 last:border-0 dark:border-slate-800">
      <Icon
        name={f.severity === 'error' ? 'error' : 'warning'}
        className={`mt-0.5 h-4 w-4 shrink-0 ${f.severity === 'error' ? 'text-red-600' : 'text-amber-600'}`}
      />
      <div className="min-w-0 text-[13px]">
        <Link to={to} className="font-medium text-strong hover:text-accent hover:underline">
          {r.namespace ? `${r.namespace}/` : ''}
          {r.name}
        </Link>
        <p className="mt-0.5 text-xs leading-5 text-muted">{f.message}</p>
      </div>
    </li>
  )
}

function ActionRow({ a, kind, canRestore }: { a: AuditEntry; kind: string; canRestore: boolean }) {
  // A deletion made in the cluster being viewed can be reopened in the editor and recreated.
  const restorable = canRestore && a.verb === 'delete' && a.ok && a.id && (a.cluster ?? '') === currentCluster()
  return (
    <li className="flex flex-wrap items-center gap-2 py-2.5 text-xs">
      <KindBadge kind={kind} typeId={a.typeId} />
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium text-strong">
          {a.namespace ? `${a.namespace}/` : ''}
          {a.name}
        </span>
        <span className="mt-0.5 block text-[11px] text-muted">
          <span className={`mr-1.5 rounded px-1.5 py-px font-medium ${VERB_TINT[a.verb]}`}>{t(VERB_LABEL[a.verb])}</span>
          {a.user}
          {a.user && a.provider === 'oidc' && ` ${t('home.auditViaSso')}`}
        </span>
      </span>
      {!a.ok && <span className="text-red-600">{t('home.failed')}</span>}
      {restorable && (
        <Link
          to={`/resources/${a.typeId}/new?restore=${encodeURIComponent(a.id ?? '')}`}
          title={t('home.restoreTitle')}
          className="btn-ghost !min-h-6 !px-2 text-xs"
        >
          {t('home.restore')}
        </Link>
      )}
      <time dateTime={a.time} className="text-[11px] tabular-nums text-muted">
        {formatTime(a.time)}
      </time>
    </li>
  )
}
const currentCluster = () => (getCluster() === 'local' ? '' : getCluster())
