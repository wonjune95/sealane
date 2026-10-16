import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useFlowMap } from '../api/flowmap'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Icon } from '../components/icons'
import { t, locale, type Key } from '../i18n'
import {
  buildTopology,
  connectedNodes,
  filterTopology,
  type FlowNode,
  type Topology,
} from './flowTopology'

type Offsets = Record<string, { x: number; y: number }>
const CANVAS_WIDTH = 1064
const stages: Key[] = ['flow.source', 'flow.gateways', 'flow.routes', 'flow.destinations']

export function FlowMap() {
  const { data, isLoading, error, isFetching, refetch, dataUpdatedAt } =
    useFlowMap()
  const [direction, setDirection] = useState<'ingress' | 'egress'>('ingress')
  const [query, setQuery] = useState('')
  const [namespace, setNamespace] = useState('')
  const [issuesOnly, setIssuesOnly] = useState(false)
  const ingress = useMemo(() => buildTopology(data, 'ingress'), [data])
  const egress = useMemo(() => buildTopology(data, 'egress'), [data])
  const topology = direction === 'ingress' ? ingress : egress
  const filtered = useMemo(
    () => filterTopology(topology, query, namespace, issuesOnly),
    [topology, query, namespace, issuesOnly],
  )
  const namespaces = [
    ...new Set(
      [...ingress.nodes, ...egress.nodes].flatMap((n) =>
        n.namespace ? [n.namespace] : [],
      ),
    ),
  ].sort()
  const issues = topology.nodes.filter((n) => n.issue).length
  const resetFilters = () => {
    setQuery('')
    setNamespace('')
    setIssuesOnly(false)
  }
  const filteredCount = filtered.nodes.filter((n) => n.stage === 2).length

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow={t('eyebrow.flowmap')}
        title={t('flow.title')}
        description={t('flow.description')}
        actions={
          <button
            className="btn-ghost"
            onClick={() => void refetch()}
            disabled={isFetching}
          >
            <Icon
              name="refresh"
              className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`}
            />{' '}
            {t('common.refresh')}
          </button>
        }
      />
      <div className="flow-summary">
        {[
          {
            label: t('flow.gateways'),
            value: topology.nodes.filter((n) => n.stage === 1).length,
            icon: 'shield',
            tone: 'blue',
          },
          {
            label: t('flow.routes'),
            value: topology.nodes.filter((n) => n.stage === 2).length,
            icon: 'route',
            tone: 'violet',
          },
          {
            label: t('flow.destinations'),
            value: topology.nodes.filter((n) => n.stage === 3).length,
            icon: 'cube',
            tone: 'green',
          },
          {
            label: t('flow.issueNodes'),
            value: issues,
            icon: issues ? 'warning' : 'check',
            tone: issues ? 'red' : 'green',
          },
        ].map((stat) => (
          <div className="panel flex items-center gap-3 p-4" key={stat.label}>
            <span className={`flow-icon flow-tone-${stat.tone}`}>
              <Icon name={stat.icon} className="h-5 w-5" />
            </span>
            <div>
              <p className="text-xs text-muted">{stat.label}</p>
              <p className="mt-1 text-xl font-semibold tabular-nums text-strong">
                {isLoading ? '—' : stat.value}
              </p>
            </div>
          </div>
        ))}
      </div>
      <section
        className="panel min-w-0 overflow-hidden"
        aria-label={t('flow.explore')}
      >
        <div className="flow-toolbar">
          <div className="flow-tabs" aria-label={t('flow.direction')}>
            {(['ingress', 'egress'] as const).map((value) => (
              <button
                key={value}
                className={direction === value ? 'is-active' : ''}
                aria-pressed={direction === value}
                onClick={() => setDirection(value)}
              >
                <Icon
                  name={value === 'ingress' ? 'download' : 'send'}
                  className="h-4 w-4"
                />
                {value === 'ingress' ? t('flow.ingress') : t('flow.egress')}
                <span>
                  {
                    (value === 'ingress' ? ingress : egress).nodes.filter(
                      (n) => n.stage === 2 || n.kind === 'ServiceEntry',
                    ).length
                  }
                </span>
              </button>
            ))}
          </div>
          <p className="text-xs text-muted">
            {t('flow.refreshNote')}
            {dataUpdatedAt > 0 &&
              ` · ${new Date(dataUpdatedAt).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })}`}
          </p>
        </div>
        <div className="flow-filters">
          <div className="relative min-w-0 flex-1">
            <Icon
              name="search"
              className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted"
            />
            <input
              className="input-base pl-9"
              aria-label={t('flow.search')}
              placeholder={t('flow.searchPlaceholder')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <select
            className="input-base flow-namespace"
            aria-label={t('flow.namespace')}
            value={namespace}
            onChange={(e) => setNamespace(e.target.value)}
          >
            <option value="">{t('flow.allNamespaces')}</option>
            {namespaces.map((ns) => (
              <option key={ns}>{ns}</option>
            ))}
          </select>
          <button
            className={`flow-issue-filter ${issuesOnly ? 'is-active' : ''}`}
            aria-pressed={issuesOnly}
            onClick={() => setIssuesOnly(!issuesOnly)}
          >
            <Icon name="warning" className="h-4 w-4" /> {t('flow.problemPaths')}{' '}
            <span>{issues}</span>
          </button>
          {(query || namespace || issuesOnly) && (
            <button className="btn-ghost" onClick={resetFilters}>
              {t('flow.resetFilters')}
            </button>
          )}
        </div>
        {error && (
          <div
            className="m-5 flex flex-wrap items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-600 dark:text-red-400"
            role="alert"
          >
            <Icon name="warning" />
            {t('flow.loadFailed', { message: error.message })}
            <button className="btn-ghost" onClick={() => void refetch()}>
              {t('common.retry')}
            </button>
          </div>
        )}
        {isLoading ? (
          <div className="p-12 text-center text-sm text-muted" role="status">
            {t('flow.loading')}
          </div>
        ) : !data ? null : filtered.nodes.length <= 1 ? (
          <EmptyState
            icon="route"
            title={
              topology.nodes.length <= 1
                ? t(direction === 'ingress' ? 'flow.noIngress' : 'flow.noEgress')
                : t('flow.noMatch')
            }
            description={
              topology.nodes.length <= 1
                ? t('flow.emptyHint')
                : t('flow.noMatchHint')
            }
            action={
              (query || namespace || issuesOnly) && (
                <button className="btn-ghost" onClick={resetFilters}>
                  {t('flow.resetFilters')}
                </button>
              )
            }
          />
        ) : (
          <FlowCanvas key={direction} topology={filtered} />
        )}
        <div className="flow-footer">
          <span>
            <span className="flow-legend-line" /> {t('flow.legendConfigured')}
          </span>
          <span>
            <span className="flow-legend-line is-broken" /> {t('flow.legendBroken')}
          </span>
          <span className="ml-auto">
            {t('flow.footer', { n: filteredCount })}
          </span>
        </div>
      </section>
    </div>
  )
}

function FlowCanvas({ topology }: { topology: Topology }) {
  const [selected, setSelected] = useState<string | null>(null)
  const [offsets, setOffsets] = useState<Offsets>({})
  const [zoom, setZoom] = useState<number | null>(null)
  const [viewportWidth, setViewportWidth] = useState(CANVAS_WIDTH + 48)
  const [height, setHeight] = useState(340)
  const viewportRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLDivElement>(null)
  const refs = useRef(new Map<string, HTMLButtonElement>())
  const [paths, setPaths] = useState<
    { key: string; from: string; to: string; d: string; broken?: boolean }[]
  >([])
  const scale =
    zoom ?? Math.max(0.65, Math.min(1, (viewportWidth - 48) / CANVAS_WIDTH))
  const fit = () =>
    setZoom(Math.max(0.25, Math.min(1, (viewportWidth - 48) / CANVAS_WIDTH)))
  const selectedNode = topology.nodes.find((n) => n.id === selected)
  const highlight = useMemo(
    () =>
      selectedNode ? connectedNodes([selectedNode.id], topology.edges) : null,
    [selectedNode, topology.edges],
  )

  useLayoutEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const observer = new ResizeObserver(() =>
      setViewportWidth(viewport.clientWidth),
    )
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [])

  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const draw = () => {
      const bounds = canvas.getBoundingClientRect()
      let bottom = 320
      for (const node of refs.current.values())
        bottom = Math.max(
          bottom,
          (node.getBoundingClientRect().bottom - bounds.top) / scale + 32,
        )
      setHeight(bottom)
      setPaths(
        topology.edges.flatMap((edge) => {
          const a = refs.current.get(edge.from)?.getBoundingClientRect()
          const b = refs.current.get(edge.to)?.getBoundingClientRect()
          if (!a || !b) return []
          const x1 = (a.right - bounds.left) / scale,
            y1 = (a.top + a.height / 2 - bounds.top) / scale
          const x2 = (b.left - bounds.left) / scale,
            y2 = (b.top + b.height / 2 - bounds.top) / scale
          const bend = Math.max(28, (x2 - x1) / 2)
          return [
            {
              ...edge,
              key: `${edge.from}>${edge.to}`,
              d: `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`,
            },
          ]
        }),
      )
    }
    draw()
    const observer = new ResizeObserver(draw)
    for (const node of refs.current.values()) observer.observe(node)
    window.addEventListener('resize', draw)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', draw)
    }
  }, [topology, offsets, scale])

  const register = useCallback((id: string, el: HTMLButtonElement | null) => {
    if (el) refs.current.set(id, el)
    else refs.current.delete(id)
  }, [])

  return (
    <>
      <div className="flow-canvas-controls">
        <p className="text-xs text-muted">
          {t('flow.canvasHint')}
        </p>
        <div
          className="flex flex-wrap items-center gap-1"
          aria-label={t('flow.viewControls')}
        >
          <button
            className="flow-control"
            aria-label={t('flow.zoomOut')}
            disabled={scale <= 0.25}
            onClick={() => setZoom(Math.max(0.25, scale - 0.1))}
          >
            <Icon name="minus" className="h-4 w-4" />
          </button>
          <span
            className="w-12 text-center text-xs tabular-nums text-muted"
            aria-live="polite"
          >
            {Math.round(scale * 100)}%
          </span>
          <button
            className="flow-control"
            aria-label={t('flow.zoomIn')}
            disabled={scale >= 1.5}
            onClick={() => setZoom(Math.min(1.5, scale + 0.1))}
          >
            <Icon name="plus" className="h-4 w-4" />
          </button>
          <button className="flow-control px-2" onClick={fit}>
            <Icon name="fit" className="h-4 w-4" /> {t('flow.fit')}
          </button>
          <button
            className="flow-control px-2"
            onClick={() => {
              setOffsets({})
              setZoom(null)
              setSelected(null)
              viewportRef.current?.scrollTo({ left: 0, top: 0 })
            }}
          >
            <Icon name="refresh" className="h-4 w-4" /> {t('flow.resetLayout')}
          </button>
        </div>
      </div>
      <div
        ref={viewportRef}
        className="flow-viewport"
        tabIndex={0}
        aria-label={t('flow.graph')}
        onKeyDown={(e) => {
          if (e.key === 'Escape') setSelected(null)
        }}
        onClick={(e) => {
          if (!(e.target as HTMLElement).closest('button')) setSelected(null)
        }}
      >
        <div
          style={{
            width: CANVAS_WIDTH * scale,
            height: height * scale,
            margin: 24,
          }}
        >
          <div
            ref={canvasRef}
            className="flow-canvas"
            style={{
              width: CANVAS_WIDTH,
              minHeight: height,
              transform: `scale(${scale})`,
              transformOrigin: 'top left',
            }}
          >
            <svg
              className="pointer-events-none absolute inset-0 h-full w-full"
              aria-hidden="true"
            >
              {paths.map((path) => {
                const active =
                  !!highlight &&
                  highlight.has(path.from) &&
                  highlight.has(path.to)
                return (
                  <g
                    key={path.key}
                    className={highlight && !active ? 'opacity-20' : ''}
                  >
                    <path
                      d={path.d}
                      className={`flow-edge ${path.broken ? 'is-broken' : active ? 'is-active' : ''}`}
                      fill="none"
                    />
                    <path
                      d={path.d}
                      className={`flow-edge-motion ${path.broken ? 'is-broken' : ''}`}
                      fill="none"
                    />
                  </g>
                )
              })}
            </svg>
            <div className="flow-columns">
              {stages.map((stage, index) => (
                <div key={stage} className="min-w-0">
                  <div className="flow-stage">
                    <span>{String(index + 1).padStart(2, '0')}</span>
                    {t(stage)}
                    <small>
                      {topology.nodes.filter((n) => n.stage === index).length}
                    </small>
                  </div>
                  <div className="flex flex-col gap-5">
                    {topology.nodes
                      .filter((n) => n.stage === index)
                      .map((node) => (
                        <DraggableCard
                          key={node.id}
                          node={node}
                          selected={selectedNode?.id === node.id}
                          dimmed={!!highlight && !highlight.has(node.id)}
                          offset={offsets[node.id] ?? { x: 0, y: 0 }}
                          register={register}
                          scale={scale}
                          onSelect={() =>
                            setSelected(selected === node.id ? null : node.id)
                          }
                          onMove={(offset) =>
                            setOffsets((previous) => ({
                              ...previous,
                              [node.id]: offset,
                            }))
                          }
                        />
                      ))}
                    {!topology.nodes.some((n) => n.stage === index) && (
                      <div className="flow-stage-empty">
                        {index === 1 || index === 2
                          ? t('flow.directPath')
                          : t('flow.noNodes')}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
      {selectedNode ? (
        <NodeDetails
          node={selectedNode}
          connected={highlight?.size ?? 1}
          clear={() => setSelected(null)}
        />
      ) : (
        <div className="flow-selection-hint">
          <Icon name="info" className="h-4 w-4" /> {t('flow.selectHint')}
        </div>
      )}
    </>
  )
}

function DraggableCard({
  node,
  selected,
  dimmed,
  offset,
  register,
  scale,
  onSelect,
  onMove,
}: {
  node: FlowNode
  selected: boolean
  dimmed: boolean
  offset: { x: number; y: number }
  register: (id: string, el: HTMLButtonElement | null) => void
  scale: number
  onSelect: () => void
  onMove: (offset: { x: number; y: number }) => void
}) {
  const drag = useRef<{ x: number; y: number; moved: boolean } | null>(null)
  const suppressClick = useRef(false)
  return (
    <button
      ref={(el) => register(node.id, el)}
      type="button"
      aria-label={`${node.kind} ${node.name}${node.status ? `, ${node.status}` : ''}`}
      aria-pressed={selected}
      className={`flow-node flow-tone-${node.tone} ${node.stage === 0 ? 'flow-source' : ''} ${selected ? 'is-selected' : ''} ${dimmed ? 'is-dimmed' : ''}`}
      style={{ transform: `translate(${offset.x}px, ${offset.y}px)` }}
      onClick={(e) => {
        e.stopPropagation()
        if (!suppressClick.current) onSelect()
        suppressClick.current = false
      }}
      onPointerDown={(e) => {
        if (e.pointerType !== 'mouse' || e.button !== 0) return
        suppressClick.current = false
        drag.current = { x: e.clientX, y: e.clientY, moved: false }
        e.currentTarget.setPointerCapture(e.pointerId)
      }}
      onPointerMove={(e) => {
        if (!drag.current) return
        const dx = (e.clientX - drag.current.x) / scale,
          dy = (e.clientY - drag.current.y) / scale
        if (Math.abs(dx) + Math.abs(dy) > 4) drag.current.moved = true
        if (drag.current.moved) {
          const columnStart = [0, 176, 472, 768][node.stage]
          const x = Math.min(
            Math.max(offset.x + dx, -columnStart),
            CANVAS_WIDTH - columnStart - (node.stage === 0 ? 120 : 240),
          )
          const y = Math.max(-12, offset.y + dy)
          onMove({ x, y })
          drag.current.x = e.clientX
          drag.current.y = e.clientY
        }
      }}
      onPointerUp={() => {
        suppressClick.current = !!drag.current?.moved
        drag.current = null
      }}
      onPointerCancel={() => {
        drag.current = null
        suppressClick.current = false
      }}
    >
      <div className="flex items-center gap-2.5">
        <span className="flow-icon">
          <Icon name={node.icon} className="h-4 w-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[10px] font-semibold uppercase tracking-wider text-muted">
            {node.kind}
          </span>
          <span
            className="flow-node-name mt-0.5 block truncate text-sm font-semibold text-strong"
            title={node.name}
          >
            {node.name}
          </span>
        </span>
      </div>
      {node.namespace && (
        <p className="mt-3 truncate text-[11px] text-muted">
          {node.namespace}
          {node.port ? ` · :${node.port}` : ''}
        </p>
      )}
      {node.hosts.length > 0 && (
        <p className="flow-hosts" title={node.hosts.join(', ')}>
          {node.hosts.join(', ')}
        </p>
      )}
      {node.status && (
        <span className={`flow-status ${node.issue ? 'is-issue' : ''}`}>
          <span />
          {node.status}
        </span>
      )}
      {selected && <span className="flow-selected-dot" />}
    </button>
  )
}

function NodeDetails({
  node,
  connected,
  clear,
}: {
  node: FlowNode
  connected: number
  clear: () => void
}) {
  return (
    <div
      className="flow-details"
      aria-label={t('flow.selectedDetails')}
      aria-live="polite"
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className={`flow-icon flow-tone-${node.tone}`}>
          <Icon name={node.icon} />
        </span>
        <div className="min-w-0">
          <p className="text-[11px] font-medium text-muted">
            {node.kind} · {t('flow.connectedNodes', { n: connected })}
          </p>
          <h2 className="mt-1 break-all text-sm font-semibold text-strong">
            {node.name}
          </h2>
        </div>
        <button
          className="flow-control ml-auto"
          aria-label={t('flow.clearSelection')}
          onClick={clear}
        >
          <Icon name="close" className="h-4 w-4" />
        </button>
      </div>
      <dl className="flow-detail-grid">
        <div>
          <dt>{t('common.namespace')}</dt>
          <dd>{node.namespace || '—'}</dd>
        </div>
        <div>
          <dt>{t('flow.hostPort')}</dt>
          <dd>
            {node.hosts.join(', ') || (node.port ? `:${node.port}` : '—')}
          </dd>
        </div>
        <div>
          <dt>{t('flow.statusLabel')}</dt>
          <dd className={node.issue ? 'text-red-600 dark:text-red-400' : ''}>
            {node.issue || node.status || t('flow.inRouting')}
          </dd>
        </div>
      </dl>
      {node.to && (
        <Link className="btn-ghost justify-center" to={node.to}>
          {t('flow.openResource')}
          <Icon name="arrow" className="h-4 w-4" />
        </Link>
      )}
    </div>
  )
}
