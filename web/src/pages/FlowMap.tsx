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
  ARROW,
  BAND_BOTTOM,
  BAND_PAD,
  BOX_PAD_X,
  EXTERNAL,
  boxAround,
  edgeKey,
  edgePath,
  type Band,
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
          <span className="ml-auto">{t('flow.footer', { n: filteredCount })}</span>
        </div>
      </section>
    </div>
  )
}

function FlowCanvas({ topology }: { topology: Topology }) {
  const [selected, setSelected] = useState<string | null>(null)
  const [offsets, setOffsets] = useState<Offsets>({})
  const [zoom, setZoom] = useState<number | null>(null)
  const [legendOpen, setLegendOpen] = useState(false)
  const [animate, setAnimate] = useState(true)
  const groupDrag = useRef<{ ns: string; x: number; y: number } | null>(null)
  const [viewportWidth, setViewportWidth] = useState(CANVAS_WIDTH + 48)
  const viewportRef = useRef<HTMLDivElement>(null)
  const layout = useMemo(() => layoutTopology(topology), [topology])
  const scale =
    zoom ?? Math.max(0.65, Math.min(1, (viewportWidth - 48) / layout.width))
  const fit = () =>
    setZoom(Math.max(0.25, Math.min(1, (viewportWidth - 48) / canvas.contentWidth, 612 / canvas.contentHeight)))
  const selectedNode = topology.nodes.find((n) => n.id === selected)
  const highlight = useMemo(
    () =>
      selectedNode ? connectedNodes([selectedNode.id], topology.edges) : null,
    [selectedNode, topology.edges],
  )
  // Nodes can go anywhere right of and below the canvas origin; the canvas grows to follow them.
  // Only the top and left are fixed, so nothing is dragged out of reach (boxes keep their margin).
  const stageOf = useMemo(() => new Map(topology.nodes.map((n) => [n.id, n.stage])), [topology.nodes])
  const placed = (id: string): { at: Point; offset: Point } => {
    const at = layout.positions.get(id) ?? { x: 0, y: 0 }
    const raw = offsets[id] ?? { x: 0, y: 0 }
    const min = floorOf(stageOf.get(id) ?? 0)
    return { at, offset: { x: Math.max(raw.x, min.x - at.x), y: Math.max(raw.y, min.y - at.y) } }
  }
  const position = (id: string): Point => {
    const { at, offset } = placed(id)
    return { x: at.x + offset.x, y: at.y + offset.y }
  }
  // Namespace boxes are drawn around their nodes' current positions, so they follow drags.
  const members = useMemo(() => {
    const m = new Map<string, string[]>()
    for (const n of topology.nodes)
      if (n.stage > 0) m.set(n.namespace ?? EXTERNAL, [...(m.get(n.namespace ?? EXTERNAL) ?? []), n.id])
    return m
  }, [topology.nodes])
  const boxes = layout.bands.map(
    (band) => boxAround(band.namespace, (members.get(band.namespace) ?? []).map(position)) ?? band,
  )
  // Canvas: the layout, grown to wherever nodes were dragged, and at least as wide as the viewport.
  const canvas = (() => {
    const points = topology.nodes.map((n) => position(n.id))
    const contentWidth = Math.max(layout.width, ...points.map((p) => p.x + BOX_PAD_X + 16))
    const contentHeight = Math.max(layout.height, ...points.map((p) => p.y + BAND_BOTTOM + 16))
    return { contentWidth, contentHeight, width: Math.max(contentWidth, (viewportWidth - 48) / scale), height: contentHeight }
  })()
  // Dragging a box moves every node in it, like dragging a Kiali group.
  const moveGroup = (ns: string, dx: number, dy: number) =>
    setOffsets((previous) => {
      const next = { ...previous }
      for (const id of members.get(ns) ?? []) {
        const at = layout.positions.get(id) ?? { x: 0, y: 0 }
        const o = previous[id] ?? { x: 0, y: 0 }
        const min = floorOf(stageOf.get(id) ?? 0)
        next[id] = { x: Math.max(o.x + dx, min.x - at.x), y: Math.max(o.y + dy, min.y - at.y) }
      }
      return next
    })

  useLayoutEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const observer = new ResizeObserver(() =>
      setViewportWidth(viewport.clientWidth),
    )
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [])

  const resetView = () => {
    setOffsets({})
    setZoom(null)
    setSelected(null)
    viewportRef.current?.scrollTo({ left: 0, top: 0 })
  }

  return (
    <>
      <div className="flow-stage">
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
            width={canvas.width * scale}
            height={canvas.height * scale}
            viewBox={`0 0 ${canvas.width} ${canvas.height}`}
            style={{ margin: 24 }}
          >
            <defs>
              {(['', 'is-active', 'is-broken'] as const).map((variant) => (
                <marker
                  key={variant}
                  id={`flow-arrow${variant ? `-${variant}` : ''}`}
                  className={`flow-arrow ${variant}`}
                  viewBox="0 0 10 10"
                  refX="0"
                  refY="5"
                  markerUnits="userSpaceOnUse"
                  markerWidth={ARROW}
                  markerHeight={ARROW}
                  orient="auto-start-reverse"
                >
                  <path d="M 0 0 L 10 5 L 0 10 z" />
                </marker>
              ))}
            </defs>
            {boxes.map((band) => (
              <rect
                key={band.namespace || '*'}
                className="flow-ns"
                x={band.x}
                y={band.y}
                width={band.width}
                height={band.height}
                rx={6}
                onPointerDown={(e) => {
                  if (e.pointerType !== 'mouse' || e.button !== 0) return
                  groupDrag.current = { ns: band.namespace, x: e.clientX, y: e.clientY }
                  e.currentTarget.setPointerCapture(e.pointerId)
                }}
                onPointerMove={(e) => {
                  const drag = groupDrag.current
                  if (!drag) return
                  moveGroup(drag.ns, (e.clientX - drag.x) / scale, (e.clientY - drag.y) / scale)
                  drag.x = e.clientX
                  drag.y = e.clientY
                }}
                onPointerUp={() => (groupDrag.current = null)}
                onPointerCancel={() => (groupDrag.current = null)}
              />
            ))}
            {topology.edges.map((edge) => {
              const active =
                !!highlight && highlight.has(edge.from) && highlight.has(edge.to)
              const variant = edge.broken ? 'is-broken' : active ? 'is-active' : ''
              const dimmed = highlight && !active ? 'is-dimmed' : ''
              const d = edgePath(position(edge.from), position(edge.to))
              return (
                <g key={edgeKey(edge)}>
                  <path
                    d={d}
                    className={`flow-edge ${variant} ${dimmed}`}
                    fill="none"
                    markerEnd={`url(#flow-arrow${variant ? `-${variant}` : ''})`}
                  />
                  {/* Dots travelling along the edge, like Kiali's traffic animation; nothing flows down a broken path */}
                  {animate && !edge.broken && <path d={d} className={`flow-edge-motion ${variant} ${dimmed}`} />}
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
                floor={floorOf(node.stage)}
                onSelect={() =>
                  setSelected(selected === node.id ? null : node.id)
                }
                onMove={(offset) =>
                  setOffsets((previous) => ({ ...previous, [node.id]: offset }))
                }
              />
            ))}
            {/* Box labels last, so edges fanning out of a gateway never cover them */}
            {boxes.map((band) => (
              <NamespaceLabel key={band.namespace || '*'} band={band} />
            ))}
          </svg>
        </div>
        {legendOpen && <Legend />}
        {/* Control bar in the graph's lower left, like Kiali's (PatternFly topology control bar). */}
        <div className="flow-controlbar" role="group" aria-label={t('flow.viewControls')}>
          <button
            className="flow-control"
            aria-label={t('flow.zoomIn')}
            title={t('flow.zoomIn')}
            disabled={scale >= 1.5}
            onClick={() => setZoom(Math.min(1.5, scale + 0.1))}
          >
            <Icon name="plus" className="h-4 w-4" />
          </button>
          <button
            className="flow-control"
            aria-label={t('flow.zoomOut')}
            title={t('flow.zoomOut')}
            disabled={scale <= 0.25}
            onClick={() => setZoom(Math.max(0.25, scale - 0.1))}
          >
            <Icon name="minus" className="h-4 w-4" />
          </button>
          <button className="flow-control" aria-label={t('flow.fit')} title={t('flow.fit')} onClick={fit}>
            <Icon name="fit" className="h-4 w-4" />
          </button>
          <button className="flow-control" aria-label={t('flow.resetLayout')} title={t('flow.resetLayout')} onClick={resetView}>
            <Icon name="refresh" className="h-4 w-4" />
          </button>
          <span className="flow-zoom" aria-live="polite">
            {Math.round(scale * 100)}%
          </span>
          <button
            className={`flow-control flow-legend-toggle ${animate ? 'is-active' : ''}`}
            aria-pressed={animate}
            onClick={() => setAnimate(!animate)}
          >
            {t('flow.animation')}
          </button>
          <button
            className={`flow-control flow-legend-toggle ${legendOpen ? 'is-active' : ''}`}
            aria-pressed={legendOpen}
            onClick={() => setLegendOpen(!legendOpen)}
          >
            {t('flow.legend')}
          </button>
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

// Smallest position a node may be dragged to: boxed nodes keep their box inside the canvas.
const floorOf = (stage: number): Point =>
  stage === 0 ? { x: NODE_R + 8, y: NODE_R + 8 } : { x: BOX_PAD_X, y: BAND_PAD }

const shorten = (text: string, max = 18) =>
  text.length > max ? `${text.slice(0, max - 1)}…` : text

// Kiali's shape vocabulary (PatternFly topology): workload = ellipse, app = rect,
// aggregate = hexagon, service = rhombus, ServiceEntry = trapezoid. The request source
// is drawn like a workload, gateways like apps and routes like aggregates.
type ShapeKind = 'ellipse' | 'rect' | 'hexagon' | 'rhombus' | 'trapezoid'
const shapeOf = (node: FlowNode): ShapeKind =>
  node.stage === 0
    ? 'ellipse'
    : node.stage === 1
      ? 'rect'
      : node.stage === 2
        ? 'hexagon'
        : node.icon === 'globe'
          ? 'trapezoid'
          : 'rhombus'

function Shape({ kind, r = NODE_R, className = 'flow-shape' }: { kind: ShapeKind; r?: number; className?: string }) {
  const h = r * 0.866
  switch (kind) {
    case 'ellipse':
      return <circle className={className} r={r} />
    case 'rect':
      return <rect className={className} x={-r} y={-r} width={r * 2} height={r * 2} rx={r / 6} />
    case 'hexagon':
      return <polygon className={className} points={`${-r},0 ${-r / 2},${-h} ${r / 2},${-h} ${r},0 ${r / 2},${h} ${-r / 2},${h}`} />
    case 'rhombus':
      return <polygon className={className} points={`0,${-r} ${r},0 0,${r} ${-r},0`} />
    case 'trapezoid':
      return <polygon className={className} points={`${-r},${r * 0.75} ${-r * 0.55},${-r * 0.75} ${r * 0.55},${-r * 0.75} ${r},${r * 0.75}`} />
  }
}

// Route kind shown in a purple corner decorator, as Kiali marks Istio config on a node.
const routeBadge = (node: FlowNode) =>
  node.stage !== 2 ? '' : node.kind === 'VirtualService' ? 'VS' : node.kind === 'HTTPRoute' ? 'HR' : node.kind.slice(0, 2).toUpperCase()

// Outline status like Kiali's node health: red for a problem, green for a Service with ready
// endpoints; everything else has no health signal here (configuration only) and stays neutral.
const statusOf = (node: FlowNode) =>
  node.issue ? 'is-danger' : node.stage === 3 && node.endpoints !== undefined && node.endpoints > 0 ? 'is-success' : ''

// The label pill is sized from the text, roughly 7px per character at 12px.
const pillWidth = (text: string, icon: boolean) => Math.max(36, text.length * 7 + 18 + (icon ? 16 : 0))

function GraphNode({
  node,
  at,
  offset,
  selected,
  dimmed,
  scale,
  floor,
  onSelect,
  onMove,
}: {
  node: FlowNode
  at: Point
  offset: Point
  selected: boolean
  dimmed: boolean
  scale: number
  floor: Point
  onSelect: () => void
  onMove: (offset: Point) => void
}) {
  const drag = useRef<{ x: number; y: number; moved: boolean } | null>(null)
  const suppressClick = useRef(false)
  const sub =
    node.hosts[0] ??
    [node.port ? `:${node.port}` : '', node.stage === 3 ? node.status : '']
      .filter(Boolean)
      .join(' · ')
  const label = shorten(node.name)
  const gatewayIcon = node.stage === 1
  const width = pillWidth(label, gatewayIcon)
  const badge = routeBadge(node)
  return (
    <g
      className={`flow-node ${statusOf(node)} ${selected ? 'is-selected' : ''} ${dimmed ? 'is-dimmed' : ''}`}
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
          onMove({ x: Math.max(offset.x + dx, floor.x - at.x), y: Math.max(offset.y + dy, floor.y - at.y) })
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
        <rect className="flow-hit" x={-Math.max(30, width / 2)} y={-NODE_R - 8} width={Math.max(60, width)} height={NODE_R * 2 + 52} />
        <Shape kind={shapeOf(node)} />
        {badge && (
          <g className="flow-decorator is-config" transform={`translate(${NODE_R - 2} ${-NODE_R + 2})`}>
            <circle r={9} />
            <text y={3} textAnchor="middle">
              {badge}
            </text>
          </g>
        )}
        {node.issue && (
          <g className="flow-decorator is-status" transform={`translate(${-NODE_R + 2} ${-NODE_R + 2})`}>
            <circle className="flow-badge" r={9} />
            <text y={3.5} textAnchor="middle">
              !
            </text>
          </g>
        )}
        <g className="flow-pill" transform={`translate(0 ${NODE_R + 6})`}>
          <rect x={-width / 2} width={width} height={22} rx={11} />
          {gatewayIcon && (
            <g transform={`translate(${-width / 2 + 8} 5)`}>
              <Icon name="shield" className="flow-pill-icon" size={12} />
            </g>
          )}
          <text className="flow-label" x={gatewayIcon ? 8 : 0} y={15} textAnchor="middle">
            {label}
          </text>
        </g>
        {sub && (
          <text className="flow-sublabel" y={NODE_R + 44} textAnchor="middle">
            {shorten(sub, 24)}
          </text>
        )}
      </g>
    </g>
  )
}

// Label hanging on a namespace box's bottom edge: a dark pill with an "NS" badge, the way Kiali
// labels namespace boxes.
function NamespaceLabel({ band }: { band: Band }) {
  const name = band.namespace || t('flow.legend.external')
  const ns = !!band.namespace
  const width = name.length * 8 + 22 + (ns ? 30 : 0) // 12px semibold runs about 8px per character
  const cx = band.x + band.width / 2
  const y = band.y + band.height
  return (
    <g className="flow-ns-label" transform={`translate(${cx - width / 2} ${y - 12})`}>
        <rect width={width} height={24} rx={12} />
        {ns && (
          <g transform="translate(6 4)">
            <rect className="flow-ns-badge" width={24} height={16} rx={8} />
            <text x={12} y={12} textAnchor="middle" className="flow-ns-badge-text">
              NS
            </text>
          </g>
        )}
      <text className="flow-ns-name" x={(ns ? 30 : 0) + (width - (ns ? 30 : 0)) / 2} y={16} textAnchor="middle">
        {name}
      </text>
    </g>
  )
}

// Legend panel opened from the control bar, like Kiali's graph legend.
function Legend() {
  const shapes: [ShapeKind, string][] = [
    ['ellipse', t('flow.source')],
    ['rect', t('flow.gateways')],
    ['hexagon', t('flow.routes')],
    ['rhombus', t('flow.legend.service')],
    ['trapezoid', t('flow.legend.external')],
  ]
  return (
    <div className="flow-legend" role="group" aria-label={t('flow.legend')}>
      <p className="flow-legend-title">{t('flow.legend.nodes')}</p>
      {shapes.map(([kind, text]) => (
        <span key={kind}>
          <svg aria-hidden="true" className="flow-legend-shape" viewBox="-12 -12 24 24">
            <Shape kind={kind} r={9} />
          </svg>
          {text}
        </span>
      ))}
      <p className="flow-legend-title">{t('flow.legend.marks')}</p>
      <span>
        <svg aria-hidden="true" className="flow-legend-shape" viewBox="-12 -12 24 24">
          <Shape kind="rhombus" r={9} className="flow-shape is-success" />
        </svg>
        {t('flow.legend.ready')}
      </span>
      <span>
        <svg aria-hidden="true" className="flow-legend-shape" viewBox="-12 -12 24 24">
          <circle className="flow-legend-config" r={9} />
          <text y={3} textAnchor="middle" className="flow-legend-config-text">
            VS
          </text>
        </svg>
        {t('flow.legend.routeKind')}
      </span>
      <span>
        <svg aria-hidden="true" className="flow-legend-shape" viewBox="-12 -12 24 24">
          <circle className="flow-legend-status" r={9} />
          <text y={3.5} textAnchor="middle" className="flow-legend-status-text">
            !
          </text>
        </svg>
        {t('flow.legendBroken')}
      </span>
      <p className="flow-legend-title">{t('flow.legend.edges')}</p>
      <span>
        <span className="flow-legend-line" /> {t('flow.legendConfigured')}
      </span>
      <span>
        <span className="flow-legend-line is-broken" /> {t('flow.legendBroken')}
      </span>
    </div>
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
