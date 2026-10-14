import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export interface SubsetRef {
  namespace: string
  dr: string
  host: string
  name: string
}

export function useSubsets(ns: string) {
  return useQuery({
    queryKey: ['subsets', ns],
    queryFn: () => apiGet<SubsetRef[]>(`/api/subsets?ns=${ns}`),
    enabled: !!ns,
    staleTime: 30_000,
  })
}
