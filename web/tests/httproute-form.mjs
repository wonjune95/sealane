import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { load } from 'js-yaml'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const TYPE = 'httproutes.gateway.networking.k8s.io'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': {
    httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true,
    devMode: false, user: 'test-user', role: 'admin',
  },
  '/api/resourceTypes': [{
    typeId: TYPE, group: 'gateway.networking.k8s.io', version: 'v1', kind: 'HTTPRoute',
    category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
  }],
  '/api/gateways': [{ type: 'gateway-api', namespace: 'default', name: 'demo-gw' }],
  '/api/services': [{ namespace: 'default', name: 'my-svc', ports: [80] }],
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const path = new URL(interception.request().url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true } })
  return interception.fulfill({ json: fixtures[path] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/resources/${TYPE}/new`)
  const hostnames = page.locator('h3', { hasText: 'Hostnames' }).locator('xpath=../..')
  await hostnames.waitFor()
  const rows = hostnames.getByPlaceholder('example.com', { exact: true })
  assert.equal(await rows.count(), 1)
  // A new blank row survives the YAML round trip and can be filled in.
  await hostnames.getByRole('button', { name: '+ Add' }).click()
  assert.equal(await rows.count(), 2)
  await rows.nth(1).fill('shop.example.com')
  await page.getByRole('button', { name: 'YAML', exact: true }).click()
  const doc = load(await page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString()))
  assert.deepEqual(doc.spec.hostnames, ['example.com', 'shop.example.com'])
  assert.deepEqual(errors, [])
  console.log('httproute-form: ok')
} finally {
  await browser.close()
}
