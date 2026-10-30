import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useServiceList } from '../api/mesh'
import { useNamespaces } from '../api/namespaces'
import { useNamespace } from '../ui/namespace'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Icon } from '../components/icons'
import { SortHeader } from '../components/SortHeader'
import { PAGE, sortBy, toggleSort, type SortState } from '../lib/sort'
import { t, type Key } from '../i18n'
import { errorText } from '../api/client'

type Count = 'routes' | 'destinationRules' | 'policies' | 'findings'
const COUNTS: [Count, Key][] = [
  ['routes', 'services.colRoutes'],
  ['destinationRules', 'services.colRules'],
  ['policies', 'services.colPolicies'],
  ['findings', 'services.colFindings'],
]

export function Services() {
  const { ns, setNs } = useNamespace()
  const namespaces = useNamespaces()
  const { data, isLoading, error } = useServiceList(ns)
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SortState | null>(null)
  const [meshOnly, setMeshOnly] = useState(false)
  const [shown, setShown] = useState(PAGE)
  const rows = sortBy(
    (data ?? []).filter(
      (s) =>
        `${s.namespace} ${s.name}`.toLowerCase().includes(search.toLowerCase()) &&
        (!meshOnly || COUNTS.some(([k]) => (s[k] ?? 0) > 0)),
    ),
    sort,
    (s, k) => (k === 'name' ? s.name : k === 'namespace' ? s.namespace : (s[k as Count] ?? 0)),
  )
  const onSort = (k: string) => {
    setSort((s) => toggleSort(s, k))
    setShown(PAGE)
  }

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <PageHeader
        eyebrow={t('eyebrow.services')}
        title={
          <>
            {t('services.title')}
            <span className="nav-count !text-xs">{data ? (meshOnly ? t('services.countOf', { shown: rows.length, total: data.length }) : data.length) : '—'}</span>
          </>
        }
        description={t('services.description')}
      />
      <div className="panel flex flex-wrap items-center gap-3 p-3">
        <label className="relative min-w-0 flex-1 basis-full sm:min-w-[14rem] sm:max-w-sm sm:basis-auto">
          <Icon name="search" className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-faint" />
          <input
            aria-label={t('services.search')}
            placeholder={t('services.search')}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setShown(PAGE)
            }}
            className="input-base pl-9"
          />
        </label>
        <label className="flex shrink-0 items-center gap-2 text-xs text-muted">
          <span>{t('common.namespace')}</span>
          <select className="input-base !w-auto max-w-56" value={ns} onChange={(e) => setNs(e.target.value)}>
            <option value="">{t('list.allNamespaces')}</option>
            {(namespaces.data ?? []).map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
        <label className="flex shrink-0 items-center gap-2 text-xs text-muted">
          <input
            type="checkbox"
            checked={meshOnly}
            onChange={(e) => {
              setMeshOnly(e.target.checked)
              setShown(PAGE)
            }}
          />
          <span>{t('services.meshOnly')}</span>
        </label>
      </div>
      {error && (
        <div className="flex items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-600 dark:text-red-400" role="alert">
          <Icon name="warning" />
          {t('services.loadFailed', { message: errorText(error) })}
        </div>
      )}
      {isLoading ? (
        <div className="p-12 text-center text-sm text-muted" role="status">
          {t('common.loading')}
        </div>
      ) : data && rows.length === 0 ? (
        <EmptyState icon="search" title={t('services.empty')} description={t('services.emptyHint')} />
      ) : data ? (
        <>
        <div className="panel table-scroll">
          <table className="data-table">
            <thead className="border-b border-base bg-slate-50 text-left text-xs uppercase tracking-wide text-muted dark:bg-slate-800/50">
              <tr>
                <SortHeader label="Namespace" sortKey="namespace" state={sort} onToggle={onSort} />
                <SortHeader label="Name" sortKey="name" state={sort} onToggle={onSort} />
                <th className="px-3 py-2 font-medium">{t('services.ports')}</th>
                {COUNTS.map(([k, label]) => (
                  <SortHeader key={k} label={t(label)} sortKey={k} state={sort} onToggle={onSort} />
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {rows.slice(0, shown).map((s) => (
                <tr key={`${s.namespace}/${s.name}`} className="row-hover">
                  <td className="px-3 py-1.5 text-muted">{s.namespace}</td>
                  <td className="px-3 py-1.5">
                    <Link to={`/services/${s.namespace}/${s.name}`} className="font-medium text-accent hover:underline">
                      {s.name}
                    </Link>
                  </td>
                  <td className="px-3 py-1.5 font-mono text-xs text-muted">{s.ports.join(', ') || '—'}</td>
                  {COUNTS.map(([k]) => {
                    const n = s[k] ?? 0
                    return (
                      <td key={k} className="px-3 py-1.5 tabular-nums">
                        {n === 0 ? (
                          <span className="text-muted">—</span>
                        ) : k === 'findings' ? (
                          <span className="chip bg-red-500/10 text-red-600 dark:text-red-400" title={t('services.findingsTitle', { n })}>
                            {n}
                          </span>
                        ) : (
                          n
                        )}
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {rows.length > shown && (
          <button type="button" className="btn-ghost mx-auto block" onClick={() => setShown((n) => n + PAGE)}>
            {t('list.showMore', { shown, total: rows.length })}
          </button>
        )}
        </>
      ) : null}
    </div>
  )
}
