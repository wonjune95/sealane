import { useState } from 'react'
import { diffLines } from 'diff'
import { Modal } from './Modal'
import { Icon } from './icons'
import { t } from '../i18n'

// "kubectl diff"-style preview before applying: shows line-level changes between
// the current object and the dry-run result. For dangerous kinds, requires typing
// the resource name to confirm.
export function DiffModal({
  oldYaml,
  newYaml,
  name,
  dangerous,
  busy,
  title = t('diff.title'),
  subtitle = t('diff.subtitle'),
  confirmLabel = t('diff.confirm'),
  warn = false,
  onConfirm,
  onCancel,
}: {
  oldYaml: string
  newYaml: string
  name: string
  dangerous: boolean
  busy: boolean
  title?: string
  subtitle?: string
  confirmLabel?: string
  warn?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const [typed, setTyped] = useState('')
  const parts = diffLines(oldYaml, newYaml)
  const noChange = parts.every((p) => !p.added && !p.removed)
  const confirmOk = (!dangerous || typed === name) && !busy

  return (
    <Modal label={title} onCancel={onCancel} busy={busy}>
      <div className="panel flex max-h-[85dvh] w-full flex-col rounded-xl shadow-xl">
        <div className="flex items-start justify-between gap-3 border-b border-base px-5 py-4">
          <div>
            <h3
              className={`text-base font-semibold ${warn ? 'text-amber-600 dark:text-amber-400' : 'text-strong'}`}
            >
              {title}
            </h3>
            <p className="mt-1 text-xs text-muted">{subtitle}</p>
          </div>
          <button
            onClick={onCancel}
            disabled={busy}
            className="icon-button -mr-2 -mt-1"
            aria-label={t('diff.close')}
          >
            <Icon name="close" className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto p-4">
          {noChange ? (
            <div className="py-8 text-center text-sm text-faint">{t('diff.noChanges')}</div>
          ) : (
            <pre className="overflow-x-auto rounded-lg border border-base bg-slate-50 p-3 text-xs leading-5 dark:bg-slate-950">
              {parts.flatMap((p, pi) =>
                p.value
                  .replace(/\n$/, '')
                  .split('\n')
                  .map((line, li) => {
                    const cls = p.added
                      ? 'bg-emerald-50 text-emerald-800 dark:bg-emerald-500/10 dark:text-emerald-300'
                      : p.removed
                        ? 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
                        : 'text-muted'
                    const sign = p.added ? '+' : p.removed ? '-' : ' '
                    return (
                      <div key={`${pi}-${li}`} className={`whitespace-pre ${cls}`}>
                        <span className="select-none opacity-50">{sign} </span>
                        {line}
                      </div>
                    )
                  }),
              )}
            </pre>
          )}
        </div>

        <div className="border-t border-base px-5 py-3">
          {dangerous && (
            <div className="mb-3">
              <p className="mb-1.5 text-xs text-red-600 dark:text-red-400">
                {t('diff.dangerBefore')}{' '}
                <code className="rounded bg-slate-100 px-1 dark:bg-slate-800">{name}</code>{' '}
                {t('diff.dangerAfter')}
              </p>
              <input
                autoFocus
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder={name}
                className="input-base"
              />
            </div>
          )}
          <div className="flex justify-end gap-2">
            <button onClick={onCancel} disabled={busy} className="btn-ghost text-sm">
              {t('common.cancel')}
            </button>
            <button
              onClick={onConfirm}
              disabled={!confirmOk}
              className={`rounded-md px-4 py-1.5 text-sm font-medium text-white disabled:opacity-40 ${dangerous || warn ? 'bg-red-600 hover:bg-red-700' : 'btn-primary'}`}
            >
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </Modal>
  )
}
