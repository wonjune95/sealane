import { apiGet } from './client'

export type RouteDest = {
  host: string
  namespace?: string
  name?: string
  subset?: string
  port?: number
  weight?: number
  external?: boolean
  exists: boolean
  endpoints: number
  subsetOk?: boolean
  subsetDr?: string
}
export type RouteCandidate = {
  kind: string
  typeId: string
  namespace: string
  name: string
  ruleIndex: number
  matched: boolean
  reason: string
  winner?: boolean
}
export type RouteMatchResult = {
  matched: boolean
  destinations: RouteDest[]
  candidates: RouteCandidate[]
  notes?: string[]
}

// Simulation computed from config alone; no real traffic is sent.
export function checkRoute(input: {
  host: string
  path: string
  method: string
  headers: { key: string; value: string }[]
}) {
  const p = new URLSearchParams()
  p.set('host', input.host)
  p.set('path', input.path || '/')
  p.set('method', input.method)
  for (const h of input.headers) {
    if (h.key.trim()) p.append('header', `${h.key.trim()}:${h.value.trim()}`)
  }
  return apiGet<RouteMatchResult>(`/api/routematch?${p.toString()}`)
}
