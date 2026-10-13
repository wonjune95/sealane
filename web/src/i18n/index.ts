import en from './en.json'
import ko from './ko.json'

export type Locale = 'en' | 'ko'
export type Key = keyof typeof en
const KEY = 'sealane-locale'
const DICT: Record<Locale, Record<string, string>> = { en, ko }

function detect(): Locale {
  try {
    const saved = localStorage.getItem(KEY)
    if (saved === 'en' || saved === 'ko') return saved
  } catch {
    /* storage unavailable */
  }
  return navigator.language?.toLowerCase().startsWith('ko') ? 'ko' : 'en'
}

// Resolved once per page load. Switching writes the choice and reloads (same
// pattern as cluster switching), so no provider/context is needed.
export const locale: Locale = detect()

export function t(key: Key, vars?: Record<string, string | number>): string {
  let s = DICT[locale][key] ?? DICT.en[key] ?? key
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
  return s
}

// Untyped accessor for dynamic keys (form help/example text); t() stays typed.
export function lookup(key: string): string | undefined {
  return DICT[locale][key] ?? DICT.en[key]
}

export function setLocale(next: Locale) {
  try {
    localStorage.setItem(KEY, next)
  } catch {
    /* storage unavailable */
  }
  window.location.reload()
}
