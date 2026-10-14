import assert from 'node:assert/strict'
import { chromium } from 'playwright'

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
  role: 'viewer',
}
const ages = ['3d22h', '5h', '12m']
// Served order is descending by name so ascending sort visibly changes the first row.
const items = Array.from({ length: 120 }, (_, i) => {
  const n = 119 - i
  return {
    typeId: TYPE,
    kind: 'VirtualService',
    namespace: 'apps',
    name: `vs-${String(n).padStart(3, '0')}`,
    summary: '',
    age: ages[n % 3],
  }
})
const types = [
  { typeId: TYPE, kind: 'VirtualService', category: 'traffic', namespaced: true, installed: true, dangerous: false },
]
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const path = new URL(interception.request().url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path === '/api/clusters') return interception.fulfill({ json: [{ name: 'local' }] })
  if (path === '/api/capabilities') return interception.fulfill({ json: capabilities })
  if (path === '/api/resourceTypes') return interception.fulfill({ json: types })
  if (path === `/api/resources/${TYPE}`) return interception.fulfill({ json: items })
  return interception.fulfill({ json: [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/resources/${TYPE}`)
  const rows = page.locator('tbody tr')
  const first = (col) => rows.first().locator('td').nth(col).innerText()
  await rows.first().waitFor()
  assert.equal(await rows.count(), 50)
  await page.getByRole('button', { name: /Show more \(50 of 120\)/ }).click()
  assert.equal(await rows.count(), 100)
  await page.getByRole('button', { name: /Show more \(100 of 120\)/ }).click()
  assert.equal(await rows.count(), 120)
  assert.equal(await page.getByRole('button', { name: /Show more/ }).count(), 0)

  // Sorting resets the window; Name asc then desc.
  const name = page.getByRole('button', { name: 'Sort by Name', exact: true })
  assert.equal(await first(2), 'vs-119')
  await name.click()
  assert.equal(await rows.count(), 50)
  assert.equal(await first(2), 'vs-000')
  assert.equal(await page.locator('th[aria-sort="ascending"]').count(), 1)
  await name.click()
  assert.equal(await first(2), 'vs-119')
  assert.equal(await page.locator('th[aria-sort="descending"]').count(), 1)

  // Age sorts by seconds, not lexically: 12m < 5h < 3d22h.
  const age = page.getByRole('button', { name: 'Sort by Age', exact: true })
  await age.click()
  assert.equal(await first(4), '12m')
  await age.click()
  assert.equal(await first(4), '3d22h')

  // Search resets the window.
  await page.getByRole('button', { name: /Show more/ }).click()
  assert.equal(await rows.count(), 100)
  await page.getByRole('textbox', { name: 'Search resources' }).fill('vs-')
  assert.equal(await rows.count(), 50)
  assert.deepEqual(errors, [])
  console.log('ok')
} finally {
  await browser.close()
}
