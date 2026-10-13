import type { WidgetProps } from '@rjsf/utils'
import { Link } from 'react-router-dom'
import { useReferenceOptions, type RefKind } from '../api/references'
import { t, type Key } from '../i18n'

const UNKNOWN_HINT: Record<RefKind, Key> = {
  service: 'form.unknown.service',
  gateway: 'form.unknown.gateway',
  gatewayName: 'form.unknown.gatewayName',
  subset: 'form.unknown.subset',
}

// Reference-resolving input: a free-text field backed by a <datalist> of real
// cluster resources, with an existence badge and a "go to" link. Free text stays
// valid (no <select>) so external hosts / cross-ns refs aren't blocked.
export function ReferenceWidget(props: WidgetProps) {
  const { id, value, onChange, onBlur, onFocus, placeholder, disabled, readonly, options, formContext, registry } = props
  const refKind = (options?.refKind as RefKind) ?? 'service'
  // rjsf v6 exposes formContext via registry; older path kept as fallback.
  const ctx = (registry?.formContext ?? formContext) as { ns?: string } | undefined
  const ns = ctx?.ns || ''
  const query = useReferenceOptions(refKind, ns)
  const opts = query.data ?? []

  const v = (value as string) ?? ''
  const matched = opts.find((o) => o.value === v || o.aliases?.includes(v))
  const known = !!matched || (refKind === 'gateway' && v === 'mesh')
  const listId = `${id}-ref`

  return (
    <div>
      <input
        id={id}
        list={listId}
        className="input-base"
        value={v}
        placeholder={placeholder}
        disabled={disabled || readonly}
        autoComplete="off"
        onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
        onBlur={(e) => onBlur?.(id, e.target.value)}
        onFocus={(e) => onFocus?.(id, e.target.value)}
      />
      <datalist id={listId}>
        {opts.map((o, i) => (
          <option key={i} value={o.value}>{o.label}</option>
        ))}
      </datalist>
      {v && (
        <div className="mt-1 flex items-center gap-2 text-xs">
          {known ? (
            <span className="text-emerald-600 dark:text-emerald-400">✓ {t('form.verified')}</span>
          ) : (
            <span className="text-amber-600 dark:text-amber-400">⚠ {t(UNKNOWN_HINT[refKind])}</span>
          )}
          {matched?.navTypeId && (
            <Link
              to={`/resources/${matched.navTypeId}/${matched.navNs}/${matched.navName}`}
              className="text-accent hover:underline"
            >
              ↗ {t('form.goTo')}
            </Link>
          )}
        </div>
      )}
    </div>
  )
}
