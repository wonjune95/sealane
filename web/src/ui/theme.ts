import { useSyncExternalStore } from 'react'

export type Theme = 'dark' | 'light'
const KEY = 'sealane-theme'
const EVENT = 'sealane-theme-change'

export function initialTheme(): Theme {
  return localStorage.getItem(KEY) === 'dark' ? 'dark' : 'light'
}
export function applyTheme(t: Theme) {
  document.documentElement.classList.toggle('dark', t === 'dark')
}
function subscribe(onChange: () => void) {
  const sync = () => {
    applyTheme(initialTheme())
    onChange()
  }
  window.addEventListener(EVENT, sync)
  window.addEventListener('storage', sync)
  return () => {
    window.removeEventListener(EVENT, sync)
    window.removeEventListener('storage', sync)
  }
}
// Share the theme with all consumers, including the mounted YAML editor.
export function useTheme() {
  const theme = useSyncExternalStore(subscribe, initialTheme)
  return {
    theme,
    toggle: () => {
      const next = initialTheme() === 'dark' ? 'light' : 'dark'
      localStorage.setItem(KEY, next)
      applyTheme(next)
      window.dispatchEvent(new Event(EVENT))
    },
  }
}
