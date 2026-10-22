import type { FlowEdge, FlowNode, Topology } from './flowTopology'

// Kiali-style layered layout: stages become columns (left → right) and every
// namespace gets its own horizontal band so namespace boxes never overlap.
// Nodes without a namespace (external hosts) share the EXTERNAL band; the
// request source sits alone in column 0, vertically centred.
export const EXTERNAL = ''
export const COLUMN_X = [60, 260, 500, 740]
export const ROW_GAP = 84
export const BAND_PAD = 56 // room for the band label above the first row (badge and ring reach 31px above a node centre)
export const BAND_GAP = 28
export const NODE_R = 24
export const CANVAS_WIDTH = COLUMN_X[3] + 160

export type Point = { x: number; y: number }
export type Band = { namespace: string; x: number; y: number; width: number; height: number }
export type Layout = { width: number; height: number; positions: Map<string, Point>; bands: Band[] }

export function layoutTopology({ nodes, edges }: Topology): Layout {
  const positions = new Map<string, Point>()
  const bands: Band[] = []
  const bandOrder: string[] = []
  const byBand = new Map<string, FlowNode[][]>() // namespace → column → nodes
  for (const node of nodes) {
    if (node.stage === 0) continue
    const ns = node.namespace ?? EXTERNAL
    if (!byBand.has(ns)) {
      byBand.set(ns, [[], [], [], []])
      bandOrder.push(ns)
    }
    byBand.get(ns)![node.stage].push(node)
  }
  // External hosts go last, like Kiali's "unknown"/external box.
  bandOrder.sort((a, b) => Number(a === EXTERNAL) - Number(b === EXTERNAL))

  const predecessors = new Map<string, string[]>()
  for (const edge of edges)
    predecessors.set(edge.to, [...(predecessors.get(edge.to) ?? []), edge.from])

  let y = BAND_GAP
  for (const ns of bandOrder) {
    const columns = byBand.get(ns)!
    const rows = Math.max(...columns.map((c) => c.length))
    const top = y
    const height = BAND_PAD + rows * ROW_GAP
    for (let stage = 1; stage <= 3; stage++) {
      // Barycenter ordering: a node goes near the average y of its already placed predecessors
      // (routes under their gateways, services under their routes); gateways keep insertion order.
      const column = columns[stage]
        .map((node, index) => {
          const ys = (predecessors.get(node.id) ?? [])
            .map((id) => positions.get(id)?.y)
            .filter((v): v is number => v !== undefined)
          return { node, key: ys.length ? ys.reduce((a, b) => a + b, 0) / ys.length : top + BAND_PAD + index * ROW_GAP }
        })
        .sort((a, b) => a.key - b.key)
      // Centre the column's rows inside the band.
      const offset = ((rows - column.length) * ROW_GAP) / 2
      column.forEach(({ node }, index) =>
        positions.set(node.id, { x: COLUMN_X[stage], y: top + BAND_PAD + offset + index * ROW_GAP }),
      )
    }
    bands.push({ namespace: ns, x: COLUMN_X[1] - 80, y: top, width: COLUMN_X[3] - COLUMN_X[1] + 160, height })
    y = top + height + BAND_GAP
  }
  const height = Math.max(y, BAND_GAP + BAND_PAD + ROW_GAP + BAND_GAP)
  for (const node of nodes)
    if (node.stage === 0) positions.set(node.id, { x: COLUMN_X[0], y: height / 2 })
  return { width: CANVAS_WIDTH, height, positions, bands }
}

// Edge path from the right side of `from` to the left side of `to`, as a cubic curve.
export function edgePath(from: Point, to: Point) {
  const x1 = from.x + NODE_R + 2,
    x2 = to.x - NODE_R - 8 // leave room for the arrowhead
  const bend = Math.max(32, (x2 - x1) / 2)
  return `M ${x1} ${from.y} C ${x1 + bend} ${from.y}, ${x2 - bend} ${to.y}, ${x2} ${to.y}`
}

export const edgeKey = (edge: FlowEdge) => `${edge.from}>${edge.to}`
