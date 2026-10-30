import type { FlowEdge, FlowNode, Topology } from './flowTopology'

// Kiali-style layered layout: stages become columns (left → right) and every
// namespace gets its own box (band) so boxes never overlap. Nodes without a
// namespace (external hosts) share the EXTERNAL band; the request source sits
// alone in column 0, vertically centred. Each box's label hangs on its bottom
// edge, as in Kiali, so BAND_GAP leaves room for it.
export const EXTERNAL = ''
export const COLUMN_X = [60, 260, 500, 740]
export const NODE_R = 20 // nodes are 40 × 40, like Kiali's
export const ROW_GAP = 96 // node + label pill + sublabel
export const BAND_PAD = 44 // above the first row: room for decorators
export const BAND_BOTTOM = 80 // below the last row's centre: node half, label pill, sublabel, padding
export const BAND_GAP = 48 // between boxes: the hanging box label plus air
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
  const span = (ns: string) => {
    const used = byBand.get(ns)!.flatMap((c, stage) => (c.length ? [stage] : []))
    return { from: Math.min(...used), to: Math.max(...used) }
  }
  // Left-most stages first so predecessors are placed before the nodes they order;
  // external hosts go last, like Kiali's "unknown"/external box.
  bandOrder.sort((a, b) => Number(a === EXTERNAL) - Number(b === EXTERNAL) || span(a).from - span(b).from)

  // Boxes whose columns do not overlap share a lane, side by side (gateways' box next to the
  // routes' box, as Kiali's dagre layout puts them); a lane is as tall as its tallest box.
  const lanes: string[][] = []
  for (const ns of bandOrder) {
    const s = span(ns)
    const lane = lanes.find((l) => l.every((other) => span(other).to < s.from || span(other).from > s.to))
    if (lane) lane.push(ns)
    else lanes.push([ns])
  }
  const rowsOf = (ns: string) => Math.max(...byBand.get(ns)!.map((c) => c.length))
  const heightOf = (ns: string) => BAND_PAD + (rowsOf(ns) - 1) * ROW_GAP + BAND_BOTTOM

  const predecessors = new Map<string, string[]>()
  for (const edge of edges)
    predecessors.set(edge.to, [...(predecessors.get(edge.to) ?? []), edge.from])

  let y = BAND_GAP / 2
  for (const lane of lanes) {
    const laneHeight = Math.max(...lane.map(heightOf))
    for (const ns of lane) {
      const columns = byBand.get(ns)!
      const rows = rowsOf(ns)
      const height = heightOf(ns)
      const top = y + (laneHeight - height) / 2 // shorter boxes sit centred in the lane
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
      const { from, to } = span(ns)
      bands.push({ namespace: ns, x: COLUMN_X[from] - 80, y: top, width: COLUMN_X[to] - COLUMN_X[from] + 160, height })
    }
    y += laneHeight + BAND_GAP
  }
  const height = Math.max(y - BAND_GAP / 2, BAND_GAP + BAND_PAD + BAND_BOTTOM)
  for (const node of nodes)
    if (node.stage === 0) positions.set(node.id, { x: COLUMN_X[0], y: height / 2 })
  return { width: CANVAS_WIDTH, height, positions, bands }
}

// Straight edge between the two node outlines, as Kiali draws them; the end stops short
// so the arrowhead (ARROW long) touches the target.
export const ARROW = 10
export function edgePath(from: Point, to: Point) {
  const dx = to.x - from.x,
    dy = to.y - from.y
  const len = Math.hypot(dx, dy) || 1
  const ux = dx / len,
    uy = dy / len
  const start = NODE_R + 2,
    end = NODE_R + 2 + ARROW
  return `M ${from.x + ux * start} ${from.y + uy * start} L ${to.x - ux * end} ${to.y - uy * end}`
}

export const edgeKey = (edge: FlowEdge) => `${edge.from}>${edge.to}`
