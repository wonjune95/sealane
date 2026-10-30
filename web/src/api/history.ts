import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'
import type { AuditEntry } from './audit'

// A stored revision with the objects it changed; null when not kept (or none: no before for a
// create, no after for a delete).
export interface RevisionDetail extends AuditEntry {
  before: unknown | null
  after: unknown | null
}

export function useHistory(type: string, ns: string, name: string) {
  const q = new URLSearchParams({ type, ns, name })
  return useQuery({
    queryKey: ['history', type, ns, name],
    queryFn: () => apiGet<AuditEntry[]>(`/api/history?${q}`),
  })
}

export const getRevision = (id: string) => apiGet<RevisionDetail>(`/api/history/${encodeURIComponent(id)}`)

// The version a revision leaves behind: what it applied, or for a delete what it removed.
export const revisionObject = (r: RevisionDetail) => (r.verb === 'delete' ? r.before : r.after)
