import type { ManagedBy } from '../api/resources'
import { managedText, managedTool } from '../lib/managed'

export function ManagedBadge({ managedBy }: { managedBy: ManagedBy }) {
  return (
    <span
      title={managedText(managedBy)}
      className="ml-2 inline-block rounded bg-slate-100 px-1.5 py-0.5 align-middle text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300"
    >
      {managedTool(managedBy)}
    </span>
  )
}
