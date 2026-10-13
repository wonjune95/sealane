import { useState } from 'react'
import type { ObjectFieldTemplateProps } from '@rjsf/utils'
import { t } from '../i18n'

/* eslint-disable @typescript-eslint/no-explicit-any */

// Collapses nested objects (native <details>) so deep Istio specs don't render
// hundreds of fields at once (Gemini ④). Fixes from Gemini review:
//  - controlled `open` (uncontrolled <details> snaps shut on rjsf re-render),
//  - lazy children (closed sections don't mount their fields → no typing lag),
//  - start open when the object already has data (existing resources / filled
//    sections / freshly added array items stay editable),
//  - start open when the object is required — a mandatory section shouldn't
//    hide behind a collapsed accordion.
export function CollapsibleObjectField(props: ObjectFieldTemplateProps) {
  const { title, properties, fieldPathId, required } = props
  const isRoot = fieldPathId.$id === 'root'
  const formData = (props as any).formData
  const hasData = formData && typeof formData === 'object' && Object.keys(formData).length > 0
  const [open, setOpen] = useState<boolean>(isRoot || !!hasData || !!required)

  const fields = (
    <div className={isRoot ? '' : 'border-l-2 border-slate-100 pl-3 pt-2 dark:border-slate-800'}>
      {properties.map((p) => (
        <div key={p.name} className="mb-1">{p.content}</div>
      ))}
    </div>
  )

  if (isRoot) return fields

  return (
    <details className="my-1" open={open} onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary className="cursor-pointer select-none rounded bg-slate-50 px-2 py-1 text-sm font-medium text-muted hover:bg-slate-100 dark:bg-slate-800/50 dark:hover:bg-slate-800">
        {title || 'object'}
        {/* same required convention as FieldTemplate (object fields skip its
            label). Array items get no mark — requiredness belongs to the array. */}
        {!/_\d+$/.test(fieldPathId.$id) && (required ? (
          <span className="ml-1 text-red-500" title={t('form.required')}>*</span>
        ) : (
          <span className="ml-1 text-[10px] font-normal text-faint">{t('form.optional')}</span>
        ))}
      </summary>
      {open && fields}
    </details>
  )
}
