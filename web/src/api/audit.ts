import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

// History of changes made through Sealane (in server memory; reset when the pod restarts).
export interface AuditEntry {
  time: string
  user?: string
  verb: 'create' | 'update' | 'delete'
  typeId: string
  namespace: string
  name: string
  ok: boolean
}

export function useAudit() {
  return useQuery({
    queryKey: ['audit'],
    queryFn: () => apiGet<AuditEntry[]>('/api/audit'),
    refetchInterval: 30_000, // pick up other users' changes periodically
  })
}
