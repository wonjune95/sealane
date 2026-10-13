import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'

import { Icon } from './icons'
import { t } from '../i18n'

type ToastKind = 'success' | 'error' | 'info'
interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const Ctx = createContext<((kind: ToastKind, message: string) => void) | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const push = useCallback((kind: ToastKind, message: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, kind, message }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5000)
  }, [])
  return (
    <Ctx.Provider value={push}>
      {children}
      <div
        aria-live="polite"
        aria-atomic="false"
        className="fixed bottom-5 right-5 z-50 w-96 max-w-[calc(100vw-40px)] space-y-2"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            role={toast.kind === 'error' ? 'alert' : 'status'}
            className="panel flex items-start gap-3 rounded-xl p-4 text-xs leading-5 shadow-lg"
          >
            <Icon
              name={toast.kind === 'success' ? 'check' : toast.kind === 'error' ? 'warning' : 'info'}
              className={`mt-0.5 h-4 w-4 shrink-0 ${toast.kind === 'success' ? 'text-emerald-600' : toast.kind === 'error' ? 'text-red-500' : 'text-accent'}`}
            />
            <span className="min-w-0 flex-1 break-words text-strong">{toast.message}</span>
            <button
              aria-label={t('toast.close')}
              onClick={() => setToasts((current) => current.filter((x) => x.id !== toast.id))}
              className="text-muted hover:text-strong"
            >
              <Icon name="close" className="h-4 w-4" />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}

export function useToast() {
  const push = useContext(Ctx)
  if (!push) throw new Error('useToast must be used within ToastProvider')
  return push
}
