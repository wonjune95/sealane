import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useResources, type ResourceSummary, type ResourceDetail } from '../api/resources'
import { useResourceTypes } from '../api/resourceTypes'
import { useNamespaces } from '../api/namespaces'
import { useNamespace } from '../ui/namespace'
import { apiGet, apiDelete, ApiError, errorText } from '../api/client'
import { useToast } from '../components/Toast'
import { downloadText, toManifestYaml } from '../lib/download'
import { Icon } from '../components/icons'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { DangerConfirm } from '../components/DangerConfirm'
import { useCapabilities } from '../api/capabilities'
import { SortHeader } from '../components/SortHeader'
import { PAGE, sortBy, toggleSort, parseAge, type SortState } from '../lib/sort'
import { t } from '../i18n'

export function ResourceList() {
  const { type = '' } = useParams()
  const { ns, setNs } = useNamespace()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SortState | null>(null)
  const [shown, setShown] = useState(PAGE)
  const [pendingDelete, setPendingDelete] = useState<ResourceSummary | null>(null)
  const caps = useCapabilities().data
  const canWrite = caps?.role === 'admin' || caps?.role === 'editor'
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const namespaces = useNamespaces()
  const types = useResourceTypes()
  const rt = types.data?.find((x) => x.typeId === type)
  const { data, isLoading, isError, error, isFetching } = useResources(type, ns)
  const queryClient = useQueryClient()
  const toast = useToast()

  const keyOf = (r: ResourceSummary) => `${r.namespace}/${r.name}`
  const rows = sortBy(
    (data ?? []).filter((r) =>
      `${r.name} ${r.namespace} ${r.summary} ${r.status ?? ''}`.toLowerCase().includes(search.toLowerCase()),
    ),
    sort,
    (r, k) => (k === 'age' ? parseAge(r.age) : k === 'summary' ? `${r.status ?? ''} ${r.summary}` : k === 'name' ? r.name : r.namespace),
  )
  const onSort = (k: string) => {
    setSort((s) => toggleSort(s, k))
    setShown(PAGE)
  }
  const allKeys = rows.slice(0, shown).map(keyOf)
  const allSelected = allKeys.length > 0 && allKeys.every((k) => selected.has(k))

  function toggle(k: string) {
    setSelected((s) => {
      const n = new Set(s)
      if (n.has(k)) n.delete(k)
      else n.add(k)
      return n
    })
  }
  function toggleAll() {
    setSelected((s) => {
      const next = new Set(s)
      for (const k of allKeys) {
        if (allSelected) next.delete(k)
        else next.add(k)
      }
      return next
    })
  }

  function onDelete(r: ResourceSummary) {
    if (!canWrite) return
    if (rt?.dangerous) {
      setPendingDelete(r)
      return
    }
    if (window.confirm(t('list.confirmDelete', { kind: r.kind, ns: r.namespace || '', name: r.name }))) void doDelete(r)
  }

  async function doDelete(r: ResourceSummary) {
    try {
      await apiDelete(`/api/resources/${type}/${r.namespace || '-'}/${r.name}`)
      toast('success', t('list.deleted'))
      queryClient.invalidateQueries({ queryKey: ['resources', type] })
    } catch (e) {
      toast('error', e instanceof ApiError ? `${e.status} ${e.reason}: ${errorText(e)}` : String(e))
    } finally {
      queryClient.invalidateQueries({ queryKey: ['audit'] })
    }
  }

  // Downloads the checked resources as clean, re-appliable manifests.
  async function downloadYaml() {
    const items = (data ?? []).filter((r) => selected.has(keyOf(r)))
    if (items.length === 0) return
    const docs: string[] = []
    for (const r of items) {
      try {
        const d = await apiGet<ResourceDetail>(`/api/resources/${type}/${r.namespace || '-'}/${r.name}`)
        docs.push(toManifestYaml(d.raw))
      } catch {
        /* skip unreadable */
      }
    }
    if (docs.length === 0) {
      toast('error', t('list.downloadReadFailed'))
      return
    }
    const file =
      items.length === 1 ? `${items[0].kind}-${items[0].name}.yaml` : `${rt?.kind ?? 'resources'}-export.yaml`
    downloadText(file, docs.join('---\n'))
    toast('success', t('list.downloaded', { n: docs.length }))
  }

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <PageHeader
        eyebrow={t('eyebrow.resources')}
        title={
          <>
            {rt?.kind ?? type}
            <span className="nav-count !text-xs">{data?.length ?? '—'}</span>
          </>
        }
        description={t('list.description', { kind: rt?.kind ?? t('list.resource') })}
        actions={
          <>
            <button
              onClick={downloadYaml}
              disabled={selected.size === 0}
              title={selected.size === 0 ? t('list.selectToDownload') : undefined}
              className="btn-ghost"
            >
              <Icon name="download" className="h-4 w-4" />
              {t('list.downloadYaml')}{selected.size > 0 ? ` (${selected.size})` : ''}
            </button>
            {canWrite && (
              <Link to={`/resources/${type}/new`} className="btn-primary">
                <Icon name="plus" className="h-4 w-4" />
                {t('list.newResource')}
              </Link>
            )}
          </>
        }
      />
      {rt?.dangerous && (
        <div className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          <Icon name="warning" className="h-4 w-4" />
          {t('list.dangerBanner')}
        </div>
      )}
      <div className="panel flex flex-wrap items-center gap-3 p-3">
        <label className="relative min-w-0 flex-1 basis-full sm:min-w-[14rem] sm:max-w-sm sm:basis-auto">
          <Icon name="search" className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-faint" />
          <input
            aria-label={t('list.searchLabel')}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setShown(PAGE)
            }}
            placeholder={t('list.searchPlaceholder')}
            className="input-base pl-9"
          />
        </label>
        {rt?.namespaced && (
          <label className="flex shrink-0 items-center gap-2 text-xs text-muted">
            <span>Namespace</span>
            <select
              className="input-base !w-auto max-w-56"
              value={ns}
              onChange={(e) => {
                setNs(e.target.value)
                setSelected(new Set())
              }}
            >
              <option value="">{t('list.allNamespaces')}</option>
              {(namespaces.data ?? []).map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
        )}
        <span className="ml-auto text-xs text-muted">
          {t('list.count', { n: rows.length })}
          {isFetching && ` · ${t('list.refreshing')}`}
        </span>
      </div>
      <div className="panel table-scroll">
        <table className="data-table">
          <thead className="border-b border-base bg-slate-50 text-left text-xs uppercase tracking-wide text-muted dark:bg-slate-800/50">
            <tr>
              <th className="w-8 px-3 py-2">
                <input
                  type="checkbox"
                  aria-label={t('list.selectAll')}
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded border-base text-accent focus:ring-accent"
                />
              </th>
              <SortHeader label="Namespace" sortKey="namespace" state={sort} onToggle={onSort} />
              <SortHeader label="Name" sortKey="name" state={sort} onToggle={onSort} />
              <SortHeader label="Summary" sortKey="summary" state={sort} onToggle={onSort} />
              <SortHeader label="Age" sortKey="age" state={sort} onToggle={onSort} />
              <Th />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {isLoading && <Msg>{t('common.loading')}</Msg>}
            {isError && <Msg>{t('list.loadFailed', { message: (error as Error).message })}</Msg>}
            {!isLoading && !isError && rows.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    icon="search"
                    title={search ? t('list.noResults') : t('list.noResources')}
                    description={
                      search ? t('list.noResultsHint') : t('list.emptyHint')
                    }
                  />
                </td>
              </tr>
            )}
            {rows.slice(0, shown).map((r) => {
              const k = keyOf(r)
              return (
                <tr key={k} className="row-hover">
                  <td className="px-3 py-1.5">
                    <input
                      type="checkbox"
                      aria-label={t('list.selectRow', { name: r.name })}
                      checked={selected.has(k)}
                      onChange={() => toggle(k)}
                      className="rounded border-base text-accent focus:ring-accent"
                    />
                  </td>
                  <Td className="text-muted">{r.namespace || '—'}</Td>
                  <Td>
                    <Link
                      to={`/resources/${type}/${r.namespace || '-'}/${r.name}`}
                      className="font-medium text-accent hover:underline"
                    >
                      {r.name}
                    </Link>
                  </Td>
                  <Td className="text-muted">
                    {r.status && <StatusPill status={r.status} />}
                    {r.summary || (r.status ? '' : '—')}
                  </Td>
                  <Td className="text-faint">{r.age}</Td>
                  <Td className="text-right">
                    {canWrite && (
                      <button
                        onClick={() => onDelete(r)}
                        aria-label={t('list.deleteRow', { name: r.name })}
                        title={t('common.delete')}
                        className="icon-button !h-7 !w-7 hover:!bg-red-50 hover:!text-red-600 dark:hover:!bg-red-500/10"
                      >
                        <Icon name="trash" className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </Td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
{rows.length > shown && (
        <button type="button" className="btn-ghost mx-auto block" onClick={() => setShown((n) => n + PAGE)}>
          {t('list.showMore', { shown: Math.min(shown, rows.length), total: rows.length })}
        </button>
      )}
      {pendingDelete && (
        <DangerConfirm
          name={pendingDelete.name}
          action={t('common.delete')}
          onCancel={() => setPendingDelete(null)}
          onConfirm={() => {
            void doDelete(pendingDelete)
            setPendingDelete(null)
          }}
        />
      )}
    </div>
  )
}

function Th({ children }: { children?: React.ReactNode }) {
  return <th className="px-3 py-2 font-medium">{children}</th>
}
function Td({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <td className={`px-3 py-1.5 ${className}`}>{children}</td>
}
function Msg({ children }: { children: React.ReactNode }) {
  return (
    <tr>
      <td colSpan={7} className="px-3 py-10 text-center text-faint">
        {children}
      </td>
    </tr>
  )
}

const PILL = {
  ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300',
  error: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300',
  warning: 'bg-amber-50 text-amber-800 dark:bg-amber-500/10 dark:text-amber-300',
}

function StatusPill({ status }: { status: string }) {
  const [kind, reason] = status.split(':')
  const cls = PILL[kind as keyof typeof PILL] ?? PILL.warning
  return (
    <span
      title={status}
      className={`mr-2 inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium ${cls}`}
    >
      {kind === 'ok' ? t('list.status.ok') : reason}
    </span>
  )
}
