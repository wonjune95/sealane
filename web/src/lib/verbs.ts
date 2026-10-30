import type { AuditEntry } from '../api/audit'
import type { Key } from '../i18n'

export const VERB_LABEL: Record<AuditEntry['verb'], Key> = {
  create: 'home.verb.create',
  update: 'home.verb.update',
  delete: 'home.verb.delete',
}
export const VERB_TINT: Record<AuditEntry['verb'], string> = {
  create: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400',
  update: 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300',
  delete: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400',
}

export const formatTime = (iso: string) => {
  const d = new Date(iso)
  return d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString() : d.toLocaleString()
}
