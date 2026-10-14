import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import { useResourceTypes, CATEGORY_LABELS, type ResourceType } from '../api/resourceTypes'
import { useResources } from '../api/resources'
import { useCapabilities } from '../api/capabilities'
import { apiPost, getCluster, setCluster } from '../api/client'
import { useClusters } from '../api/clusters'
import { CATEGORY_ORDER, CATEGORY_ICON, accentFor } from '../ui/categories'
import { useNamespace } from '../ui/namespace'
import { Icon } from './icons'
import { t, type Key } from '../i18n'

const LINKS: { to: string; icon: string; label: Key }[] = [
  { to: '/', icon: 'home', label: 'nav.home' },
  { to: '/overview', icon: 'cube', label: 'nav.overview' },
  { to: '/flowmap', icon: 'route', label: 'nav.flowmap' },
  { to: '/analysis', icon: 'shield', label: 'nav.analysis' },
  { to: '/services', icon: 'network', label: 'nav.services' },
  { to: '/routecheck', icon: 'terminal', label: 'nav.routecheck' },
]

export function Sidebar({ onNavigate, onClose }: { onNavigate?: () => void; onClose?: () => void }) {
  const { data, isLoading } = useResourceTypes()
  const caps = useCapabilities().data
  const clusters = useClusters().data
  const [search, setSearch] = useState('')
  const installed = (data ?? []).filter((rt) => rt.installed)
  const filtered = installed.filter((rt) => rt.kind.toLowerCase().includes(search.toLowerCase()))
  const topLink = ({ isActive }: { isActive: boolean }) => `nav-link ${isActive ? 'active' : ''}`

  return (
    <nav className="app-sidebar" aria-label={t('sidebar.primaryNav')}>
      {onClose && (
        <div className="flex items-center justify-between border-b border-base px-4 py-3">
          <span className="text-sm font-semibold text-strong">Sealane</span>
          <button onClick={onClose} className="icon-button" aria-label={t('sidebar.closeMenu')}>
            <Icon name="close" />
          </button>
        </div>
      )}
      {onClose && clusters && clusters.length > 1 && (
        <div className="border-b border-base px-4 py-2">
          <select
            aria-label={t('layout.selectCluster')}
            className="input-base text-xs"
            value={getCluster() || 'local'}
            onChange={(e) => setCluster(e.target.value)}
          >
            {clusters.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
        </div>
      )}
      <div className="space-y-0.5 px-3 pb-3 pt-3">
        {LINKS.map((l) => (
          <NavLink key={l.to} to={l.to} end={l.to === '/'} className={topLink} onClick={onNavigate}>
            <Icon name={l.icon} className="h-[18px] w-[18px]" />
            {t(l.label)}
          </NavLink>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto border-t border-base px-3 pb-4 pt-4">
        <div className="mb-3 flex items-center justify-between px-3">
          <span className="text-[10px] font-semibold uppercase tracking-[.12em] text-muted">{t('sidebar.resources')}</span>
          <span className="text-[10px] tabular-nums text-muted">{t('sidebar.kindCount', { n: installed.length })}</span>
        </div>
        <label className="relative mb-4 block">
          <Icon
            name="search"
            className="pointer-events-none absolute left-3 top-2.5 h-3.5 w-3.5 text-faint"
          />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('sidebar.searchKinds')}
            aria-label={t('sidebar.searchKinds')}
            className="input-base !min-h-8 !py-1.5 pl-8 text-xs"
          />
        </label>
        {isLoading && (
          <div className="space-y-2 px-3" aria-label={t('sidebar.loading')}>
            {[1, 2, 3].map((n) => (
              <div key={n} className="h-5 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
            ))}
          </div>
        )}
        {search && filtered.length === 0 && (
          <p className="px-3 py-3 text-xs text-muted">{t('sidebar.noKinds')}</p>
        )}
        {CATEGORY_ORDER.map((cat) => {
          const items = filtered.filter((rt) => rt.category === cat)
          if (!items.length) return null
          const accent = accentFor(cat)
          return (
            <div key={cat} className="mb-4">
              <div className="mb-1.5 flex items-center gap-2 px-3 text-[10px] font-semibold tracking-wide text-muted">
                <Icon name={CATEGORY_ICON[cat]} className={`h-3.5 w-3.5 ${accent.text}`} />
                {CATEGORY_LABELS[cat]}
              </div>
              {items.map((rt) => (
                <KindLink key={rt.typeId} type={rt} onNavigate={onNavigate} />
              ))}
            </div>
          )
        })}
      </div>
      <div className="border-t border-base p-3">
        <NavLink to="/settings" className={topLink} onClick={onNavigate}>
          <Icon name="settings" className="h-[18px] w-[18px]" />
          {t('nav.settings')}<span className="ml-auto text-[10px] capitalize text-faint">{caps?.role}</span>
        </NavLink>
        {onClose && !caps?.devMode && (
          <button
            className="nav-link w-full"
            onClick={() => {
              void apiPost('/api/logout', {}).finally(() => window.location.reload())
            }}
          >
            <Icon name="logout" className="h-[18px] w-[18px]" />
            {t('common.logout')}
          </button>
        )}
      </div>
    </nav>
  )
}

function KindLink({ type, onNavigate }: { type: ResourceType; onNavigate?: () => void }) {
  const { ns } = useNamespace()
  const { data } = useResources(type.typeId, ns)
  return (
    <NavLink
      to={`/resources/${type.typeId}`}
      onClick={onNavigate}
      className={({ isActive }) => `resource-link ${isActive ? 'active' : ''}`}
    >
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="truncate">{type.kind}</span>
        {type.dangerous && <Icon name="warning" className="h-3 w-3 shrink-0 text-amber-500" />}
      </span>
      {data && <span className="nav-count">{data.length}</span>}
    </NavLink>
  )
}
