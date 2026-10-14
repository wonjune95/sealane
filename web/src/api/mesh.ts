import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'
import type { Finding, Ref } from './analyze'
import type { ServiceRef } from './services'

export interface PolicyRef extends Ref { scope: 'workload' | 'namespace' | 'mesh' }
export interface VsRef extends Ref { match: 'host' | 'destination' }
export interface ServiceMesh {
  namespace: string; name: string; selector?: Record<string, string>; ports: number[]; endpoints: number
  virtualServices: VsRef[]; destinationRules: Ref[]; httpRoutes: Ref[]; gateways: Ref[]; policies: PolicyRef[]; findings: Finding[]
}

export function useServiceList(ns: string) {
  return useQuery({ queryKey: ['services-all', ns], queryFn: () => apiGet<ServiceRef[]>(`/api/services?ns=${encodeURIComponent(ns)}&mesh=true`) })
}
export function useServiceMesh(ns: string, name: string) {
  return useQuery({ queryKey: ['service-mesh', ns, name], queryFn: () => apiGet<ServiceMesh>(`/api/services/${ns}/${name}/mesh`), refetchInterval: 30_000 })
}
