import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const caps = (role) => ({
  httpRouteInstalled: true,
  virtualServiceInstalled: true,
  namespaceListAllowed: true,
  devMode: false,
  user: 'test-user',
  role,
})

async function open(role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'en-US' })
  await context.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/clusters') return route.fulfill({ json: [{ name: 'local' }, { name: 'edge-a' }] })
    if (path === '/api/capabilities') return route.fulfill({ json: caps(role) })
    return route.fulfill({ json: [] })
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto(`${base}/`)
  return { page, errors, context }
}

try {
  const { page, errors } = await open('admin')
  const trigger = page.getByRole('button', { name: 'Select cluster: local' })
  const menu = page.getByRole('menu', { name: 'Select cluster' })
  const local = page.getByRole('menuitemradio', { name: 'local' })
  const edge = page.getByRole('menuitemradio', { name: 'edge-a' })

  // Click opens a styled menu (not a native select) with the current cluster checked and focused.
  await trigger.click()
  await menu.waitFor()
  assert.equal(await page.locator('header select').count(), 0)
  assert.equal(await trigger.getAttribute('aria-expanded'), 'true')
  assert.equal(await local.getAttribute('aria-checked'), 'true')
  assert.equal(await edge.getAttribute('aria-checked'), 'false')
  assert.ok(await local.evaluate((el) => el === document.activeElement))
  await page.getByRole('menuitem', { name: 'Manage clusters…' }).waitFor()

  // Escape closes and returns focus; a click outside closes too.
  await page.keyboard.press('Escape')
  assert.equal(await menu.count(), 0)
  assert.ok(await trigger.evaluate((el) => el === document.activeElement))
  await trigger.click()
  await page.mouse.click(600, 500)
  assert.equal(await menu.count(), 0)

  // Keyboard: ArrowDown opens, arrows wrap, Enter switches cluster (stored, app reloads).
  await trigger.focus()
  await page.keyboard.press('ArrowDown')
  await menu.waitFor()
  await page.keyboard.press('ArrowDown')
  assert.ok(await edge.evaluate((el) => el === document.activeElement))
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  assert.ok(await local.evaluate((el) => el === document.activeElement), 'wraps past Manage back to the first item')
  await page.keyboard.press('End')
  await page.keyboard.press('ArrowUp')
  await Promise.all([page.waitForEvent('load'), page.keyboard.press('Enter')])
  assert.equal(await page.evaluate(() => localStorage.getItem('sealane-cluster')), 'edge-a')
  await page.getByRole('button', { name: 'Select cluster: edge-a' }).waitFor()

  // Manage clusters goes to Settings.
  await page.getByRole('button', { name: 'Select cluster: edge-a' }).click()
  await page.getByRole('menuitem', { name: 'Manage clusters…' }).click()
  await page.waitForURL(/\/settings$/)
  assert.equal(await menu.count(), 0)
  assert.deepEqual(errors, [])

  // Viewers get the clusters but no Manage entry.
  const viewer = await open('viewer')
  await viewer.page.getByRole('button', { name: 'Select cluster: local' }).click()
  await viewer.page.getByRole('menu', { name: 'Select cluster' }).waitFor()
  assert.equal(await viewer.page.getByRole('menuitem', { name: 'Manage clusters…' }).count(), 0)
  assert.equal(await viewer.page.getByRole('menuitemradio').count(), 2)
  assert.deepEqual(viewer.errors, [])
  console.log('cluster-menu: ok')
} finally {
  await browser.close()
}
