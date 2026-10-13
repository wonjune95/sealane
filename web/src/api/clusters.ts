import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export type Cluster = { name: string; connected?: boolean; version?: string; error?: string }

// "local" + the remote clusters registered in the Secret. status=true adds connection state and version
// (parallel ping, for the settings panel; the header dropdown uses the fast stateless list).
export function useClusters(status = false) {
  return useQuery({
    queryKey: ['clusters', status],
    queryFn: () => apiGet<Cluster[]>(`/api/clusters${status ? '?status=true' : ''}`),
    staleTime: 60_000,
  })
}
