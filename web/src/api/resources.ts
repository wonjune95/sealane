import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export interface ResourceSummary {
  typeId: string
  kind: string
  namespace: string
  name: string
  summary: string
  status?: string
  age: string
  resourceVersion: string
  managedBy?: ManagedBy
}

// The tool that owns the resource (from Argo CD, Flux or Helm metadata); edits made here get undone.
export interface ManagedBy {
  tool: 'argocd' | 'flux' | 'helm'
  name: string
}

export interface ResourceDetail extends ResourceSummary {
  raw: unknown
}

export function useResources(type: string, ns: string) {
  return useQuery({
    queryKey: ['resources', type, ns],
    queryFn: () => apiGet<ResourceSummary[]>(`/api/resources/${type}${ns ? `?ns=${ns}` : ''}`),
  })
}

export function useResource(type: string, ns: string, name: string, enabled: boolean) {
  return useQuery({
    queryKey: ['resource', type, ns, name],
    queryFn: () => apiGet<ResourceDetail>(`/api/resources/${type}/${ns}/${name}`),
    enabled,
  })
}
