import type { Category } from '../api/resourceTypes'

// Per-category accent colors (Gemini: visual anchoring). Full literal class
// strings (incl. dark: variants) so Tailwind's JIT scanner picks them up — never
// build these by string interpolation or airgap builds drop the styles.
export interface Accent {
  text: string
  bg: string
  border: string
  dot: string
  badge: string // solid kind badge (white text)
}

export const ACCENT: Record<Category, Accent> = {
  'gateway-api': {
    text: 'text-cyan-700 dark:text-cyan-400',
    bg: 'bg-cyan-50 dark:bg-cyan-500/10',
    border: 'border-cyan-200 dark:border-cyan-500/20',
    dot: 'bg-cyan-500',
    badge: 'bg-[#0b8a83] text-white',
  },
  traffic: {
    text: 'text-blue-700 dark:text-blue-400',
    bg: 'bg-blue-50 dark:bg-blue-500/10',
    border: 'border-blue-200 dark:border-blue-500/20',
    dot: 'bg-blue-500',
    badge: 'bg-[#2f6fdf] text-white',
  },
  security: {
    text: 'text-amber-700 dark:text-amber-400',
    bg: 'bg-amber-50 dark:bg-amber-500/10',
    border: 'border-amber-200 dark:border-amber-500/20',
    dot: 'bg-amber-600',
    badge: 'bg-[#a15c00] text-white',
  },
  telemetry: {
    text: 'text-slate-600 dark:text-slate-400',
    bg: 'bg-slate-100 dark:bg-slate-500/10',
    border: 'border-slate-200 dark:border-slate-500/20',
    dot: 'bg-slate-500',
    badge: 'bg-[#5b6876] text-white',
  },
}

export const CATEGORY_ORDER: Category[] = ['gateway-api', 'traffic', 'security', 'telemetry']

export const CATEGORY_ICON: Record<Category, string> = {
  'gateway-api': 'grid',
  traffic: 'bolt',
  security: 'shield',
  telemetry: 'chart',
}

// categoryOf derives a kind's category from its typeId's API group.
export function categoryOf(typeId: string): Category | undefined {
  const group = typeId.slice(typeId.indexOf('.') + 1)
  if (group === 'gateway.networking.k8s.io') return 'gateway-api'
  if (group === 'networking.istio.io') return 'traffic'
  if (group === 'security.istio.io') return 'security'
  if (group === 'telemetry.istio.io') return 'telemetry'
  return undefined
}

export function accentFor(cat?: Category): Accent {
  return (cat && ACCENT[cat]) || ACCENT.telemetry
}
