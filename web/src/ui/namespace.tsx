import { createContext, useContext, useState, type ReactNode } from 'react'

// Global namespace selection (header dropdown). '' = all namespaces. Drives the
// sidebar count badges and the list/overview pages so one control filters the app.
interface NamespaceCtx {
  ns: string
  setNs: (ns: string) => void
}

const Ctx = createContext<NamespaceCtx>({ ns: '', setNs: () => {} })
const KEY = 'sealane-ns'

export function NamespaceProvider({ children }: { children: ReactNode }) {
  const [ns, setNsState] = useState<string>(() => localStorage.getItem(KEY) ?? '')
  const setNs = (v: string) => { setNsState(v); localStorage.setItem(KEY, v) }
  return <Ctx.Provider value={{ ns, setNs }}>{children}</Ctx.Provider>
}

export function useNamespace() {
  return useContext(Ctx)
}
