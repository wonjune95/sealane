import type { WidgetProps } from '@rjsf/utils'
import { ReferenceWidget } from './ReferenceWidget'
import { t } from '../i18n'

// Tailwind-styled rjsf widgets (Gemini ①): replaces the bare default HTML inputs.
// .input-base is the shared dark-aware, accent-focused input class (index.css).
const inputCls = 'input-base'

export function TextWidget(props: WidgetProps) {
  const { id, value, onChange, onBlur, onFocus, placeholder, schema, disabled, readonly } = props
  const numeric = schema.type === 'number' || schema.type === 'integer'
  return (
    <input
      id={id}
      type={numeric ? 'number' : 'text'}
      className={inputCls}
      value={value ?? ''}
      placeholder={placeholder}
      disabled={disabled || readonly}
      onChange={(e) => {
        const v = e.target.value
        onChange(v === '' ? undefined : numeric ? Number(v) : v)
      }}
      onBlur={(e) => onBlur?.(id, e.target.value)}
      onFocus={(e) => onFocus?.(id, e.target.value)}
    />
  )
}

export function SelectWidget(props: WidgetProps) {
  const { id, value, onChange, options, disabled, readonly, placeholder } = props
  const enumOptions = (options?.enumOptions ?? []) as { value: unknown; label: string }[]
  return (
    <select
      id={id}
      className={inputCls}
      value={value ?? ''}
      disabled={disabled || readonly}
      onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
    >
      <option value="">{placeholder || t('form.selectPlaceholder')}</option>
      {enumOptions.map((o, i) => (
        <option key={i} value={String(o.value)}>{o.label}</option>
      ))}
    </select>
  )
}

export function CheckboxWidget(props: WidgetProps) {
  const { id, value, onChange, label, disabled, readonly } = props
  return (
    <label className="inline-flex items-center gap-2 text-sm text-strong">
      <input
        id={id}
        type="checkbox"
        className="rounded border-base text-accent focus:ring-accent"
        checked={!!value}
        disabled={disabled || readonly}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label}
    </label>
  )
}

export function TextareaWidget(props: WidgetProps) {
  const { id, value, onChange, placeholder, disabled, readonly } = props
  return (
    <textarea
      id={id}
      rows={4}
      className={`${inputCls} font-mono`}
      value={value ?? ''}
      placeholder={placeholder}
      disabled={disabled || readonly}
      onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
    />
  )
}

export const widgets = { TextWidget, SelectWidget, CheckboxWidget, TextareaWidget, reference: ReferenceWidget }
