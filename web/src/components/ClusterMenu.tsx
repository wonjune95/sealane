import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { setCluster } from '../api/client'
import { Icon } from './icons'
import { t } from '../i18n'

// Header cluster switcher: a menu button styled like the rest of the app instead of a native
// <select>, whose dropdown list the browser draws and CSS cannot style.
export function ClusterMenu({
  clusters,
  current,
  connected,
  canManage,
}: {
  clusters: string[]
  current: string
  connected: boolean
  canManage: boolean
}) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const navigate = useNavigate()
  const items = () => [...(root.current?.querySelectorAll<HTMLElement>('[role^="menuitem"]') ?? [])]

  useEffect(() => {
    if (!open) return
    // Focus the checked cluster when the menu opens, like a native select does.
    ;(root.current?.querySelector<HTMLElement>('[aria-checked="true"]') ?? items()[0])?.focus()
    const onPointer = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', onPointer)
    return () => document.removeEventListener('pointerdown', onPointer)
  }, [open])

  function close() {
    setOpen(false)
    button.current?.focus()
  }
  function pick(name: string) {
    if (name === current) close()
    else setCluster(name) // reloads the app against the chosen cluster
  }
  function onMenuKey(e: KeyboardEvent) {
    const list = items()
    const i = list.indexOf(document.activeElement as HTMLElement)
    const go = (n: number) => list[(n + list.length) % list.length]?.focus()
    if (e.key === 'ArrowDown') go(i + 1)
    else if (e.key === 'ArrowUp') go(i - 1)
    else if (e.key === 'Home') go(0)
    else if (e.key === 'End') go(list.length - 1)
    else if (e.key === 'Escape') close()
    else if (e.key === 'Tab') setOpen(false)
    else return
    if (e.key !== 'Tab') e.preventDefault()
  }

  const item =
    'flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-xs hover:bg-slate-50 focus:bg-slate-50 focus:outline-none dark:hover:bg-slate-800 dark:focus:bg-slate-800'

  return (
    <div ref={root} className="relative hidden sm:block">
      <button
        ref={button}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls="cluster-menu"
        aria-label={`${t('layout.selectCluster')}: ${current}`}
        onClick={() => setOpen((o) => !o)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault()
            setOpen(true)
          }
        }}
        className={`flex items-center gap-2 rounded-lg border border-base py-1 pl-2.5 pr-2 text-left text-xs hover:bg-slate-50 dark:hover:bg-slate-800 ${open ? 'bg-slate-50 dark:bg-slate-800' : ''}`}
      >
        <Icon name="network" className="h-4 w-4 text-muted" />
        <span className="flex flex-col">
          <span className="hidden text-[9px] font-semibold uppercase leading-3 tracking-[.12em] text-muted md:block">
            {t('layout.cluster')}
          </span>
          <span className="flex items-center gap-1.5">
            <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${connected ? 'bg-emerald-500' : 'bg-slate-400'}`} />
            <span className="max-w-40 truncate font-medium leading-5 text-strong">{current}</span>
          </span>
        </span>
        <Icon name="down" className={`h-3.5 w-3.5 text-faint transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>
      {open && (
        <div
          id="cluster-menu"
          role="menu"
          aria-label={t('layout.selectCluster')}
          onKeyDown={onMenuKey}
          // One highlight at a time: the item under the pointer takes focus.
          onPointerMove={(e) => (e.target as HTMLElement).closest<HTMLElement>('[role^="menuitem"]')?.focus()}
          className="menu-panel max-h-80 overflow-y-auto"
        >
          <div className="px-3 pb-1.5 pt-1 text-[10px] font-semibold uppercase tracking-[.12em] text-muted">
            {t('layout.switchCluster')}
          </div>
          {clusters.map((name) => {
            const active = name === current
            return (
              <button
                key={name}
                type="button"
                role="menuitemradio"
                aria-checked={active}
                tabIndex={-1}
                onClick={() => pick(name)}
                className={`${item} ${active ? 'font-semibold text-strong' : 'text-slate-700 dark:text-slate-300'}`}
              >
                <Icon name="network" className={`h-4 w-4 shrink-0 ${active ? 'text-accent' : 'text-faint'}`} />
                <span className="min-w-0 flex-1 truncate">{name}</span>
                {active && <Icon name="check" className="h-4 w-4 shrink-0 text-accent" />}
              </button>
            )
          })}
          {canManage && (
            <>
              <div className="my-1.5 h-px bg-slate-100 dark:bg-slate-800" />
              <button
                type="button"
                role="menuitem"
                tabIndex={-1}
                onClick={() => {
                  setOpen(false)
                  navigate('/settings')
                }}
                className={`${item} text-slate-700 dark:text-slate-300`}
              >
                <Icon name="settings" className="h-4 w-4 shrink-0 text-faint" />
                {t('layout.manageClusters')}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
