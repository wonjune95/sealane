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
  role: 'viewer',
}
const analysis = {
  checked: 12,
  skipped: ['pods'],
  findings: [
    {
      severity: 'error',
      code: 'vs-host-missing',
      message: 'VirtualService host shop.example.com is not served by any gateway',
      resource: { typeId: 'virtualservices.networking.istio.io', kind: 'VirtualService', namespace: 'apps', name: 'shop-vs' },
    },
    {
      severity: 'warning',
      code: 'backend-no-endpoints',
      message: 'Backend checkout has no ready endpoints',
      resource: { typeId: 'virtualservices.networking.istio.io', kind: 'VirtualService', namespace: 'apps', name: 'checkout-vs' },
      related: [{ typeId: 'services', kind: 'Service', namespace: 'apps', name: 'checkout' }],
    },
  ],
}
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const path = new URL(interception.request().url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path === '/api/clusters') return interception.fulfill({ json: [{ name: 'local' }] })
  if (path === '/api/capabilities') return interception.fulfill({ json: capabilities })
  if (path === '/api/analyze') return interception.fulfill({ json: analysis })
  return interception.fulfill({ json: [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/analysis`)
  await page.getByRole('heading', { name: 'Mesh analysis' }).waitFor()
  const rows = page.locator('tbody tr')
  await rows.first().waitFor()
  assert.equal(await rows.count(), 2)
  await page.getByText('Pod checks were skipped', { exact: false }).waitFor()
  const link = page.getByRole('link', { name: /VirtualService shop-vs/ })
  assert.ok(
    (await link.getAttribute('href')).endsWith('/resources/virtualservices.networking.istio.io/apps/shop-vs'),
  )
  await page.getByRole('button', { name: 'Errors', exact: true }).click()
  assert.equal(await rows.count(), 1)
  await page.getByRole('button', { name: 'All', exact: true }).click()
  await page.getByRole('textbox', { name: 'Search findings' }).fill('no-endpoints')
  assert.equal(await rows.count(), 1)
  await page.getByRole('textbox', { name: 'Search findings' }).fill('zzz')
  await page.getByText('No findings', { exact: true }).waitFor()
  assert.deepEqual(errors, [])
  console.log('ok')
} finally {
  await browser.close()
}
