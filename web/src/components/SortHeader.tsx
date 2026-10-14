import type { SortState } from '../lib/sort'
import { t } from '../i18n'

export function SortHeader({
  label,
  sortKey,
  state,
  onToggle,
}: {
  label: string
  sortKey: string
  state: SortState | null
  onToggle: (key: string) => void
}) {
  const on = state?.key === sortKey ? state.dir : null
  return (
    <th
      className="px-3 py-2 font-medium"
      aria-sort={on === 'asc' ? 'ascending' : on === 'desc' ? 'descending' : 'none'}
    >
      <button
        type="button"
        onClick={() => onToggle(sortKey)}
        aria-label={t('list.sortBy', { column: label })}
        className="inline-flex items-center gap-1 uppercase tracking-wide hover:text-accent"
      >
        {label}
        <span aria-hidden="true" className={on ? '' : 'opacity-30'}>
          {on === 'desc' ? '▼' : '▲'}
        </span>
      </button>
    </th>
  )
}
