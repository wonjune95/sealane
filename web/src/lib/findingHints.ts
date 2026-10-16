import { t, type Key } from '../i18n'

const HINTS = {
  'vs-host-missing': 'hint.vs-host-missing',
  'vs-subset-undefined': 'hint.vs-subset-undefined',
  'vs-port-not-exposed': 'hint.vs-port-not-exposed',
  'vs-gateway-missing': 'hint.vs-gateway-missing',
  'vs-host-conflict': 'hint.vs-host-conflict',
  'backend-no-endpoints': 'hint.backend-no-endpoints',
  'httproute-backend-missing': 'hint.httproute-backend-missing',
  'httproute-port-not-exposed': 'hint.httproute-port-not-exposed',
  'httproute-not-accepted': 'hint.httproute-not-accepted',
  'gateway-not-programmed': 'hint.gateway-not-programmed',
  'dr-host-missing': 'hint.dr-host-missing',
  'dr-subset-no-pods': 'hint.dr-subset-no-pods',
  'gateway-selector-no-pods': 'hint.gateway-selector-no-pods',
  'namespace-injection-no-sidecar': 'hint.namespace-injection-no-sidecar',
} as const satisfies Record<string, Key>

export const hintKey = (code: string): Key | null => HINTS[code as keyof typeof HINTS] ?? null
export const hintText = (code: string) => {
  const k = hintKey(code)
  return k && t(k)
}
