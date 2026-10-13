import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { apiGet, getCluster } from '../api/client'
import { useCapabilities } from '../api/capabilities'
import { useResourceTypes, CATEGORY_LABELS, type Category } from '../api/resourceTypes'
import { CATEGORY_ORDER, CATEGORY_ICON, accentFor } from '../ui/categories'
import { useAudit, type AuditEntry } from '../api/audit'
import { Icon } from '../components/icons'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import type { ResourceSummary } from '../api/resources'
import { t, type Key } from '../i18n'

const QUICK: { type: string; label: string; hint: Key }[] = [
  { type: 'virtualservices.networking.istio.io', label: 'VirtualService', hint: 'home.hint.virtualService' },
  { type: 'destinationrules.networking.istio.io', label: 'DestinationRule', hint: 'home.hint.destinationRule' },
  { type: 'gateways.networking.istio.io', label: 'Gateway', hint: 'home.hint.gateway' },
  { type: 'httproutes.gateway.networking.k8s.io', label: 'HTTPRoute', hint: 'home.hint.httpRoute' },
]

export function Home() {
  const caps = useCapabilities()
  const types = useResourceTypes()
  const installed = (types.data ?? []).filter((rt) => rt.installed)
  const audit = useAudit()
  const kindOf = (typeId: string) => types.data?.find((rt) => rt.typeId === typeId)?.kind ?? typeId
  const results = useQueries({
    queries: installed.map((rt) => ({
      queryKey: ['resources', rt.typeId, ''],
      queryFn: () => apiGet<ResourceSummary[]>(`/api/resources/${rt.typeId}`),
    })),
  })
  const countByType = new Map(installed.map((rt, i) => [rt.typeId, results[i]?.data?.length]))
  const loading = types.isLoading || results.some((r) => r.isLoading)
  const failed = types.isError || results.some((r) => r.isError)
  const total = [...countByType.values()].reduce<number>((a, b) => a + (b ?? 0), 0)
  const dangerousActive = installed.filter((rt) => rt.dangerous && (countByType.get(rt.typeId) ?? 0) > 0).length
  const catCount = (cat: Category) =>
    installed.filter((rt) => rt.category === cat).reduce((a, rt) => a + (countByType.get(rt.typeId) ?? 0), 0)
  const canWrite = caps.data?.role === 'admin' || caps.data?.role === 'editor'
  const quick = QUICK.filter((q) => installed.some((rt) => rt.typeId === q.type))

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Workspace overview"
        title={t('home.title')}
        description={t('home.description')}
        actions={
          <>
            <Link to="/overview" className="btn-ghost">
              <Icon name="cube" className="h-4 w-4" />
              {t('home.allResources')}
            </Link>
            {canWrite && quick.length > 0 && (
              <details className="create-menu">
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
                      className="flex items-center gap-3 rounded-lg px-3 py-2.5 hover:bg-slate-50 dark:hover:bg-slate-800"
                    >
                      <Icon name="plus" className="h-4 w-4 text-accent" />
                      <span>
                        <span className="block text-xs font-medium text-strong">{q.label}</span>
                        <span className="text-[11px] text-muted">{t(q.hint)}</span>
                      </span>
                    </Link>
                  ))}
                </div>
              </details>
            )}
          </>
        }
      />
      <div className="flex flex-wrap items-center gap-3 text-xs text-muted">
        <span className="inline-flex items-center gap-1.5 font-medium text-emerald-700 dark:text-emerald-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          {t('home.apiConnected')}
        </span>
        <span className="h-3 w-px bg-slate-200 dark:bg-slate-700" />
        <span className="inline-flex items-center gap-1.5">
          <Icon name="network" className="h-3.5 w-3.5" />
          {getCluster() || 'local'}
        </span>
        <span>{t('home.clusterWide')}</span>
      </div>
      {failed && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
        >
          <Icon name="warning" className="h-4 w-4" />
          {t('home.partialFailure')}
        </div>
      )}
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4 xl:gap-4">
        <Diag
          label={t('home.allResources')}
          value={loading || failed ? '—' : total}
          icon="cube"
          hint={t('home.kindCount', { n: installed.length })}
        />
        <Diag
          label={t('home.istioControlPlane')}
          value={caps.data?.istiodVersion || caps.data?.istioApiVersion || t('home.notInstalled')}
          icon="network"
          hint={caps.data?.istioApiVersion ? `API ${caps.data.istioApiVersion}` : t('home.noIstio')}
        />
        <Diag
          label="Gateway API"
          value={caps.data?.gatewayAPIVersion || t('home.notInstalled')}
          icon="globe"
          hint={t('home.gatewayHint')}
        />
        <Diag
          label={t('home.dangerousActive')}
          value={loading || failed ? '—' : dangerousActive}
          icon="shield"
          hint={t('home.dangerousHint')}
          warning={dangerousActive > 0}
        />
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <Shortcut
          to="/flowmap"
          icon="route"
          title={t('home.flowTitle')}
          description={t('home.flowDescription')}
        />
        <Shortcut
          to="/routecheck"
          icon="terminal"
          title={t('home.routeCheckTitle')}
          description={t('home.routeCheckDescription')}
        />
      </div>
      <section aria-labelledby="resource-status-title">
        <div className="mb-3 flex items-center justify-between">
          <h2 id="resource-status-title" className="section-title">
            {t('home.resourceStatus')}
          </h2>
          <Link
            to="/overview"
            className="inline-flex items-center gap-1 text-xs text-muted hover:text-accent"
          >
            {t('home.viewAll')}
            <Icon name="arrow" className="h-3.5 w-3.5" />
          </Link>
        </div>
        {types.isLoading ? (
          <div className="grid gap-4 md:grid-cols-2" aria-label={t('home.resourceStatusLoading')}>
            {[1, 2, 3, 4].map((n) => (
              <div key={n} className="panel h-36 animate-pulse bg-slate-100 dark:bg-slate-800" />
            ))}
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            {CATEGORY_ORDER.map((cat) => {
              const a = accentFor(cat)
              const kinds = installed.filter((rt) => rt.category === cat)
              if (!kinds.length) return null
              const incomplete = kinds.some((rt) => countByType.get(rt.typeId) === undefined)
              return (
                <div key={cat} className="panel overflow-hidden">
                  <div className="flex items-center gap-2.5 border-b border-base px-4 py-3">
                    <span className={`flex h-7 w-7 items-center justify-center rounded-lg ${a.bg} ${a.text}`}>
                      <Icon name={CATEGORY_ICON[cat]} className="h-4 w-4" />
                    </span>
                    <h3 className="text-xs font-semibold text-strong">{CATEGORY_LABELS[cat]}</h3>
                    <span className="ml-auto text-xs tabular-nums text-muted">
                      {incomplete ? '—' : catCount(cat)} <span className="text-[10px]">{t('home.resources')}</span>
                    </span>
                  </div>
                  <div className="grid gap-x-2 p-2 sm:grid-cols-2">
                    {kinds.map((rt) => (
                      <Link key={rt.typeId} to={`/resources/${rt.typeId}`} className="category-item">
                        <span className="flex min-w-0 items-center gap-1.5">
                          <span className="truncate">{rt.kind}</span>
                          {rt.dangerous && <Icon name="warning" className="h-3 w-3 shrink-0 text-amber-500" />}
                        </span>
                        <span className="nav-count">{countByType.get(rt.typeId) ?? '—'}</span>
                      </Link>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </section>
      <section className="panel overflow-hidden" aria-labelledby="audit-title">
        <div className="flex items-center justify-between border-b border-base px-5 py-4">
          <div className="flex items-center gap-2">
            <Icon name="clock" className="h-4 w-4 text-muted" />
            <h2 id="audit-title" className="section-title">
              {t('home.recentChanges')}
            </h2>
          </div>
          <span className="text-[11px] text-muted">{t('home.recentChangesScope')}</span>
        </div>
        {audit.isLoading ? (
          <p className="px-5 py-8 text-sm text-muted">{t('home.auditLoading')}</p>
        ) : audit.isError ? (
          <p role="alert" className="px-5 py-8 text-sm text-red-600 dark:text-red-400">
            {t('home.auditFailed')}
          </p>
        ) : !audit.data?.length ? (
          <EmptyState
            icon="clock"
            title={t('home.auditEmptyTitle')}
            description={t('home.auditEmptyDescription')}
          />
        ) : (
          <ul className="max-h-72 divide-y divide-slate-100 overflow-y-auto px-5 dark:divide-slate-800">
            {audit.data.map((a, i) => (
              <ActionRow key={i} a={a} kind={kindOf(a.typeId)} />
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

const VERB_LABEL: Record<AuditEntry['verb'], Key> = {
  create: 'home.verb.create',
  update: 'home.verb.update',
  delete: 'home.verb.delete',
}
const VERB_TINT: Record<AuditEntry['verb'], string> = {
  create: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400',
  update: 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300',
  delete: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400',
}
function ActionRow({ a, kind }: { a: AuditEntry; kind: string }) {
  const d = new Date(a.time)
  return (
    <li className="flex flex-wrap items-center gap-3 py-3 text-xs">
      <span className={`rounded-md px-2 py-1 font-medium ${VERB_TINT[a.verb]}`}>{t(VERB_LABEL[a.verb])}</span>
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium text-strong">
          {a.namespace ? `${a.namespace}/` : ''}
          {a.name}
        </span>
        <span className="mt-0.5 block text-[11px] text-muted">
          {kind}
          {a.user && ` · ${a.user}`}
        </span>
      </span>
      {!a.ok && <span className="text-red-500">{t('home.failed')}</span>}
      <time dateTime={a.time} className="text-[11px] tabular-nums text-muted">
        {d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString() : d.toLocaleString()}
      </time>
    </li>
  )
}
function Diag({
  label,
  value,
  icon,
  hint,
  warning = false,
}: {
  label: string
  value: ReactNode
  icon: string
  hint: string
  warning?: boolean
}) {
  return (
    <div className="stat-card">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted">{label}</span>
        <span
          className={`stat-icon ${warning ? 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-400' : 'bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300'}`}
        >
          <Icon name={icon} className="h-[18px] w-[18px]" />
        </span>
      </div>
      <div className="stat-value">{value}</div>
      <p className="mt-2 text-[11px] text-muted">{hint}</p>
    </div>
  )
}
function Shortcut({
  to,
  icon,
  title,
  description,
}: {
  to: string
  icon: string
  title: string
  description: string
}) {
  return (
    <Link to={to} className="shortcut-card group">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300">
        <Icon name={icon} className="h-5 w-5" />
      </span>
      <span className="min-w-0">
        <span className="block text-xs font-semibold text-strong">{title}</span>
        <span className="mt-1 block text-[11px] leading-5 text-muted">{description}</span>
      </span>
      <Icon name="arrow" className="ml-auto h-4 w-4 shrink-0 text-faint group-hover:text-accent" />
    </Link>
  )
}
