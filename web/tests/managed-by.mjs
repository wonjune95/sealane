import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const TYPE = 'httproutes.gateway.networking.k8s.io'
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
}
const types = [
  {
    typeId: TYPE, group: 'gateway.networking.k8s.io', version: 'v1', kind: 'HTTPRoute',
    category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
  },
]
const row = (name, managedBy) => ({
  typeId: TYPE, kind: 'HTTPRoute', namespace: 'shop', name, summary: '', age: '1d', resourceVersion: '7', managedBy,
})
const items = [
  row('by-argo', { tool: 'argocd', name: 'shop-prod' }),
  row('by-helm', { tool: 'helm', name: 'shop' }),
  row('by-hand'),
]
const raw = (name) => ({
  apiVersion: 'gateway.networking.k8s.io/v1',
  kind: 'HTTPRoute',
  metadata: { name, namespace: 'shop', resourceVersion: '7' },
  spec: { parentRefs: [{ name: 'gw' }], rules: [{ backendRefs: [{ name: 'shop', port: 80 }] }] },
})
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': capabilities,
  '/api/resourceTypes': types,
  [`/api/resources/${TYPE}`]: items,
  ...Object.fromEntries(items.map((r) => [`/api/resources/${TYPE}/shop/${r.name}`, { ...r, raw: raw(r.name) }])),
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const path = new URL(interception.request().url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true, update: true, delete: true } })
  return interception.fulfill({ json: fixtures[path] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
const note = page.getByRole('note')

try {
  // List: a badge naming the tool on managed rows only, with the warning as its tooltip.
  await page.goto(`${base}/resources/${TYPE}`)
  const rowOf = (name) => page.locator('tbody tr', { has: page.getByRole('link', { name, exact: true }) })
  await rowOf('by-argo').waitFor()
  const argo = rowOf('by-argo').getByText('Argo CD', { exact: true })
  assert.match(await argo.getAttribute('title'), /^Managed by Argo CD \(shop-prod\)\. .*Change the source in Git instead\.$/)
  assert.match(await rowOf('by-helm').getByText('Helm', { exact: true }).getAttribute('title'), /next helm upgrade overwrites/)
  assert.equal(await rowOf('by-hand').locator('span[title]').count(), 0)

  // Search matches the tool name.
  await page.getByRole('textbox', { name: 'Search resources' }).fill('argo cd')
  assert.equal(await page.locator('tbody tr').count(), 1)

  // Editor: the warning stays on screen while editing a managed resource.
  await page.goto(`${base}/resources/${TYPE}/shop/by-argo`)
  await note.waitFor()
  assert.match(await note.innerText(), /Argo CD reverts them on its next sync/)
  await page.goto(`${base}/resources/${TYPE}/shop/by-helm`)
  await note.waitFor()
  assert.match(await note.innerText(), /^Managed by Helm release shop\./)
  await page.goto(`${base}/resources/${TYPE}/shop/by-hand`)
  await page.getByRole('tab', { name: 'YAML', exact: true }).waitFor()
  assert.equal(await note.count(), 0)

  assert.deepEqual(errors, [])
  console.log('managed-by: ok')
} finally {
  await browser.close()
}
