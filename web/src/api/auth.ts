import { useQuery } from '@tanstack/react-query'
import { apiGet } from './client'

export interface AuthMethods {
  local: boolean
  oidc?: { enabled: boolean; displayName: string }
}

// Unauthenticated: the login page asks which sign-in methods the server offers.
export function useAuthMethods() {
  return useQuery({ queryKey: ['auth-methods'], queryFn: () => apiGet<AuthMethods>('/api/auth/methods'), staleTime: 5 * 60 * 1000, retry: 0 })
}
