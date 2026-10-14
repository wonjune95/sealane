import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export interface ServiceRef {
  namespace: string
  name: string
  ports: number[]
  // set by /api/services?mesh=true (omitted when 0)
  routes?: number
  destinationRules?: number
  policies?: number
  findings?: number
}

export function useServices(ns: string) {
  return useQuery({
    queryKey: ['services', ns],
    queryFn: () => apiGet<ServiceRef[]>(`/api/services?ns=${ns}`),
    enabled: !!ns,
  })
}
