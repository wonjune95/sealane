// Mirrors the backend error envelope.
export class ApiError extends Error {
  status: number
  reason: string
  constructor(status: number, reason: string, message: string) {
    super(message)
    this.status = status
    this.reason = reason
  }
}

// Multi-cluster: the selected cluster is appended to every API call as ?cluster=.
// Switching does a full reload, so the react-query cache (not keyed per cluster) resets wholesale.
const CLUSTER_KEY = 'sealane-cluster'
export const getCluster = () => localStorage.getItem(CLUSTER_KEY) ?? ''
export function setCluster(name: string, redirectTo?: string) {
  if (name && name !== 'local') localStorage.setItem(CLUSTER_KEY, name)
  else localStorage.removeItem(CLUSTER_KEY)
  if (redirectTo) window.location.assign(redirectTo)
  else window.location.reload()
}
function withClusterParam(path: string) {
  // Accounts and cluster registrations belong to Sealane's host cluster.
  if (/^\/api\/(?:clusters|accounts|account|login|logout)(?:\/|\?|$)/.test(path)) return path
  const c = getCluster()
  if (!c) return path
  return path + (path.includes('?') ? '&' : '?') + 'cluster=' + encodeURIComponent(c)
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  // Auth is the HttpOnly session cookie (sent automatically with same-origin fetch).
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(withClusterParam(path), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    let reason = 'Error'
    let message = res.statusText
    try {
      const env = await res.json()
      if (env?.error) {
        reason = env.error.reason ?? reason
        message = env.error.message ?? message
      }
    } catch {
      // non-JSON error body; keep status text
    }
    throw new ApiError(res.status, reason, message)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const apiGet = <T>(path: string) => request<T>('GET', path)
export const apiPost = <T>(path: string, body: unknown) => request<T>('POST', path, body)
export const apiPut = <T>(path: string, body: unknown) => request<T>('PUT', path, body)
export const apiDelete = (path: string) => request<void>('DELETE', path)
