import type { AuditEntry } from '../api/audit'
import type { Key } from '../i18n'

export const VERB_LABEL: Record<AuditEntry['verb'], Key> = {
  create: 'home.verb.create',
  update: 'home.verb.update',
  delete: 'home.verb.delete',
}
export const VERB_TINT: Record<AuditEntry['verb'], string> = {
  create: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400',
  update: 'bg-cyan-50 text-cyan-800 dark:bg-cyan-500/10 dark:text-cyan-300',
  delete: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400',
}

// "5 min. ago" / "5분 전" for lists; the full time goes in a tooltip.
export const timeAgo = (iso: string, lang: string) => {
  const s = (new Date(iso).getTime() - Date.now()) / 1000
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto', style: 'narrow' })
  for (const [unit, sec] of [['day', 86400], ['hour', 3600], ['minute', 60]] as const)
    if (Math.abs(s) >= sec) return rtf.format(Math.round(s / sec), unit)
  return rtf.format(0, 'minute')
}

export const formatTime = (iso: string) => {
  const d = new Date(iso)
  return d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString() : d.toLocaleString()
}
