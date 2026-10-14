import { t } from '../i18n'

// Building blocks shared by the curated forms.
export const input = 'input-base disabled:bg-gray-100 dark:disabled:bg-slate-800'

// Same required convention as the auto-form's FieldTemplate: red * vs (optional).
function RequiredMark({ required }: { required?: boolean }) {
  return required
    ? <span className="ml-1 text-red-500" title={t('form.required')}>*</span>
    : <span className="ml-1 text-[10px] font-normal text-faint">{t('form.optional')}</span>
}

export function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-muted">{label}<RequiredMark required={required} /></span>
      {children}
    </label>
  )
}

export function Section({ title, required, onAdd, children }: { title: string; required?: boolean; onAdd: () => void; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-strong">{title}<RequiredMark required={required} /></h3>
        <button onClick={onAdd} className="text-sm text-accent hover:underline">+ {t('form.add')}</button>
      </div>
      <div className="space-y-2">{children}</div>
    </div>
  )
}

export function Row({ children, onRemove }: { children: React.ReactNode; onRemove: () => void }) {
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1">{children}</div>
      <button onClick={onRemove} className="text-sm text-red-600 hover:underline dark:text-red-400">×</button>
    </div>
  )
}
