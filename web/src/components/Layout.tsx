import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useCapabilities } from '../api/capabilities'
import { useResourceTypes } from '../api/resourceTypes'
import { getCluster } from '../api/client'
import { useTheme } from '../ui/theme'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { t, locale, setLocale, type Key } from '../i18n'

const PAGE_NAMES: Record<string, Key> = {
  '/': 'nav.home',
  '/overview': 'nav.overview',
  '/flowmap': 'nav.flowmap',
  '/routecheck': 'nav.routecheck',
  '/settings': 'nav.settings',
  '/guard': 'nav.guard',
  '/analysis': 'nav.analysis',
  '/services': 'nav.services',
}

export function Layout({ children }: { children: ReactNode }) {
  const { data } = useCapabilities()
  const cluster = getCluster() || 'local'
  const types = useResourceTypes().data
  const { theme, toggle } = useTheme()
  const location = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)
  const drawer = useRef<HTMLDialogElement>(null)
  const menuButton = useRef<HTMLButtonElement>(null)
  const resource = types?.find((rt) => location.pathname.split('/')[2] === rt.typeId)
  const pageKey = PAGE_NAMES[location.pathname] ?? (location.pathname.startsWith('/services/') ? 'nav.services' : undefined)
  const page = pageKey ? t(pageKey) : (resource?.kind ?? t('nav.resource'))

  useEffect(() => {
    if (menuOpen) drawer.current?.showModal()
    else drawer.current?.close()
  }, [menuOpen])
  useEffect(() => {
    document.title = `${page} · Sealane`
  }, [page])

  function closeMenu() {
    setMenuOpen(false)
    menuButton.current?.focus()
  }

  return (
    <div className="app-shell">
      <a
        href="#main-content"
        className="sr-only z-50 rounded bg-white p-3 text-accent focus:not-sr-only focus:absolute focus:left-3 focus:top-3"
      >
        {t('layout.skipToContent')}
      </a>
      <div className="hidden shrink-0 md:block">
        <Sidebar />
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="app-header">
          <button
            ref={menuButton}
            className="icon-button ml-2 md:hidden"
            aria-label={t('layout.openMenu')}
            aria-expanded={menuOpen}
            aria-controls="mobile-navigation"
            onClick={() => setMenuOpen(true)}
          >
            <Icon name="list" />
          </button>
          <Link to="/" className="app-brand md:hidden" aria-label={t('layout.brandHome')}>
            <img src="/favicon.svg" alt="" className="h-7 w-7 shrink-0" />
            <span className="text-[15px] font-bold text-strong">Sealane</span>
          </Link>
          <nav aria-label={t('layout.breadcrumb')} className="hidden min-w-0 items-center gap-2 px-6 text-[13px] md:flex">
            <span className="text-muted">{cluster}</span>
            <span className="text-faint">/</span>
            <span className="truncate font-semibold text-strong">{page}</span>
          </nav>
          <div className="ml-auto flex items-center gap-1 pr-3 md:pr-5">
            {data?.devMode && (
              <span className="chip hidden bg-amber-50 text-amber-700 sm:inline-flex dark:bg-amber-500/10 dark:text-amber-300">
                {t('layout.devMode')}
              </span>
            )}
            <button
              onClick={() => setLocale(locale === 'ko' ? 'en' : 'ko')}
              title={t('lang.switchLabel')}
              lang={locale === 'ko' ? 'en' : 'ko'}
              className="icon-button w-auto whitespace-nowrap px-2 text-xs font-semibold"
            >
              {t('lang.switch')}
            </button>
            <button
              onClick={toggle}
              aria-label={theme === 'dark' ? t('theme.light') : t('theme.dark')}
              title={theme === 'dark' ? t('theme.light') : t('theme.dark')}
              className="icon-button"
            >
              <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="h-4 w-4" />
            </button>
          </div>
        </header>
        <main id="main-content" tabIndex={-1} className="app-main">
          {children}
        </main>
      </div>
      <dialog
        ref={drawer}
        id="mobile-navigation"
        className="mobile-drawer"
        aria-label={t('layout.navMenu')}
        onCancel={closeMenu}
        onClose={() => setMenuOpen(false)}
        onClick={(e) => {
          if (e.target === e.currentTarget && e.clientX > e.currentTarget.getBoundingClientRect().right)
            closeMenu()
        }}
      >
        <Sidebar onNavigate={closeMenu} onClose={closeMenu} />
      </dialog>
    </div>
  )
}
