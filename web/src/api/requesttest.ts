import { useQuery } from '@tanstack/react-query'
import { apiGet, apiPost } from './client'

export type PodRef = { name: string; containers: string[]; mesh: boolean; ready: boolean }

export type RequestTestResult = {
  status?: number
  statusText?: string
  headers?: Record<string, string>
  body?: string
  durationMs: number
  truncated?: boolean
  error?: string
  hint?: string
}

// Only answers admins with the request tester enabled (403 otherwise).
export function usePods(ns: string, enabled: boolean) {
  return useQuery({
    queryKey: ['pods', ns],
    queryFn: () => apiGet<PodRef[]>(`/api/pods${ns ? `?ns=${encodeURIComponent(ns)}` : ''}`),
    enabled,
    staleTime: 30_000,
  })
}

// Sends a real HTTP request from inside the chosen pod; this has side effects.
export function sendRequestTest(input: {
  namespace: string
  pod: string
  container: string
  method: string
  url: string
  headers: Record<string, string>
  body?: string
}) {
  return apiPost<RequestTestResult>('/api/requesttest', input)
}
