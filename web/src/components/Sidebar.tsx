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

const LINKS = [
  { to: '/', icon: 'home', label: '클러스터 개요' },
  { to: '/overview', icon: 'cube', label: '전체 리소스' },
  { to: '/flowmap', icon: 'route', label: '트래픽 흐름' },
  { to: '/routecheck', icon: 'terminal', label: '요청 콘솔' },
]

export function Sidebar({ onNavigate, onClose }: { onNavigate?: () => void; onClose?: () => void }) {
  const { data, isLoading } = useResourceTypes()
  const caps = useCapabilities().data
  const clusters = useClusters().data
  const [search, setSearch] = useState('')
  const installed = (data ?? []).filter((t) => t.installed)
  const filtered = installed.filter((t) => t.kind.toLowerCase().includes(search.toLowerCase()))
  const topLink = ({ isActive }: { isActive: boolean }) => `nav-link ${isActive ? 'active' : ''}`

  return (
    <nav className="app-sidebar" aria-label="주요 탐색">
      {onClose && (
        <div className="flex items-center justify-between border-b border-base px-4 py-3">
          <span className="text-sm font-semibold text-strong">Sealane</span>
          <button onClick={onClose} className="icon-button" aria-label="탐색 메뉴 닫기">
            <Icon name="close" />
          </button>
        </div>
      )}
      <div className="px-4 pb-3 pt-5">
        <div className="workspace-card">
          <div className="mb-2 text-[9px] font-semibold uppercase tracking-[.14em] text-muted">
            Current workspace
          </div>
          <div className="flex items-center gap-2.5">
            <span className="flex h-7 w-7 items-center justify-center rounded-md border border-base bg-white text-muted dark:bg-slate-800">
              <Icon name="network" className="h-4 w-4" />
            </span>
            <span className="min-w-0 flex-1 truncate text-xs font-semibold text-strong">
              {getCluster() || 'local'}
            </span>
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" title="API 연결됨" />
          </div>
          {onClose && clusters && clusters.length > 1 && (
            <select
              aria-label="클러스터 선택"
              className="input-base mt-3 text-xs"
              value={getCluster() || 'local'}
              onChange={(e) => setCluster(e.target.value)}
            >
              {clusters.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
      <div className="space-y-1 px-3 pb-4">
        {LINKS.map((l) => (
          <NavLink key={l.to} to={l.to} end={l.to === '/'} className={topLink} onClick={onNavigate}>
            <Icon name={l.icon} className="h-[18px] w-[18px]" />
            {l.label}
          </NavLink>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto border-t border-base px-3 pb-4 pt-4">
        <div className="mb-3 flex items-center justify-between px-3">
          <span className="text-[10px] font-semibold uppercase tracking-[.12em] text-muted">Resources</span>
          <span className="text-[10px] tabular-nums text-muted">{installed.length} kinds</span>
        </div>
        <label className="relative mb-4 block">
          <Icon
            name="search"
            className="pointer-events-none absolute left-3 top-2.5 h-3.5 w-3.5 text-faint"
          />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="리소스 종류 찾기"
            aria-label="리소스 종류 찾기"
            className="input-base !min-h-8 !py-1.5 pl-8 text-xs"
          />
        </label>
        {isLoading && (
          <div className="space-y-2 px-3" aria-label="리소스 로딩 중">
            {[1, 2, 3].map((n) => (
              <div key={n} className="h-5 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
            ))}
          </div>
        )}
        {search && filtered.length === 0 && (
          <p className="px-3 py-3 text-xs text-muted">일치하는 종류가 없습니다.</p>
        )}
        {CATEGORY_ORDER.map((cat) => {
          const items = filtered.filter((t) => t.category === cat)
          if (!items.length) return null
          const accent = accentFor(cat)
          return (
            <div key={cat} className="mb-4">
              <div className="mb-1.5 flex items-center gap-2 px-3 text-[10px] font-semibold tracking-wide text-muted">
                <Icon name={CATEGORY_ICON[cat]} className={`h-3.5 w-3.5 ${accent.text}`} />
                {CATEGORY_LABELS[cat]}
              </div>
              {items.map((t) => (
                <KindLink key={t.typeId} t={t} onNavigate={onNavigate} />
              ))}
            </div>
          )
        })}
      </div>
      <div className="border-t border-base p-3">
        <NavLink to="/settings" className={topLink} onClick={onNavigate}>
          <Icon name="settings" className="h-[18px] w-[18px]" />
          설정<span className="ml-auto text-[10px] capitalize text-faint">{caps?.role}</span>
        </NavLink>
        {onClose && !caps?.devMode && (
          <button
            className="nav-link w-full"
            onClick={() => {
              void apiPost('/api/logout', {}).finally(() => window.location.reload())
            }}
          >
            <Icon name="logout" className="h-[18px] w-[18px]" />
            로그아웃
          </button>
        )}
      </div>
    </nav>
  )
}

function KindLink({ t, onNavigate }: { t: ResourceType; onNavigate?: () => void }) {
  const { ns } = useNamespace()
  const { data } = useResources(t.typeId, ns)
  return (
    <NavLink
      to={`/resources/${t.typeId}`}
      onClick={onNavigate}
      className={({ isActive }) => `resource-link ${isActive ? 'active' : ''}`}
    >
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="truncate">{t.kind}</span>
        {t.dangerous && <Icon name="warning" className="h-3 w-3 shrink-0 text-amber-500" />}
      </span>
      {data && <span className="nav-count">{data.length}</span>}
    </NavLink>
  )
}
