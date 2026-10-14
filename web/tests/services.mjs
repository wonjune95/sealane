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
// Zero counts are omitted by the API, exactly as the server serialises them.
const services = [
  { namespace: 'shop', name: 'details', ports: [9080] },
  { namespace: 'shop', name: 'ratings', ports: [9080], policies: 2 },
  { namespace: 'shop', name: 'reviews', ports: [9080], routes: 3, destinationRules: 1, policies: 1, findings: 2 },
]
const servicesQueries = []
const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const url = new URL(interception.request().url())
  if (!url.pathname.startsWith('/api/')) return interception.continue()
  if (url.pathname === '/api/clusters') return interception.fulfill({ json: [{ name: 'local' }] })
  if (url.pathname === '/api/capabilities') return interception.fulfill({ json: capabilities })
  if (url.pathname === '/api/namespaces') return interception.fulfill({ json: ['shop'] })
  if (url.pathname === '/api/services') {
    servicesQueries.push(url.search)
    return interception.fulfill({ json: services })
  }
  return interception.fulfill({ json: [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/services`)
  const rows = page.locator('tbody tr')
  await rows.first().waitFor()
  assert.ok(servicesQueries.some((q) => new URLSearchParams(q).get('mesh') === 'true'), `mesh=true not sent: ${servicesQueries}`)

  for (const col of ['Routes', 'Rules', 'Policies', 'Findings']) {
    await page.getByRole('button', { name: `Sort by ${col}`, exact: true }).waitFor()
  }
  const cells = (row) => rows.nth(row).locator('td').allInnerTexts()
  // Namespace, Name, Ports, Routes, Rules, Policies, Findings — zero renders as a dash.
  assert.deepEqual(await cells(0), ['shop', 'details', '9080', '—', '—', '—', '—'])
  assert.deepEqual(await cells(2), ['shop', 'reviews', '9080', '3', '1', '1', '2'])
  const pill = rows.nth(2).locator('td').nth(6).locator('span')
  assert.equal(await pill.innerText(), '2')
  assert.equal(await pill.getAttribute('title'), '2 findings for this service')
  assert.match(await pill.getAttribute('class'), /red/)
  if (process.env.SEALANE_SCREENSHOT) await page.screenshot({ path: process.env.SEALANE_SCREENSHOT })

  // Findings descending puts the service with findings first.
  const findings = page.getByRole('button', { name: 'Sort by Findings', exact: true })
  await findings.click()
  await findings.click()
  assert.equal(await page.locator('th[aria-sort="descending"]').count(), 1)
  assert.equal((await cells(0))[1], 'reviews')
  // Mesh-only keeps services with any count; the pill shows shown of total.
  const header = page.locator('h1')
  await page.getByLabel('Only services with mesh config').check()
  assert.equal(await rows.count(), 2)
  assert.deepEqual(await rows.locator('td:nth-child(2)').allInnerTexts(), ['reviews', 'ratings'])
  assert.match(await header.innerText(), /2 of 3/)
  await page.getByLabel('Only services with mesh config').uncheck()
  assert.equal(await rows.count(), 3)
  assert.doesNotMatch(await header.innerText(), /of 3/)
  assert.deepEqual(errors, [])
  console.log('ok')
} finally {
  await browser.close()
}
