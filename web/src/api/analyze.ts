import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export interface Ref { typeId: string; kind: string; namespace: string; name: string }
export interface Finding {
  severity: 'error' | 'warning'
  code: string
  message: string
  resource: Ref
  related?: Ref[]
  service?: string
}
export interface Analysis { findings: Finding[]; skipped?: string[]; checked: number }

export function useAnalysis(ns: string, poll = true) {
  return useQuery({
    queryKey: ['analyze', ns],
    queryFn: () => apiGet<Analysis>(`/api/analyze${ns ? `?ns=${encodeURIComponent(ns)}` : ''}`),
    refetchInterval: poll ? 30_000 : false,
  })
}
