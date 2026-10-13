import { locale } from '../i18n'

// Light-gray example placeholders for common Istio/Gateway API fields, so an empty
// form shows the expected shape ("oh, that's the format") instead of a blank box.
// Same fallback chain as fieldHelp: "{typeId}/{field}" then global "{field}";
// the active locale first, then English.
// Only curated fields get an example — a wrong guess is worse than none.
const EXAMPLE: Record<'en' | 'ko', Record<string, string>> = {
  en: {
    host: 'e.g. reviews or reviews.default.svc.cluster.local',
    hosts: 'e.g. bookinfo.com',
    hostnames: 'e.g. bookinfo.com',
    gateways: 'e.g. my-gateway (mesh for sidecars)',
    subset: 'e.g. v1',
    number: 'e.g. 8080',
    port: 'e.g. 8080',
    weight: 'e.g. 50',
    timeout: 'e.g. 5s',
    protocol: 'e.g. HTTP / HTTPS / TCP / TLS / GRPC',
    resolution: 'e.g. DNS / STATIC / NONE',
    location: 'e.g. MESH_EXTERNAL',
    action: 'e.g. ALLOW / DENY / AUDIT',
    prefix: 'e.g. /api',
    uri: 'e.g. /v1/products',
    exportTo: 'e.g. . (current NS) or *',
    address: 'e.g. 10.0.0.1',
    mode: 'e.g. SIMPLE / MUTUAL',
    credentialName: 'e.g. my-tls-secret',
    value: 'e.g. a text value',
  },
  ko: {
    host: '예: reviews 또는 reviews.default.svc.cluster.local',
    hosts: '예: bookinfo.com',
    hostnames: '예: bookinfo.com',
    gateways: '예: my-gateway (사이드카는 mesh)',
    subset: '예: v1',
    number: '예: 8080',
    port: '예: 8080',
    weight: '예: 50',
    timeout: '예: 5s',
    protocol: '예: HTTP / HTTPS / TCP / TLS / GRPC',
    resolution: '예: DNS / STATIC / NONE',
    location: '예: MESH_EXTERNAL',
    action: '예: ALLOW / DENY / AUDIT',
    prefix: '예: /api',
    uri: '예: /v1/products',
    exportTo: '예: . (현재 NS) 또는 *',
    address: '예: 10.0.0.1',
    mode: '예: SIMPLE / MUTUAL',
    credentialName: '예: my-tls-secret',
    value: '예: 텍스트 값',
  },
}

export function fieldExample(typeId: string, field: string): string | undefined {
  const d = EXAMPLE[locale]
  return d[`${typeId}/${field}`] ?? d[field] ?? EXAMPLE.en[`${typeId}/${field}`] ?? EXAMPLE.en[field]
}
