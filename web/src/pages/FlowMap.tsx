import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useFlowMap } from '../api/flowmap'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Icon } from '../components/icons'
import { t, locale } from '../i18n'
import {
  CANVAS_WIDTH,
  NODE_R,
  edgeKey,
  edgePath,
  layoutTopology,
  type Point,
} from './flowLayout'
import {
  buildTopology,
  connectedNodes,
  filterTopology,
  type FlowNode,
  type Topology,
} from './flowTopology'

type Offsets = Record<string, Point>

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
        <div className="flow-footer" role="group" aria-label={t('flow.legend')}>
          <span>
            <svg aria-hidden="true" className="flow-legend-shape flow-tone-blue" viewBox="-14 -14 28 28"><circle className="flow-shape" r={10} /></svg>
            {t('flow.source')}
          </span>
          <span>
            <svg aria-hidden="true" className="flow-legend-shape flow-tone-blue" viewBox="-14 -14 28 28"><rect className="flow-shape" x={-10} y={-10} width={20} height={20} rx={5} /></svg>
            {t('flow.gateways')}
          </span>
          <span>
            <svg aria-hidden="true" className="flow-legend-shape flow-tone-violet" viewBox="-14 -14 28 28"><polygon className="flow-shape" points="0,-12 12,0 0,12 -12,0" /></svg>
            {t('flow.routes')}
          </span>
          <span>
            <svg aria-hidden="true" className="flow-legend-shape flow-tone-green" viewBox="-14 -14 28 28"><circle className="flow-shape" r={10} /></svg>
            {t('flow.legend.service')}
          </span>
          <span>
            <svg aria-hidden="true" className="flow-legend-shape flow-tone-slate" viewBox="-14 -14 28 28"><circle className="flow-shape is-dashed" r={10} /></svg>
            {t('flow.legend.external')}
          </span>
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
  const viewportRef = useRef<HTMLDivElement>(null)
  const layout = useMemo(() => layoutTopology(topology), [topology])
  const scale =
    zoom ?? Math.max(0.65, Math.min(1, (viewportWidth - 48) / layout.width))
  const fit = () =>
    setZoom(
      Math.max(
        0.25,
        Math.min(1, (viewportWidth - 48) / layout.width, 612 / layout.height),
      ),
    )
  const selectedNode = topology.nodes.find((n) => n.id === selected)
  const highlight = useMemo(
    () =>
      selectedNode ? connectedNodes([selectedNode.id], topology.edges) : null,
    [selectedNode, topology.edges],
  )
  // Offsets outlive re-layouts (filters, refetch), so clamp them to the current canvas here.
  const placed = (id: string): { at: Point; offset: Point } => {
    const at = layout.positions.get(id) ?? { x: 0, y: 0 }
    const raw = offsets[id] ?? { x: 0, y: 0 }
    const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi)
    return {
      at,
      offset: {
        x: clamp(raw.x, NODE_R - at.x, layout.width - NODE_R - at.x),
        y: clamp(raw.y, NODE_R - at.y, layout.height - NODE_R - at.y),
      },
    }
  }
  const position = (id: string): Point => {
    const { at, offset } = placed(id)
    return { x: at.x + offset.x, y: at.y + offset.y }
  }

  useLayoutEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const observer = new ResizeObserver(() =>
      setViewportWidth(viewport.clientWidth),
    )
    observer.observe(viewport)
    return () => observer.disconnect()
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
          if (!(e.target as Element).closest('.flow-node')) setSelected(null)
        }}
      >
        <svg
          className="flow-svg"
          width={layout.width * scale}
          height={layout.height * scale}
          viewBox={`0 0 ${layout.width} ${layout.height}`}
          style={{ margin: 24 }}
        >
          <defs>
            {(['', 'is-active', 'is-broken'] as const).map((variant) => (
              <marker
                key={variant}
                id={`flow-arrow${variant ? `-${variant}` : ''}`}
                className={`flow-arrow ${variant}`}
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M 0 0 L 10 5 L 0 10 z" />
              </marker>
            ))}
          </defs>
          {layout.bands.map((band) => (
            <g className="flow-ns" key={band.namespace || '*'}>
              <rect
                x={band.x}
                y={band.y}
                width={band.width}
                height={band.height}
                rx={14}
              />
              <text x={band.x + 14} y={band.y + 22}>
                {band.namespace || t('flow.legend.external')}
              </text>
            </g>
          ))}
          {topology.edges.map((edge) => {
            const active =
              !!highlight && highlight.has(edge.from) && highlight.has(edge.to)
            const variant = edge.broken ? 'is-broken' : active ? 'is-active' : ''
            const d = edgePath(position(edge.from), position(edge.to))
            return (
              <g
                key={edgeKey(edge)}
                className={highlight && !active ? 'opacity-20' : ''}
              >
                <path
                  d={d}
                  className={`flow-edge ${variant}`}
                  fill="none"
                  markerEnd={`url(#flow-arrow${variant ? `-${variant}` : ''})`}
                />
                <path
                  d={d}
                  className={`flow-edge-motion ${edge.broken ? 'is-broken' : ''}`}
                  fill="none"
                />
              </g>
            )
          })}
          {topology.nodes.map((node) => (
            <GraphNode
              key={node.id}
              node={node}
              at={placed(node.id).at}
              offset={placed(node.id).offset}
              selected={selectedNode?.id === node.id}
              dimmed={!!highlight && !highlight.has(node.id)}
              scale={scale}
              bounds={layout}
              onSelect={() =>
                setSelected(selected === node.id ? null : node.id)
              }
              onMove={(offset) =>
                setOffsets((previous) => ({ ...previous, [node.id]: offset }))
              }
            />
          ))}
        </svg>
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

const shorten = (text: string, max = 18) =>
  text.length > max ? `${text.slice(0, max - 1)}…` : text

// Kiali-style node: a small shape per kind with the icon inside and the name below.
function GraphNode({
  node,
  at,
  offset,
  selected,
  dimmed,
  scale,
  bounds,
  onSelect,
  onMove,
}: {
  node: FlowNode
  at: Point
  offset: Point
  selected: boolean
  dimmed: boolean
  scale: number
  bounds: { width: number; height: number }
  onSelect: () => void
  onMove: (offset: Point) => void
}) {
  const drag = useRef<{ x: number; y: number; moved: boolean } | null>(null)
  const suppressClick = useRef(false)
  const external = node.icon === 'globe' && node.stage === 3
  const sub =
    node.hosts[0] ??
    [node.port ? `:${node.port}` : '', node.stage === 3 ? node.status : '']
      .filter(Boolean)
      .join(' · ')
  const shape =
    node.stage === 1 ? (
      <rect
        className="flow-shape"
        x={-NODE_R + 2}
        y={-NODE_R + 2}
        width={NODE_R * 2 - 4}
        height={NODE_R * 2 - 4}
        rx={10}
      />
    ) : node.stage === 2 ? (
      <polygon
        className="flow-shape"
        points={`0,${-NODE_R - 2} ${NODE_R + 2},0 0,${NODE_R + 2} ${-NODE_R - 2},0`}
      />
    ) : (
      <circle className={`flow-shape ${external ? 'is-dashed' : ''}`} r={NODE_R} />
    )
  return (
    <g
      className={`flow-node flow-tone-${node.tone} ${selected ? 'is-selected' : ''} ${dimmed ? 'is-dimmed' : ''}`}
      style={{ transform: `translate(${offset.x}px, ${offset.y}px)` }}
      role="button"
      tabIndex={0}
      aria-label={`${node.kind} ${node.name}${node.status ? `, ${node.status}` : ''}`}
      aria-pressed={selected}
      onClick={(e) => {
        e.stopPropagation()
        if (!suppressClick.current) onSelect()
        suppressClick.current = false
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onSelect()
        }
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
          const x = Math.min(
            Math.max(offset.x + dx, NODE_R - at.x),
            bounds.width - NODE_R - at.x,
          )
          const y = Math.min(
            Math.max(offset.y + dy, NODE_R - at.y),
            bounds.height - NODE_R - at.y,
          )
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
      <g transform={`translate(${at.x} ${at.y})`}>
        <title>
          {`${node.kind} · ${node.name}${node.status ? ` · ${node.status}` : ''}`}
        </title>
        {/* transparent hit area so the label and the gaps around the shape are clickable/draggable */}
        <rect className="flow-hit" x={-36} y={-30} width={72} height={84} />
        {selected && <circle className="flow-ring" r={NODE_R + 7} />}
        {shape}
        <g transform="translate(-8 -8)">
          <Icon name={node.icon} className="flow-node-icon" size={16} />
        </g>
        {node.issue && (
          <g transform={`translate(${NODE_R - 6} ${-NODE_R + 6})`}>
            <circle className="flow-badge" r={8} />
            <text y={3.5} textAnchor="middle">
              !
            </text>
          </g>
        )}
        <text className="flow-label" y={NODE_R + 17} textAnchor="middle">
          {shorten(node.name)}
        </text>
        {sub && (
          <text className="flow-sublabel" y={NODE_R + 31} textAnchor="middle">
            {shorten(sub, 24)}
          </text>
        )}
      </g>
    </g>
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
