import { t, type Key } from '../i18n'

const codes = new Set([
  'vs-host-missing',
  'vs-subset-undefined',
  'vs-gateway-missing',
  'vs-host-conflict',
  'backend-no-endpoints',
  'httproute-backend-missing',
  'httproute-not-accepted',
  'gateway-not-programmed',
  'dr-host-missing',
  'dr-subset-no-pods',
  'gateway-selector-no-pods',
  'namespace-injection-no-sidecar',
])

export const hintKey = (code: string): Key | null => (codes.has(code) ? (`hint.${code}` as Key) : null)
export const hintText = (code: string) => {
  const k = hintKey(code)
  return k && t(k)
}
