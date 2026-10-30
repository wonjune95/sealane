import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { load } from 'js-yaml'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const TYPE = 'virtualservices.networking.istio.io'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const types = [
  {
    typeId: TYPE, group: 'networking.istio.io', version: 'v1', kind: 'VirtualService',
    category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
  },
]
const vs = (name, hosts, rv) => ({
  apiVersion: 'networking.istio.io/v1',
  kind: 'VirtualService',
  metadata: { name, namespace: 'shop', ...(rv ? { resourceVersion: rv } : {}) },
  spec: { hosts, http: [{ route: [{ destination: { host: 'reviews' } }] }] },
})
const entry = (id, verb, name, extra = {}) => ({
  id, verb, typeId: TYPE, namespace: 'shop', name, ok: true, user: 'alice', provider: 'local',
  time: '2026-10-30T01:00:00Z', ...extra,
})
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/resourceTypes': types,
  [`/api/resources/${TYPE}/shop/reviews`]: {
    typeId: TYPE, kind: 'VirtualService', namespace: 'shop', name: 'reviews', summary: '', age: '1d',
    resourceVersion: '9', raw: vs('reviews', ['reviews'], '9'),
  },
  '/api/history': [entry('3', 'update', 'reviews'), entry('1', 'create', 'reviews', { user: 'bob' }), entry('2', 'update', 'reviews', { ok: false })],
  '/api/history/1': { ...entry('1', 'create', 'reviews'), before: null, after: vs('reviews', ['reviews', 'old.example.com']) },
  '/api/history/3': { ...entry('3', 'update', 'reviews'), before: vs('reviews', ['x']), after: null },
  '/api/audit': [entry('5', 'delete', 'gone'), entry('6', 'delete', 'elsewhere', { cluster: 'edge-a' }), entry('7', 'update', 'reviews')],
  '/api/history/5': { ...entry('5', 'delete', 'gone'), before: vs('gone', ['gone.example.com']), after: null },
}

async function open(role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
  const write = role !== 'viewer'
  await context.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (!url.pathname.startsWith('/api/')) return route.continue()
    if (url.pathname === '/api/capabilities')
      return route.fulfill({ json: { httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true, user: 'u', role } })
    if (url.pathname.startsWith('/api/access/')) return route.fulfill({ json: { create: write, update: write, delete: write } })
    if (url.pathname === '/api/history') {
      assert.equal(url.searchParams.get('type'), TYPE)
      assert.equal(url.searchParams.get('ns'), 'shop')
      assert.equal(url.searchParams.get('name'), 'reviews')
    }
    return route.fulfill({ json: fixtures[url.pathname] ?? [] })
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  return { page, errors }
}
const editorYaml = (page) =>
  page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString()).then((s) => load(s))

try {
  // Editor: History tab lists the resource's changes; failed ones have no Compare.
  const { page, errors } = await open('editor')
  await page.goto(`${base}/resources/${TYPE}/shop/reviews`)
  await page.getByRole('button', { name: 'History', exact: true }).click()
  const rows = page.locator('li', { has: page.locator('time') })
  await rows.first().waitFor()
  assert.equal(await rows.count(), 3)
  assert.match(await rows.nth(1).innerText(), /Created[\s\S]*bob/)
  assert.equal(await rows.nth(2).getByRole('button', { name: 'Compare with current' }).count(), 0)

  // Compare shows current (red) against the stored version (green); loading fills the editor.
  await rows.nth(1).getByRole('button', { name: 'Compare with current' }).click()
  const dialog = page.getByRole('dialog', { name: /Version from/ })
  await dialog.waitFor()
  assert.match(await dialog.innerText(), /\+\s+- old\.example\.com/)
  await dialog.getByRole('button', { name: 'Load into editor' }).click()
  await page.getByRole('status').filter({ hasText: 'Loaded the version from' }).waitFor()
  const loaded = await editorYaml(page)
  assert.deepEqual(loaded.spec.hosts, ['reviews', 'old.example.com'])
  assert.equal(loaded.metadata.resourceVersion, '9', 'carries the current resourceVersion so Apply is a normal update')

  // A revision whose objects were pruned says so.
  await page.getByRole('button', { name: 'History', exact: true }).click()
  await rows.first().getByRole('button', { name: 'Compare with current' }).click()
  await page.getByRole('alert').filter({ hasText: 'no longer kept' }).waitFor()

  // Home: a deletion in this cluster offers Restore, which opens the new-resource editor with it.
  await page.goto(`${base}/`)
  const restore = page.getByRole('link', { name: 'Restore' })
  await restore.first().waitFor()
  assert.equal(await restore.count(), 1, 'only the local deletion, not the edge-a one or the update')
  await restore.click()
  await page.waitForURL(/\/new\?restore=5$/)
  await page.getByRole('status').filter({ hasText: 'Loaded the resource deleted at' }).waitFor()
  await page.getByRole('button', { name: 'YAML', exact: true }).click()
  const restored = await editorYaml(page)
  assert.equal(restored.metadata.name, 'gone')
  assert.deepEqual(restored.spec.hosts, ['gone.example.com'])
  assert.equal(restored.metadata.resourceVersion, undefined)
  assert.deepEqual(errors, [])

  // Viewers compare but cannot load, and get no Restore on the home page.
  const viewer = await open('viewer')
  await viewer.page.goto(`${base}/resources/${TYPE}/shop/reviews`)
  await viewer.page.getByRole('button', { name: 'History', exact: true }).click()
  await viewer.page.getByRole('button', { name: 'Compare with current' }).nth(1).click()
  const vDialog = viewer.page.getByRole('dialog', { name: /Version from/ })
  await vDialog.waitFor()
  assert.equal(await vDialog.getByRole('button', { name: 'Load into editor' }).count(), 0)
  await vDialog.getByRole('button', { name: 'Close preview' }).first().click()
  await viewer.page.goto(`${base}/`)
  await viewer.page.getByText('shop/gone', { exact: true }).waitFor()
  assert.equal(await viewer.page.getByRole('link', { name: 'Restore' }).count(), 0)
  assert.deepEqual(viewer.errors, [])
  console.log('history: ok')
} finally {
  await browser.close()
}
