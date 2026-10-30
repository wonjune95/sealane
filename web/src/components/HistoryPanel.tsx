import { useState } from 'react'
import { dump as yamlDump } from 'js-yaml'
import { useHistory, getRevision, revisionObject } from '../api/history'
import { useCapabilities } from '../api/capabilities'
import type { AuditEntry } from '../api/audit'
import { ApiError, errorText } from '../api/client'
import { cleanManifest } from '../lib/download'
import { VERB_LABEL, VERB_TINT, formatTime } from '../lib/verbs'
import { DiffModal } from './DiffModal'
import { EmptyState } from './EmptyState'
import { useToast } from './Toast'
import { t } from '../i18n'

// Editor "History" tab: changes made to this resource through Sealane. Comparing a version
// with the current object and loading it into the editor is the restore path, so a restore
// goes through the usual dry-run, diff preview and conflict check on Apply.
export function HistoryPanel({
  type,
  ns,
  name,
  current,
  onLoad,
}: {
  type: string
  ns: string
  name: string
  current: unknown
  onLoad?: (obj: unknown, at: string) => void // absent for users who cannot update
}) {
  const history = useHistory(type, ns, name)
  // Stored versions go to editors and admins only (an old version can hold what was since removed).
  const role = useCapabilities().data?.role
  const canCompare = role === 'admin' || role === 'editor'
  const toast = useToast()
  const [opening, setOpening] = useState('')
  const [compare, setCompare] = useState<{ at: string; obj: unknown } | null>(null)

  async function open(rev: AuditEntry) {
    setOpening(rev.id ?? '')
    try {
      const d = await getRevision(rev.id ?? '')
      const obj = revisionObject(d)
      if (obj) setCompare({ at: d.time, obj })
      else toast('error', t('history.notKept'))
    } catch (e) {
      toast('error', e instanceof ApiError ? errorText(e) : String(e))
    } finally {
      setOpening('')
    }
  }

  return (
    <div className="panel">
      <p className="border-b border-base px-5 py-3 text-xs text-muted">{t('history.scope')}</p>
      {history.isLoading ? (
        <p className="px-5 py-8 text-sm text-muted">{t('home.auditLoading')}</p>
      ) : history.isError ? (
        <p role="alert" className="px-5 py-8 text-sm text-red-600 dark:text-red-400">
          {t('home.auditFailed')}
        </p>
      ) : !history.data?.length ? (
        <EmptyState icon="clock" title={t('history.emptyTitle')} description={t('history.emptyDescription')} />
      ) : (
        <ul className="divide-y divide-slate-100 px-5 dark:divide-slate-800">
          {history.data.map((rev, i) => (
            <li key={rev.id || i} className="flex flex-wrap items-center gap-3 py-3 text-xs">
              <span className={`rounded-md px-2 py-1 font-medium ${VERB_TINT[rev.verb]}`}>{t(VERB_LABEL[rev.verb])}</span>
              <span className="min-w-0 flex-1 text-muted">
                <time dateTime={rev.time} className="tabular-nums text-strong">
                  {formatTime(rev.time)}
                </time>
                {rev.user && ` · ${rev.user}`}
                {rev.user && rev.provider === 'oidc' && ` ${t('home.auditViaSso')}`}
                {!rev.ok && <span className="ml-2 text-red-500">{t('home.failed')}</span>}
              </span>
              {rev.ok && rev.id && canCompare && (
                <button
                  onClick={() => void open(rev)}
                  disabled={opening !== ''}
                  className="btn-ghost text-xs disabled:opacity-50"
                >
                  {t('history.compare')}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {compare && (
        <DiffModal
          title={t('history.compareTitle', { time: formatTime(compare.at) })}
          subtitle={t('history.compareSubtitle')}
          confirmLabel={t('history.load')}
          oldYaml={yamlDump(cleanManifest(current))}
          newYaml={yamlDump(cleanManifest(compare.obj))}
          name={name}
          dangerous={false}
          busy={false}
          onConfirm={
            onLoad
              ? () => {
                  onLoad(compare.obj, compare.at)
                  setCompare(null)
                }
              : undefined
          }
          onCancel={() => setCompare(null)}
        />
      )}
    </div>
  )
}
