import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useCapabilities } from '../api/capabilities'
import { useClusters } from '../api/clusters'
import { useResourceTypes } from '../api/resourceTypes'
import { apiPost, getCluster, setCluster } from '../api/client'
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
const MANAGE = '_manage' // cluster names must start with a letter or digit, so this never collides

export function Layout({ children }: { children: ReactNode }) {
  const { data, isError } = useCapabilities()
  const clusters = useClusters().data
  const navigate = useNavigate()
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
        className="sr-only z-50 rounded-lg bg-white p-3 text-accent focus:not-sr-only focus:absolute focus:left-3 focus:top-3"
      >
        {t('layout.skipToContent')}
      </a>
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
        <Link to="/" className="app-brand" aria-label={t('layout.brandHome')}>
          <img src="/favicon.svg" alt="" className="h-8 w-8 shrink-0" />
          <span className="text-[15px] font-semibold tracking-tight text-strong">Sealane</span>
        </Link>
        <div className="hidden min-w-0 items-center gap-2.5 px-6 text-xs md:flex">
          <Icon name="globe" className="h-4 w-4 text-faint" />
          <span className="text-muted">{t('layout.workspace')}</span>
          <Icon name="chevron" className="h-3 w-3 text-faint" />
          <span className="truncate font-medium text-strong">{page}</span>
        </div>
        <div className="ml-auto flex items-center gap-1.5 pr-3 md:gap-3 md:pr-6">
          <label className="hidden items-center gap-2 rounded-lg border border-base py-1 pl-2.5 pr-2 text-xs hover:bg-slate-50 sm:flex dark:hover:bg-slate-800">
            <Icon name="network" className="h-4 w-4 text-muted" />
            <span className="flex flex-col">
              <span className="hidden text-[9px] font-semibold uppercase leading-3 tracking-[.12em] text-muted md:block">
                {t('layout.cluster')}
              </span>
              <span className="relative flex items-center gap-1.5">
                <span
                  className={`h-1.5 w-1.5 shrink-0 rounded-full ${data && !isError ? 'bg-emerald-500' : 'bg-slate-400'}`}
                />
                <select
                  aria-label={t('layout.selectCluster')}
                  value={cluster}
                  onChange={(e) => (e.target.value === MANAGE ? navigate('/settings') : setCluster(e.target.value))}
                  className="max-w-40 appearance-none bg-transparent pr-5 font-medium leading-5 text-strong focus:outline-none"
                >
                  {(clusters?.length ? clusters : [{ name: 'local' }]).map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.name}
                    </option>
                  ))}
                  {data?.role === 'admin' && <option value={MANAGE}>{t('layout.manageClusters')}</option>}
                </select>
                <Icon name="down" className="pointer-events-none absolute right-0 h-3.5 w-3.5 text-faint" />
              </span>
            </span>
          </label>
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
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="h-[18px] w-[18px]" />
          </button>
          {!data?.devMode && data?.user && (
            <Link
              to="/settings"
              className="flex items-center gap-2.5 rounded-lg p-1 hover:bg-slate-50 dark:hover:bg-slate-800"
              title={t('layout.accountSettings')}
              aria-label={t('layout.accountSettingsFor', { user: data.user })}
            >
              <span className="flex h-8 w-8 items-center justify-center rounded-full border border-blue-100 bg-blue-50 text-xs font-semibold text-blue-700 dark:border-blue-500/20 dark:bg-blue-500/15 dark:text-blue-300">
                {data.user.slice(0, 2).toUpperCase()}
              </span>
              <span className="hidden text-xs leading-4 lg:block">
                <span className="block font-medium text-strong">{data.user}</span>
                <span className="text-[10px] capitalize text-muted">{data.role}</span>
              </span>
            </Link>
          )}
          {!data?.devMode && (
            <button
              onClick={() => {
                void apiPost('/api/logout', {}).finally(() => window.location.reload())
              }}
              aria-label={t('common.logout')}
              title={t('common.logout')}
              className="icon-button hidden sm:inline-flex"
            >
              <Icon name="logout" className="h-[18px] w-[18px]" />
            </button>
          )}
        </div>
      </header>
      <div className="flex min-h-0 flex-1">
        <div className="hidden shrink-0 md:block">
          <Sidebar />
        </div>
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
