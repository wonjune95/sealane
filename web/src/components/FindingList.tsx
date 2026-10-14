import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import type { Finding, Ref } from '../api/analyze'
import { EmptyState } from './EmptyState'
import { t } from '../i18n'

const href = (r: Ref) => `/resources/${r.typeId}/${r.namespace || '-'}/${r.name}`

function RefLink({ r }: { r: Ref }) {
  return (
    <Link to={href(r)} className="font-medium text-accent hover:underline">
      {r.kind} {r.name}
      {r.namespace && <span className="text-muted"> {r.namespace}</span>}
    </Link>
  )
}

export function FindingList({
  findings,
  emptyTitle,
  emptyHint,
}: {
  findings: Finding[]
  emptyTitle: string
  emptyHint: string
}) {
  if (findings.length === 0) return <EmptyState icon="check" title={emptyTitle} description={emptyHint} />
  return (
    <div className="panel table-scroll">
      <table className="data-table">
        <thead className="border-b border-base bg-slate-50 text-left text-xs uppercase tracking-wide text-muted dark:bg-slate-800/50">
          <tr>
            <th className="w-24 whitespace-nowrap px-3 py-2">{t('analysis.severity')}</th>
            <th className="px-3 py-2">{t('analysis.finding')}</th>
            <th className="px-3 py-2">{t('analysis.resource')}</th>
            <th className="px-3 py-2">{t('analysis.related')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
          {findings.map((f, i) => (
            <tr key={i} className="row-hover align-top">
              <td className="px-3 py-1.5">
                <span
                  className={`whitespace-nowrap rounded-full border px-2 py-0.5 text-xs font-medium ${
                    f.severity === 'error'
                      ? 'border-red-500/20 bg-red-500/5 text-red-600 dark:text-red-400'
                      : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
                  }`}
                >
                  {t(f.severity === 'error' ? 'analysis.severity.error' : 'analysis.severity.warning')}
                </span>
              </td>
              <td className="px-3 py-1.5">
                {f.message} <span className="block font-mono text-xs text-faint sm:ml-1 sm:inline">{f.code}</span>
              </td>
              <td className="px-3 py-1.5">
                <RefLink r={f.resource} />
              </td>
              <td className="px-3 py-1.5">
                {f.related?.map((r, j) => (
                  <Fragment key={j}>
                    {j > 0 && ', '}
                    <RefLink r={r} />
                  </Fragment>
                ))}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
