import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { load } from 'js-yaml'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const VS = 'virtualservices.networking.istio.io'
const DR = 'destinationrules.networking.istio.io'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const type = (typeId, kind) => ({
  typeId, group: 'networking.istio.io', version: 'v1', kind,
  category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
})
const mesh = {
  namespace: 'shop', name: 'reviews', ports: [9080], endpoints: 1,
  virtualServices: [], destinationRules: [], httpRoutes: [], gateways: [], policies: [], findings: [],
}

async function open(role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
  await context.route('**/api/**', async (interception) => {
    const path = new URL(interception.request().url()).pathname
    if (!path.startsWith('/api/')) return interception.continue()
    if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true, update: true, delete: true } })
    const fixtures = {
      '/api/clusters': [{ name: 'local' }],
      '/api/capabilities': {
        httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true,
        devMode: false, user: 'test-user', role,
      },
      '/api/resourceTypes': [type(VS, 'VirtualService'), type(DR, 'DestinationRule')],
      '/api/services': [{ namespace: 'shop', name: 'reviews', ports: [9080] }],
      '/api/services/shop/reviews/mesh': mesh,
    }
    return interception.fulfill({ json: fixtures[path] ?? [] })
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  return { page, errors }
}

const createVS = 'New VirtualService for this service'
const createDR = 'New DestinationRule for this service'
try {
  // A viewer sees the Service page without create buttons.
  {
    const { page, errors } = await open('viewer')
    await page.goto(`${base}/services/shop/reviews`)
    await page.getByRole('heading', { name: 'Virtual services' }).waitFor()
    assert.equal(await page.getByRole('link', { name: createVS }).count(), 0)
    assert.equal(await page.getByRole('link', { name: createDR }).count(), 0)
    assert.deepEqual(errors, [])
  }

  const { page, errors } = await open('editor')
  const formTab = page.getByRole('button', { name: 'Form', exact: true })
  const yamlTab = page.getByRole('button', { name: 'YAML', exact: true })
  const isActive = async (tab) => (await tab.getAttribute('class')).includes('border-accent')
  const field = (label) => page.locator(`label:has-text("${label}") input`).first()
  const editorYaml = async () => {
    await yamlTab.click()
    return load(await page.locator('.cm-content').evaluate((el) => el.cmTile.view.state.doc.toString()))
  }

  // An editor gets both buttons, pointing at the prefilled create URLs.
  await page.goto(`${base}/services/shop/reviews`)
  await page.getByRole('link', { name: createDR }).waitFor()
  assert.equal(await page.getByRole('link', { name: createDR }).getAttribute('href'), `/resources/${DR}/new?ns=shop&host=reviews`)
  await page.getByRole('link', { name: createVS }).click()

  // VirtualService: Form tab, identity and hosts seeded from the service.
  await page.getByPlaceholder('reviews / example.com').waitFor()
  assert.equal(new URL(page.url()).search, '?ns=shop&host=reviews')
  assert.ok(await isActive(formTab), 'Form tab is active for the seeded VirtualService')
  assert.equal(await field('Name').inputValue(), 'reviews')
  assert.equal(await field('Namespace').inputValue(), 'shop')
  assert.equal(await page.getByPlaceholder('reviews / example.com').inputValue(), 'reviews')
  assert.equal(await page.getByPlaceholder('host', { exact: true }).inputValue(), 'reviews')
  if (process.env.SEALANE_SCREENSHOT) await page.screenshot({ path: process.env.SEALANE_SCREENSHOT, fullPage: true })
  const vs = await editorYaml()
  assert.deepEqual(vs.metadata, { name: 'reviews', namespace: 'shop' })
  assert.deepEqual(vs.spec.hosts, ['reviews'])
  assert.equal(vs.spec.http[0].route[0].destination.host, 'reviews')

  // DestinationRule: Form tab, host seeded.
  await page.goto(`${base}/resources/${DR}/new?ns=shop&host=reviews`)
  await page.locator('label:has-text("Host") input').waitFor()
  assert.ok(await isActive(formTab), 'Form tab is active for the seeded DestinationRule')
  assert.equal(await field('Name').inputValue(), 'reviews')
  assert.equal(await field('Namespace').inputValue(), 'shop')
  assert.equal(await page.locator('label:has-text("Host") input').inputValue(), 'reviews')
  const dr = await editorYaml()
  assert.deepEqual(dr.metadata, { name: 'reviews', namespace: 'shop' })
  assert.equal(dr.spec.host, 'reviews')

  // Without both params the template is unchanged.
  await page.goto(`${base}/resources/${DR}/new?ns=shop`)
  await page.locator('label:has-text("Host") input').waitFor()
  assert.equal(await field('Name').inputValue(), 'my-destinationrule')
  assert.equal(await field('Namespace').inputValue(), 'default')

  assert.deepEqual(errors, [])
  console.log('service-create: ok')
} finally {
  await browser.close()
}
