import type { FieldTemplateProps } from '@rjsf/utils'
import { t } from '../i18n'

// Custom field layout (Gemini ③): required "*" vs "(optional)", localized help under the
// label, consistent spacing.
//   - scalar & array fields → show the label here (our Array/Object templates
//     don't render their own titles),
//   - object fields → label lives in the collapsible <summary>, so skip it,
//   - array items (id ending in _<index>) → no per-item label.
export function FieldTemplate(props: FieldTemplateProps) {
  const { id, label, required, children, help, errors, hidden, schema, uiSchema } = props
  const recommended = (uiSchema?.['ui:options'] as { recommended?: boolean } | undefined)?.recommended

  if (hidden) return <div className="hidden">{children}</div>

  const isArrayItem = /_\d+$/.test(id)
  const isObject = schema?.type === 'object'
  const isArray = schema?.type === 'array'
  const showLabel = !!label && !isArrayItem && !isObject
  // Scalar leaf fields → 2-column (label left, widget right). Object/array
  // containers stay full-width so nested content isn't cramped.
  const twoCol = showLabel && !isArray

  if (isArrayItem)
    return (
      <div className="min-w-0">
        {children}
        {errors}
      </div>
    )

  const labelEl = showLabel && (
    <label htmlFor={id} className="flex items-center gap-1 text-sm font-medium text-strong">
      {label}
      {required ? (
        <span className="text-red-500" title={t('form.required')}>
          *
        </span>
      ) : recommended ? (
        <span
          className="text-[10px] font-normal text-amber-600 dark:text-amber-400"
          title={t('form.recommendedHint')}
        >
          {t('form.recommended')}
        </span>
      ) : (
        <span className="text-[10px] font-normal text-faint">{t('form.optional')}</span>
      )}
    </label>
  )

  if (twoCol) {
    return (
      <div className="mb-4 flex flex-col sm:flex-row items-start gap-2 sm:gap-4">
        <div className="w-full sm:w-1/3 min-w-0 pt-1.5">
          {labelEl}
          {help && <div className="mt-0.5 text-xs text-faint">{help}</div>}
        </div>
        <div className="w-full sm:w-2/3 min-w-0">
          {children}
          {errors}
        </div>
      </div>
    )
  }

  return (
    <div className="mb-3">
      {labelEl}
      {help && <div className="mb-1 mt-0.5 text-xs text-faint">{help}</div>}
      {children}
      {errors}
    </div>
  )
}
