import { useState } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { useResourceTypes, CATEGORY_LABELS, type ResourceType } from '../api/resourceTypes'
import { useResources } from '../api/resources'
import { useCapabilities } from '../api/capabilities'
import { useAnalysis } from '../api/analyze'
import { apiPost, getCluster } from '../api/client'
import { useClusters } from '../api/clusters'
import { CATEGORY_ORDER } from '../ui/categories'
import { useNamespace } from '../ui/namespace'
import { ClusterMenu } from './ClusterMenu'
import { Icon } from './icons'
import { t, type Key } from '../i18n'

const LINKS: { to: string; icon: string; label: Key }[] = [
  { to: '/', icon: 'dashboard', label: 'nav.home' },
  { to: '/overview', icon: 'layers', label: 'nav.overview' },
  { to: '/flowmap', icon: 'route', label: 'nav.flowmap' },
  { to: '/analysis', icon: 'activity', label: 'nav.analysis' },
  { to: '/services', icon: 'cube', label: 'nav.services' },
  { to: '/routecheck', icon: 'terminal', label: 'nav.routecheck' },
]

export function Sidebar({ onNavigate, onClose }: { onNavigate?: () => void; onClose?: () => void }) {
  const { data, isLoading } = useResourceTypes()
  const caps = useCapabilities()
  const clusters = useClusters().data
  const errors = useAnalysis('', false).data?.findings?.filter((f) => f.severity === 'error').length ?? 0
  const [search, setSearch] = useState('')
  const installed = (data ?? []).filter((rt) => rt.installed)
  const filtered = installed.filter((rt) => rt.kind.toLowerCase().includes(search.toLowerCase()))
  const topLink = ({ isActive }: { isActive: boolean }) => `nav-link ${isActive ? 'active' : ''}`
  const user = caps.data?.user

  return (
    <nav className="app-sidebar" aria-label={t('sidebar.primaryNav')}>
      <div className="flex items-center justify-between border-b border-base pr-2">
        <Link to="/" className="app-brand" aria-label={t('layout.brandHome')} onClick={onNavigate}>
          <img src="/favicon.svg" alt="" className="h-7 w-7 shrink-0" />
          <span className="text-[16px] font-bold text-white">Sealane</span>
        </Link>
        {onClose && (
          <button onClick={onClose} className="icon-button text-white" aria-label={t('sidebar.closeMenu')}>
            <Icon name="close" />
          </button>
        )}
      </div>
      <div className="px-3 pt-3">
        <ClusterMenu
          clusters={(clusters?.length ? clusters : [{ name: 'local' }]).map((c) => c.name)}
          current={getCluster() || 'local'}
          connected={!!caps.data && !caps.isError}
          canManage={caps.data?.role === 'admin'}
        />
      </div>
      <div className="space-y-px px-3 py-3">
        {LINKS.map((l) => (
          <NavLink key={l.to} to={l.to} end={l.to === '/'} className={topLink} onClick={onNavigate}>
            <Icon name={l.icon} className="h-4 w-4" />
            {t(l.label)}
            {l.to === '/analysis' && errors > 0 && (
              <span
                className="ml-auto min-w-[18px] rounded-full bg-red-600 px-1.5 text-center text-[11px] font-semibold leading-[18px] text-white"
                title={t('sidebar.analysisErrors', { n: errors })}
              >
                {errors}
              </span>
            )}
          </NavLink>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto border-t border-base px-3 pb-4 pt-3">
        <div className="mb-2 flex items-center justify-between px-2.5">
          <span className="text-[10px] font-semibold uppercase tracking-[.12em] text-muted">{t('sidebar.resources')}</span>
          <span className="text-[10px] tabular-nums text-muted">{t('sidebar.kindCount', { n: installed.length })}</span>
        </div>
        <label className="relative mb-3 block">
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-2 h-3.5 w-3.5 text-faint" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('sidebar.searchKinds')}
            aria-label={t('sidebar.searchKinds')}
            className="input-base !min-h-7 !py-1 pl-8 text-xs"
          />
        </label>
        {isLoading && (
          <div className="space-y-2 px-2.5" aria-label={t('sidebar.loading')}>
            {[1, 2, 3].map((n) => (
              <div key={n} className="h-4 animate-pulse rounded bg-white/10" />
            ))}
          </div>
        )}
        {search && filtered.length === 0 && <p className="px-2.5 py-3 text-xs text-muted">{t('sidebar.noKinds')}</p>}
        {CATEGORY_ORDER.map((cat) => {
          const items = filtered.filter((rt) => rt.category === cat)
          if (!items.length) return null
          return (
            <div key={cat} className="mb-3">
              <div className="mb-1 px-2.5 text-[10px] font-semibold uppercase tracking-wide text-muted">
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
          <Icon name="settings" className="h-4 w-4" />
          {t('nav.settings')}
        </NavLink>
        {!caps.data?.devMode && user && (
          <div className="mt-1 flex items-center gap-1 pl-1">
            <Link
              to="/settings"
              onClick={onNavigate}
              title={t('layout.accountSettings')}
              aria-label={t('layout.accountSettingsFor', { user })}
              className="flex min-w-0 flex-1 items-center gap-2.5 rounded px-1.5 py-1.5 hover:bg-white/10"
            >
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-white/15 text-[11px] font-semibold text-white">
                {user.slice(0, 2).toUpperCase()}
              </span>
              <span className="min-w-0 flex-1 leading-4">
                <span className="block truncate text-[13px] text-white">{user}</span>
                <span className="text-[11px] capitalize text-muted">{caps.data?.role}</span>
              </span>
            </Link>
            <button
              onClick={() => {
                void apiPost('/api/logout', {}).finally(() => window.location.reload())
              }}
              aria-label={t('common.logout')}
              title={t('common.logout')}
              className="icon-button h-7 w-7 text-muted hover:bg-white/10 hover:text-white"
            >
              <Icon name="logout" className="h-4 w-4" />
            </button>
          </div>
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
        {type.dangerous && <Icon name="warning" className="h-3 w-3 shrink-0 text-amber-400" />}
      </span>
      {data && <span className="nav-count">{data.length}</span>}
    </NavLink>
  )
}
