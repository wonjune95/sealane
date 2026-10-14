import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const capabilities = {
  httpRouteInstalled: true,
  virtualServiceInstalled: true,
  namespaceListAllowed: true,
  devMode: false,
  user: 'test-user',
  role: 'admin',
  requestTester: true,
}
// The route matched, but the Service only exposes 8080 while the request lands on 80.
const route = {
  matched: true,
  destinations: [
    { host: 'echo', namespace: 'ns', name: 'echo', exists: true, endpoints: 2, portOk: false, servicePorts: [8080, 9090] },
  ],
  candidates: [],
  notes: ['destination ns/echo does not expose port 80 (service ports: 8080, 9090) — Envoy will answer 503'],
}
// Envoy's own 503: no upstream service time, no body hint from the backend.
const sent = { status: 503, statusText: 'Service Unavailable', headers: { server: 'envoy' }, body: '', durationMs: 3 }
let routeQuery
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': capabilities,
  '/api/namespaces': ['ns'],
  '/api/pods': [{ name: 'sleep-1', containers: ['sleep', 'istio-proxy'], mesh: true, ready: true }],
  '/api/requesttest': sent,
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const url = new URL(interception.request().url())
  if (!url.pathname.startsWith('/api/')) return interception.continue()
  if (url.pathname === '/api/routematch') {
    routeQuery = url.searchParams
    return interception.fulfill({ json: route })
  }
  return interception.fulfill({ json: fixtures[url.pathname] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/routecheck`)
  await page.getByLabel('Request URL').fill('http://echo.ns.svc.cluster.local/')
  await page.getByRole('button', { name: 'Check route' }).click()
  await page.getByText('Port 80 not exposed (service ports: 8080, 9090)').waitFor()
  assert.equal(routeQuery.get('port'), '80')
  assert.match(await page.getByText('Port 80 not exposed').getAttribute('class'), /text-red-600/)

  await page.getByLabel('Source namespace').selectOption('ns')
  await page.getByLabel('Source pod').selectOption('sleep-1')
  await page.getByRole('button', { name: 'Send request' }).click()
  await page.getByText('503 from the sidecar with no upstream', { exact: false }).waitFor()

  assert.deepEqual(errors, [])
  console.log('routecheck: ok')
} finally {
  await browser.close()
}
