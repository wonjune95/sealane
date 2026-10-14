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
