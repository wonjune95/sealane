import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { load } from 'js-yaml'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const TYPE = 'virtualservices.networking.istio.io'
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
    typeId: TYPE, group: 'networking.istio.io', version: 'v1', kind: 'VirtualService',
    category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
  },
]
const meta = (name) => ({ name, namespace: 'shop', labels: { app: 'reviews' }, resourceVersion: '7' })
// Every representable field, so an edit through the form must keep the rest intact.
const reviews = {
  apiVersion: 'networking.istio.io/v1',
  kind: 'VirtualService',
  metadata: meta('reviews'),
  spec: {
    exportTo: ['.'],
    hosts: ['reviews', 'reviews.example.com'],
    gateways: ['mesh', 'edge/public'],
    http: [
      {
        name: 'canary',
        match: [{ uri: { regex: '^/v2/.*' }, headers: { 'x-user': { exact: 'qa' }, 'x-env': { prefix: 'stg' } } }],
        route: [{ destination: { host: 'reviews', subset: 'v2', port: { number: 9080 } } }],
        timeout: '5s',
        retries: { attempts: 3, perTryTimeout: '2s' },
        rewrite: { uri: '/' },
      },
      { route: [{ destination: { host: 'reviews.shop.svc.cluster.local', subset: 'v1' }, weight: 100 }] },
    ],
  },
}
const faulty = {
  ...reviews,
  metadata: meta('faulty'),
  spec: { hosts: ['reviews'], http: [{ fault: { abort: { percentage: { value: 10 }, httpStatus: 503 } }, route: [{ destination: { host: 'reviews' } }] }] },
}
const detail = (raw) => ({
  typeId: TYPE, kind: 'VirtualService', namespace: 'shop', name: raw.metadata.name,
  summary: '', age: '1d', resourceVersion: '7', raw,
})
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': capabilities,
  '/api/resourceTypes': types,
  '/api/services': [{ namespace: 'shop', name: 'reviews', ports: [9080] }],
  '/api/gateways': [{ type: 'istio', namespace: 'edge', name: 'public' }, { type: 'gateway-api', namespace: 'edge', name: 'gw' }],
  '/api/subsets': [
    { namespace: 'shop', dr: 'reviews', host: 'reviews', name: 'v1' },
    { namespace: 'shop', dr: 'reviews', host: 'reviews', name: 'v2' },
    { namespace: 'shop', dr: 'ratings', host: 'ratings', name: 'v9' },
  ],
  [`/api/resources/${TYPE}/shop/reviews`]: detail(reviews),
  [`/api/resources/${TYPE}/shop/faulty`]: detail(faulty),
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const url = new URL(interception.request().url())
  const path = url.pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true, update: true, delete: true } })
  // Namespace-scoped lookups only answer for the fixture namespace.
  if (['/api/services', '/api/subsets'].includes(path) && url.searchParams.get('ns') !== 'shop') {
    return interception.fulfill({ json: [] })
  }
  return interception.fulfill({ json: fixtures[path] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))

const formTab = page.getByRole('button', { name: 'Form', exact: true })
const yamlTab = page.getByRole('button', { name: 'YAML', exact: true })
const isActive = async (tab) => (await tab.getAttribute('class')).includes('border-accent')
const section = (title) => page.locator('h3', { hasText: title }).locator('xpath=../..')
const editorYaml = async () => {
  await yamlTab.click()
  // CodeMirror renders only visible lines; read the whole document from the view
  // (the same DOM link EditorView.findFromDOM uses).
  return load(await page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString()))
}
const options = (id) => page.locator(`datalist#${id} option`).evaluateAll((os) => os.map((o) => o.value))

try {
  // (a) New VirtualService opens on the Form tab and builds a weighted canary.
  await page.goto(`${base}/resources/${TYPE}/new`)
  await section('Hosts').waitFor()
  assert.ok(await isActive(formTab), 'Form tab is active for a new VirtualService')
  await page.locator('label:has-text("Namespace") input').fill('shop')
  await section('Hosts').getByPlaceholder('reviews / example.com').fill('reviews')
  await section('Gateways').getByRole('button', { name: '+ Add' }).click()
  const gw = section('Gateways').locator('select')
  assert.deepEqual(await gw.locator('option').evaluateAll((os) => os.map((o) => o.value)), ['mesh', 'edge/public'])
  await gw.selectOption('edge/public')
  await page.getByRole('button', { name: '+ Add match' }).click()
  await page.getByLabel('URI', { exact: true }).fill('/api')
  await page.getByRole('button', { name: '+ Add route' }).click()

  // (c) Subsets are offered only once a route's host resolves to the DR host.
  const hosts = page.getByPlaceholder('host', { exact: true })
  await hosts.nth(0).fill('reviews')
  await page.locator('datalist#r0-d0-subsets option[value="v2"]').waitFor({ state: 'attached' })
  assert.deepEqual(await options('r0-d0-subsets'), ['v1', 'v2'])
  assert.deepEqual(await options('r0-d1-subsets'), [])
  await hosts.nth(1).fill('reviews.shop.svc.cluster.local')
  assert.deepEqual(await options('r0-d1-subsets'), ['v1', 'v2'])
  await hosts.nth(1).fill('reviews')

  const subsets = page.getByPlaceholder('subset', { exact: true })
  const weights = page.getByPlaceholder('weight', { exact: true })
  await subsets.nth(0).fill('v1')
  await weights.nth(0).fill('90')
  await subsets.nth(1).fill('v2')
  await weights.nth(1).fill('10')
  await page.getByText('weight total: 100').waitFor()

  const created = await editorYaml()
  assert.deepEqual(created.metadata, { name: 'my-virtualservice', namespace: 'shop' })
  assert.deepEqual(created.spec, {
    hosts: ['reviews'],
    gateways: ['edge/public'],
    http: [{
      match: [{ uri: { prefix: '/api' } }],
      route: [
        { destination: { host: 'reviews', subset: 'v1', port: { number: 80 } }, weight: 90 },
        { destination: { host: 'reviews', subset: 'v2' }, weight: 10 },
      ],
    }],
  })

  // (b) A VirtualService using fault injection locks the Form tab.
  await page.goto(`${base}/resources/${TYPE}/shop/faulty`)
  await page.getByText('Advanced fields detected — YAML only').waitFor()
  assert.ok(await formTab.isDisabled(), 'Form tab is locked for a fault-injection VirtualService')
  assert.ok(await isActive(yamlTab))

  // Lossless round trip: one form edit changes only that field.
  await page.goto(`${base}/resources/${TYPE}/shop/reviews`)
  await section('HTTP rules').waitFor()
  assert.ok(await isActive(formTab))
  assert.ok(await page.locator('label:has-text("Name") input').first().isDisabled(), 'identity is locked on edit')
  await page.locator('label:has-text("Timeout") input').first().fill('6s')
  const edited = await editorYaml()
  const want = structuredClone(reviews)
  want.spec.http[0].timeout = '6s'
  assert.deepEqual(edited, want)

  // Typing a new header name through an existing one (x-user → x-user-id) never
  // collapses the rows or overwrites x-user's value.
  await formTab.click()
  await page.getByRole('button', { name: '+ Add header' }).first().click()
  const headerNames = page.getByPlaceholder('header', { exact: true })
  await headerNames.nth(2).pressSequentially('x-user-id')
  await headerNames.nth(2).blur()
  assert.deepEqual((await editorYaml()).spec.http[0].match[0].headers, {
    'x-user': { exact: 'qa' },
    'x-env': { prefix: 'stg' },
    'x-user-id': { exact: '' },
  })

  // Renaming a header onto an existing name (x-env → x-user) is refused, not merged.
  await formTab.click()
  await headerNames.nth(1).fill('x-user')
  await headerNames.nth(1).blur()
  assert.equal(await headerNames.nth(1).inputValue(), 'x-env')
  assert.deepEqual((await editorYaml()).spec.http[0].match[0].headers, {
    'x-user': { exact: 'qa' },
    'x-env': { prefix: 'stg' },
    'x-user-id': { exact: '' },
  })

  // Mid-edit YAML where a list is briefly a map must not crash the editor; the
  // Form tab locks until the YAML is representable again.
  const setYaml = (text) =>
    page.locator('.cm-content').evaluate((el, t) => {
      const v = el.cmTile.view
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: t } })
    }, text)
  const valid = await page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString())
  await setYaml(valid.replace(/^  http:\n[\s\S]*$/m, '  http:\n    route: []\n'))
  await page.waitForTimeout(300)
  assert.deepEqual(errors, [])
  assert.ok(await formTab.isDisabled(), 'Form tab is locked while http is a map')
  await setYaml(valid)
  await page.waitForFunction(() => {
    const b = [...document.querySelectorAll('button')].find((x) => x.textContent.trim() === 'Form')
    return b && !b.disabled
  })
  await formTab.click()
  await section('HTTP rules').waitFor()

  assert.deepEqual(errors, [])
  console.log('vs-form: ok')
} finally {
  await browser.close()
}
