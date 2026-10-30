import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

// History of changes made through Sealane, kept in the cluster (in server memory only in dev mode,
// where entries have no id).
export interface AuditEntry {
  id?: string
  cluster?: string // empty or missing = local
  time: string
  user?: string
  provider?: 'local' | 'oidc'
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
