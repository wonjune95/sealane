import { useState } from 'react'
import { Modal } from './Modal'
import { Icon } from './icons'
import { t } from '../i18n'

// Typed-confirmation modal for high-risk kinds (Gemini review ⑦): the user must
// type the resource name to proceed.
export function DangerConfirm({
  name,
  action,
  onConfirm,
  onCancel,
}: {
  name: string
  action: string
  onConfirm: () => void
  onCancel: () => void
}) {
  const [typed, setTyped] = useState('')
  const ok = typed === name

  return (
    <Modal label={t('danger.title', { action })} onCancel={onCancel}>
      <div className="panel w-full max-w-md mx-auto rounded-xl p-6 shadow-xl">
        <span className="mb-4 flex h-10 w-10 items-center justify-center rounded-xl bg-red-50 text-red-600 dark:bg-red-500/10">
          <Icon name="warning" className="h-5 w-5" />
        </span>
        <h3 className="mb-2 text-base font-semibold text-red-600 dark:text-red-400">
          {t('danger.title', { action })}
        </h3>
        <p className="mb-3 text-sm text-muted">
          {t('danger.bodyBefore')}
          <code className="mx-1 rounded bg-slate-100 px-1 dark:bg-slate-800">{name}</code>
          {t('danger.bodyAfter')}
        </p>
        <input
          autoFocus
          className="input-base mb-4"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          aria-label={t('danger.inputLabel')}
          placeholder={name}
        />
        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="btn-ghost text-sm">
            {t('common.cancel')}
          </button>
          <button
            onClick={onConfirm}
            disabled={!ok}
            className="rounded bg-red-600 px-3 py-1.5 text-sm text-white hover:bg-red-700 disabled:opacity-40"
          >
            {action}
          </button>
        </div>
      </div>
    </Modal>
  )
}
