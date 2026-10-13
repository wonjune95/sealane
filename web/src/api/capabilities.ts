import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

// Matches the backend capabilities contract (DESIGN.md §4.1).
export interface Capabilities {
  httpRouteInstalled: boolean
  virtualServiceInstalled: boolean
  gatewayAPIVersion?: string
  istioApiVersion?: string
  istiodVersion?: string // actual control-plane version, e.g. "1.30.2"
  namespaceListAllowed: boolean
  devMode: boolean
  user?: string
  role?: string // admin | editor | viewer
  mustChangePassword?: boolean // still on the initial (admin) password; the UI is locked until it is changed
  requestTester?: boolean // real request tester enabled (Helm opt-in, admin only)
}

export function useCapabilities(enabled = true) {
  return useQuery({
    queryKey: ['capabilities'],
    queryFn: () => apiGet<Capabilities>('/api/capabilities'),
    staleTime: 5 * 60 * 1000,
    enabled,
  })
}
