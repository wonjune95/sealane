import type { ArrayFieldTemplateProps, ArrayFieldItemTemplateProps } from '@rjsf/utils'
import { t } from '../i18n'

// rjsf v6: items are already-rendered elements (each via ArrayFieldItemTemplate).
// We just stack them and provide a clean "+ Add" button. Label/help come from
// the surrounding FieldTemplate.
export function ArrayFieldTemplate(props: ArrayFieldTemplateProps) {
  const { items, canAdd, onAddClick } = props
  return (
    <div className="space-y-1.5">
      {items}
      {canAdd && (
        <button
          type="button"
          onClick={onAddClick}
          className="rounded-md border border-dashed border-base px-2.5 py-1 text-xs font-medium text-accent hover:bg-accent-soft"
        >
          + {t('form.add')}
        </button>
      )}
    </div>
  )
}

// One array item: the field content + a compact Delete button (replaces rjsf's
// default empty Bootstrap-glyphicon buttons).
export function ArrayFieldItemTemplate(props: ArrayFieldItemTemplateProps) {
  const { children, buttonsProps } = props
  return (
    <div className="flex items-start gap-2">
      <div className="min-w-0 flex-1">{children}</div>
      {buttonsProps.hasRemove && (
        <button
          type="button"
          onClick={buttonsProps.onRemoveItem}
          className="mt-1 shrink-0 rounded border border-base px-2 py-1 text-xs text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10"
        >
          {t('common.delete')}
        </button>
      )}
    </div>
  )
}
