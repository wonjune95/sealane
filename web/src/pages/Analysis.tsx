import { useState } from 'react'
import { useAnalysis } from '../api/analyze'
import { useNamespaces } from '../api/namespaces'
import { useNamespace } from '../ui/namespace'
import { PageHeader } from '../components/PageHeader'
import { FindingList } from '../components/FindingList'
import { Icon } from '../components/icons'
import { t } from '../i18n'

type Filter = 'all' | 'error' | 'warning'

export function Analysis() {
  const { ns, setNs } = useNamespace()
  const namespaces = useNamespaces()
  const { data, isLoading, error, isFetching, refetch } = useAnalysis(ns)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const q = search.toLowerCase()
  const filtered = (data?.findings ?? []).filter(
    (f) =>
      (filter === 'all' || f.severity === filter) &&
      (!q || [f.message, f.code, f.resource.name].some((s) => s.toLowerCase().includes(q))),
  )
  const filters: [Filter, string][] = [
    ['all', t('analysis.all')],
    ['error', t('analysis.errors')],
    ['warning', t('analysis.warnings')],
  ]

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <PageHeader
        eyebrow="Mesh analysis"
        title={
          <>
            {t('analysis.title')}
            <span className="nav-count !text-xs">{data ? data.findings.length : '—'}</span>
          </>
        }
        description={t('analysis.description')}
        actions={
          <button className="btn-ghost" onClick={() => void refetch()} disabled={isFetching}>
            <Icon name="refresh" className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} /> {t('analysis.refresh')}
          </button>
        }
      />
      <div className="panel flex flex-wrap items-center gap-3 p-3">
        <label className="relative min-w-0 flex-1 basis-full sm:min-w-[14rem] sm:max-w-sm sm:basis-auto">
          <Icon name="search" className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-faint" />
          <input
            aria-label={t('analysis.search')}
            placeholder={t('analysis.search')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input-base pl-9"
          />
        </label>
        <label className="flex shrink-0 items-center gap-2 text-xs text-muted">
          <span>Namespace</span>
          <select className="input-base !w-auto max-w-56" value={ns} onChange={(e) => setNs(e.target.value)}>
            <option value="">{t('list.allNamespaces')}</option>
            {(namespaces.data ?? []).map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
        <div className="flex gap-1">
          {filters.map(([value, label]) => (
            <button
              key={value}
              className={filter === value ? 'btn-primary' : 'btn-ghost'}
              aria-pressed={filter === value}
              onClick={() => setFilter(value)}
            >
              {label}
            </button>
          ))}
        </div>
        {data && <span className="ml-auto text-xs text-muted">{t('analysis.checked', { n: data.checked })}</span>}
      </div>
      {data?.skipped?.includes('pods') && (
        <div className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          <Icon name="info" className="h-4 w-4" />
          {t('analysis.skippedPods')}
        </div>
      )}
      {error && (
        <div
          className="flex flex-wrap items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-600 dark:text-red-400"
          role="alert"
        >
          <Icon name="warning" />
          {t('analysis.loadFailed', { message: error.message })}
          <button className="btn-ghost" onClick={() => void refetch()}>
            {t('common.retry')}
          </button>
        </div>
      )}
      {isLoading ? (
        <div className="p-12 text-center text-sm text-muted" role="status">
          {t('common.loading')}
        </div>
      ) : data ? (
        <FindingList findings={filtered} emptyTitle={t('analysis.empty')} emptyHint={t('analysis.emptyHint')} />
      ) : null}
    </div>
  )
}
