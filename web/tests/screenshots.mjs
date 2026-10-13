// Captures the README screenshots against a running Sealane (light theme, English, 1440x900).
// Usage: SEALANE_URL=http://host SEALANE_USER=u SEALANE_PASS=p node tests/screenshots.mjs
import { chromium } from 'playwright'

const url = process.env.SEALANE_URL ?? 'http://localhost:8080'
const user = process.env.SEALANE_USER ?? 'admin'
const pass = process.env.SEALANE_PASS ?? 'admin'
const out = process.env.SEALANE_SHOTS ?? '../docs/img'

const browser = await chromium.launch({ executablePath: process.env.SEALANE_CHROMIUM_PATH || undefined })
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: 'en-US' })
await page.addInitScript(() => {
  localStorage.setItem('sealane-locale', 'en')
  localStorage.setItem('sealane-theme', 'light')
})
const shot = (name) => page.screenshot({ path: `${out}/${name}.png` })

await page.goto(url + '/')
await page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
await shot('screenshot-login')
await page.getByRole('textbox', { name: 'Username', exact: true }).fill(user)
await page.getByLabel('Password', { exact: true }).fill(pass)
await page.getByRole('button', { name: 'Sign in', exact: true }).click()
await page.getByRole('heading', { name: 'Cluster overview', exact: true }).waitFor()
await page.waitForTimeout(1500)
await shot('screenshot')

await page.goto(url + '/flowmap')
await page.getByRole('heading', { name: 'Traffic flow', exact: true }).waitFor()
await page.waitForTimeout(2500)
// show only the demo mesh so the shot never leaks other namespaces or hostnames
await page.getByRole('combobox', { name: 'Flow namespace', exact: true }).selectOption('sealane-demo')
await page.waitForTimeout(1500)
await shot('screenshot-flowmap')

await page.goto(url + '/routecheck')
await page.getByRole('textbox', { name: 'Request URL', exact: true }).fill('http://echo.sealane-demo.svc.cluster.local:8080/')
await page.getByRole('button', { name: 'Check route', exact: true }).click()
await page.getByRole('button', { name: 'Route analysis', exact: true }).waitFor()
await page.waitForTimeout(1500)
await shot('screenshot-routecheck')

await page.getByRole('combobox', { name: 'Source namespace', exact: true }).selectOption('sealane-demo')
const podSelect = page.getByRole('combobox', { name: 'Source pod', exact: true })
await podSelect.locator('option').nth(1).waitFor({ state: 'attached' })
await podSelect.selectOption({ index: 1 })
await page.getByRole('button', { name: 'Send request', exact: true }).click()
await page.getByRole('button', { name: 'Response body', exact: true }).waitFor()
await page.waitForTimeout(1500)
await shot('screenshot-requesttest')

await page.goto(url + '/resources/virtualservices.networking.istio.io')
await page.getByRole('table').waitFor()
// open the demo mesh's VirtualService (other namespaces may hold unrelated leftovers)
await page.getByRole('row').filter({ hasText: 'sealane-demo' }).getByRole('link').first().click()
await page.waitForTimeout(2000)
await shot('screenshot-form')

await page.goto(url + '/settings')
await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor()
await page.waitForTimeout(1000)
await shot('screenshot-settings')

await browser.close()
console.log('screenshots written to', out)
