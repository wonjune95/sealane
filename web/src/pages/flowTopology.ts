import type { FlowMap as FlowData, FlowBackend } from '../api/flowmap'
import { t } from '../i18n'

export type FlowNode = {
  id: string
  name: string
  kind: string
  namespace?: string
  hosts: string[]
  stage: number
  icon: string
  tone: 'blue' | 'violet' | 'green' | 'red' | 'slate'
  status?: string
  issue?: string
  to?: string
  port?: number
  endpoints?: number
}
export type FlowEdge = { from: string; to: string; broken?: boolean }
export type Topology = { nodes: FlowNode[]; edges: FlowEdge[] }
export const backendId = (b: FlowBackend) =>
  b.external
    ? `b:ext:${b.name}:${b.port ?? 0}`
    : `b:${b.namespace}/${b.name}:${b.port ?? 0}`

// Search/filter from a matching node in both directions to retain its full path.
// Walks have separate visited sets so cycles and shared nodes cannot skip branches.
export function connectedNodes(ids: string[], edges: FlowEdge[]) {
  const result = new Set(ids)
  for (const reverse of [false, true]) {
    const visited = new Set(ids)
    const queue = [...ids]
    const adjacency = new Map<string, string[]>()
    for (const edge of edges) {
      const from = reverse ? edge.to : edge.from
      const to = reverse ? edge.from : edge.to
      adjacency.set(from, [...(adjacency.get(from) ?? []), to])
    }
    while (queue.length) {
      for (const next of adjacency.get(queue.pop()!) ?? []) {
        if (visited.has(next)) continue
        visited.add(next)
        result.add(next)
        queue.push(next)
      }
    }
  }
  return result
}

export function buildTopology(
  data: FlowData | undefined,
  direction: 'ingress' | 'egress',
): Topology {
  const isEgress = direction === 'egress'
  const gateways = data?.gateways ?? []
  const routes = data?.routes ?? []
  const egressIds = new Set(
    gateways.filter((g) => g.egress).map((g) => `${g.namespace}/${g.name}`),
  )
  const nodes = new Map<string, FlowNode>()
  const edges = new Map<string, FlowEdge>()
  const addEdge = (from: string, to: string, broken = false) =>
    edges.set(`${from}>${to}`, { from, to, broken })
  nodes.set('src', {
    id: 'src',
    name: isEgress ? t('flow.node.mesh') : t('flow.node.externalClient'),
    kind: isEgress ? t('flow.node.meshInternal') : t('flow.node.externalRequest'),
    hosts: [],
    stage: 0,
    icon: isEgress ? 'network' : 'globe',
    tone: 'blue',
  })
  for (const gateway of gateways.filter((g) => !!g.egress === isEgress)) {
    const id = `g:${gateway.namespace}/${gateway.name}`
    nodes.set(id, {
      id,
      name: gateway.name,
      namespace: gateway.namespace,
      kind: gateway.kind === 'IstioGateway' ? 'Istio Gateway' : 'Gateway API',
      hosts: gateway.hosts,
      stage: 1,
      icon: 'shield',
      tone: 'blue',
      to: `/resources/${gateway.typeId}/${gateway.namespace}/${gateway.name}`,
    })
    addEdge('src', id)
  }
  for (const route of routes) {
    const parents = route.gateways.filter(
      (id) => egressIds.has(id) === isEgress,
    )
    if (!parents.length && (isEgress || route.gateways.length)) continue
    const id = `r:${route.kind}/${route.namespace}/${route.name}`
    nodes.set(id, {
      id,
      name: route.name,
      namespace: route.namespace,
      kind: route.kind,
      hosts: route.hosts,
      stage: 2,
      icon: 'route',
      tone: 'violet',
      to: `/resources/${route.typeId}/${route.namespace}/${route.name}`,
    })
    for (const parent of parents) {
      const gatewayId = `g:${parent}`
      if (!nodes.has(gatewayId)) {
        const [namespace, name] = parent.split('/')
        nodes.set(gatewayId, {
          id: gatewayId,
          namespace,
          name,
          kind: 'Gateway',
          hosts: [],
          stage: 1,
          icon: 'warning',
          tone: 'red',
          issue: t('flow.issue.gatewayNotFound'),
          status: t('flow.status.noGateway'),
        })
        addEdge('src', gatewayId, true)
      }
      addEdge(gatewayId, id, !!nodes.get(gatewayId)?.issue)
    }
    for (const backend of route.backends) {
      const bid = backendId(backend)
      const broken =
        !backend.external && (!backend.exists || backend.endpoints === 0)
      nodes.set(bid, {
        id: bid,
        name: backend.name,
        namespace: backend.namespace,
        kind: backend.external ? t('flow.node.externalHost') : 'Service',
        hosts: [],
        stage: 3,
        icon: backend.external ? 'globe' : 'cube',
        tone: broken ? 'red' : backend.external ? 'slate' : 'green',
        port: backend.port,
        endpoints: backend.external ? undefined : backend.endpoints,
        status: backend.external
          ? backend.serviceEntry
            ? `ServiceEntry · ${backend.serviceEntry}`
            : t('flow.status.external')
          : !backend.exists
            ? t('flow.status.noService')
            : t('flow.status.endpoints', { n: backend.endpoints }),
        issue: broken
          ? !backend.exists
            ? t('flow.issue.serviceMissing')
            : t('flow.issue.noEndpoints')
          : undefined,
      })
      addEdge(id, bid, broken)
    }
  }
  if (isEgress) {
    const referenced = new Set(
      routes.flatMap((r) =>
        r.backends.map((b) => b.serviceEntry).filter(Boolean),
      ),
    )
    for (const entry of data?.serviceEntries ?? []) {
      if (referenced.has(entry.name)) continue
      const id = `se:${entry.namespace}/${entry.name}`
      nodes.set(id, {
        id,
        name: entry.name,
        namespace: entry.namespace,
        kind: 'ServiceEntry',
        hosts: entry.hosts,
        stage: 3,
        icon: 'globe',
        tone: 'green',
        status: t('flow.status.direct'),
        to: `/resources/${entry.typeId}/${entry.namespace}/${entry.name}`,
      })
      addEdge('src', id)
    }
  }
  return { nodes: [...nodes.values()], edges: [...edges.values()] }
}

export function filterTopology(
  topology: Topology,
  query: string,
  namespace: string,
  issuesOnly: boolean,
): Topology {
  if (!query.trim() && !namespace && !issuesOnly) return topology
  const search = query.trim().toLocaleLowerCase()
  const matches = topology.nodes.filter(
    (node) =>
      node.id !== 'src' &&
      (!namespace || node.namespace === namespace) &&
      (!issuesOnly || node.issue) &&
      `${node.name} ${node.namespace ?? ''} ${node.kind} ${node.hosts.join(' ')}`
        .toLocaleLowerCase()
        .includes(search),
  )
  const visible = connectedNodes(
    matches.map((node) => node.id),
    topology.edges,
  )
  return {
    nodes: topology.nodes.filter((node) => visible.has(node.id)),
    edges: topology.edges.filter(
      (edge) => visible.has(edge.from) && visible.has(edge.to),
    ),
  }
}
