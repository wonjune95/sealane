import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { load } from 'js-yaml'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const TYPE = 'destinationrules.networking.istio.io'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const types = [{
  typeId: TYPE, group: 'networking.istio.io', version: 'v1', kind: 'DestinationRule',
  category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
}]
const meta = (name) => ({ name, namespace: 'shop', labels: { app: 'reviews' }, resourceVersion: '7' })
// Every representable field (loadBalancer before tls on purpose), so an edit
// through the form must keep the rest intact.
const reviews = {
  apiVersion: 'networking.istio.io/v1',
  kind: 'DestinationRule',
  metadata: meta('reviews'),
  spec: {
    exportTo: ['.'],
    host: 'reviews.shop.svc.cluster.local',
    trafficPolicy: { loadBalancer: { simple: 'LEAST_REQUEST' }, tls: { mode: 'ISTIO_MUTUAL' } },
    subsets: [{ name: 'v1', labels: { app: 'reviews', version: 'v1' } }],
  },
}
const pooled = {
  ...reviews,
  metadata: meta('pooled'),
  spec: { host: 'reviews', trafficPolicy: { connectionPool: { tcp: { maxConnections: 100 } } } },
}
const detail = (raw) => ({
  typeId: TYPE, kind: 'DestinationRule', namespace: 'shop', name: raw.metadata.name,
  summary: '', age: '1d', resourceVersion: '7', raw,
})
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': {
    httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true,
    devMode: false, user: 'test-user', role: 'admin',
  },
  '/api/resourceTypes': types,
  '/api/services': [{ namespace: 'shop', name: 'reviews', ports: [9080] }],
  [`/api/resources/${TYPE}/shop/reviews`]: detail(reviews),
  [`/api/resources/${TYPE}/shop/pooled`]: detail(pooled),
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const url = new URL(interception.request().url())
  const path = url.pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true, update: true, delete: true } })
  if (path === '/api/services' && url.searchParams.get('ns') !== 'shop') return interception.fulfill({ json: [] })
  return interception.fulfill({ json: fixtures[path] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))

const formTab = page.getByRole('button', { name: 'Form', exact: true })
const yamlTab = page.getByRole('button', { name: 'YAML', exact: true })
const isActive = async (tab) => (await tab.getAttribute('class')).includes('border-accent')
const section = (title) => page.locator('h3', { hasText: title }).locator('xpath=../..')
const editorText = async () => {
  await yamlTab.click()
  return page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString())
}
const select = (label) => page.locator(`label:has-text("${label}") select`)
const host = page.locator('label:has-text("Host") input')
const missing = page.getByText('⚠ missing')

try {
  // New DestinationRule opens on the Form tab with the seeded v1 subset.
  await page.goto(`${base}/resources/${TYPE}/new`)
  await section('Subsets').waitFor()
  assert.ok(await isActive(formTab), 'Form tab is active for a new DestinationRule')
  assert.equal(await page.getByPlaceholder('v1', { exact: true }).inputValue(), 'v1')
  await page.locator('label:has-text("Namespace") input').fill('shop')
  await host.fill('ratings')
  await missing.waitFor()
  await host.fill('reviews')
  await missing.waitFor({ state: 'detached' })

  await section('Subsets').getByRole('button', { name: '+ Add', exact: true }).click()
  await page.getByPlaceholder('v1', { exact: true }).nth(1).fill('v2')
  await page.getByRole('button', { name: '+ Add label' }).nth(1).click()
  const keys = page.getByPlaceholder('label', { exact: true })
  await keys.nth(1).fill('version')
  await keys.nth(1).blur()
  await page.getByPlaceholder('value', { exact: true }).nth(1).fill('v2')
  await select('TLS mode').selectOption('ISTIO_MUTUAL')

  const createdText = await editorText()
  assert.match(createdText, /mode: ISTIO_MUTUAL/)
  const created = load(createdText)
  assert.deepEqual(created.metadata, { name: 'my-destinationrule', namespace: 'shop' })
  assert.deepEqual(created.spec, {
    host: 'reviews',
    subsets: [{ name: 'v1', labels: { version: 'v1' } }, { name: 'v2', labels: { version: 'v2' } }],
    trafficPolicy: { tls: { mode: 'ISTIO_MUTUAL' } },
  })

  // connectionPool is outside the form's subset: the Form tab locks.
  await page.goto(`${base}/resources/${TYPE}/shop/pooled`)
  await page.getByText('Advanced fields detected — YAML only').waitFor()
  assert.ok(await formTab.isDisabled(), 'Form tab is locked for a connectionPool DestinationRule')
  assert.ok(await isActive(yamlTab))

  // Lossless round trip: one select changes only that field, in place.
  await page.goto(`${base}/resources/${TYPE}/shop/reviews`)
  await section('Subsets').waitFor()
  assert.ok(await isActive(formTab))
  assert.ok(await page.locator('label:has-text("Name") input').first().isDisabled(), 'identity is locked on edit')
  await select('Load balancer').selectOption('RANDOM')
  const edited = load(await editorText())
  const want = structuredClone(reviews)
  want.spec.trafficPolicy.loadBalancer.simple = 'RANDOM'
  assert.deepEqual(edited, want)
  assert.deepEqual(Object.keys(edited.spec.trafficPolicy), ['loadBalancer', 'tls'], 'key order kept')

  // Typing a label key through an existing one (app → app-tier) never collapses rows.
  await formTab.click()
  await page.getByRole('button', { name: '+ Add label' }).click()
  await keys.nth(2).pressSequentially('app-tier')
  await keys.nth(2).blur()
  assert.deepEqual(load(await editorText()).spec.subsets[0].labels, { app: 'reviews', version: 'v1', 'app-tier': '' })

  // Unset selects drop tls, then the whole trafficPolicy.
  await formTab.click()
  await select('TLS mode').selectOption('')
  assert.deepEqual(load(await editorText()).spec.trafficPolicy, { loadBalancer: { simple: 'RANDOM' } })
  await formTab.click()
  await select('Load balancer').selectOption('')
  const cleared = load(await editorText())
  assert.equal(cleared.spec.trafficPolicy, undefined)
  assert.deepEqual(cleared.spec.exportTo, ['.'])
  assert.deepEqual(cleared.metadata, reviews.metadata)

  assert.deepEqual(errors, [])
  console.log('dr-form: ok')
} finally {
  await browser.close()
}
