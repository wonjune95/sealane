import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://localhost:5173'
const browser = await chromium.launch({ executablePath: process.env.SEALANE_CHROMIUM_PATH || undefined })
const ctx = await browser.newContext({ locale: 'en-US' })
const page = await ctx.newPage()
let ssoEnabled = true
await page.route('**/*', (route) => {
  const url = new URL(route.request().url())
  if (!url.pathname.startsWith('/api/')) return route.continue()
  if (url.pathname === '/api/auth/methods')
    return route.fulfill({ status: 200, json: ssoEnabled ? { local: true, oidc: { enabled: true, displayName: 'Keycloak' } } : { local: true } })
  return route.fulfill({ status: 401, json: { error: { reason: 'Unauthorized', message: 'login required' } } })
})
let failures = 0
const check = async (name, fn) => {
  try { await fn(); console.log('PASS', name) } catch (e) { failures++; console.log('FAIL', name, e.message) }
}

await check('SSO button shows the provider name and links to the login route', async () => {
  await page.goto(base + '/')
  const btn = page.getByRole('link', { name: 'Sign in with Keycloak', exact: true })
  await btn.waitFor()
  if ((await btn.getAttribute('href')) !== '/api/auth/oidc/login') throw new Error('wrong href ' + (await btn.getAttribute('href')))
  await page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
})
await check('oidc_error=access_denied shows the no-role message', async () => {
  await page.goto(base + '/?oidc_error=access_denied')
  await page.getByText('Your account has no role in Sealane', { exact: false }).waitFor()
  await page.waitForURL((u) => !u.search) // the code is dropped so a later sign-out does not show it again
})
await check('oidc_error=state shows the expired message', async () => {
  await page.goto(base + '/?oidc_error=state')
  await page.getByText('expired', { exact: false }).waitFor()
})
await check('no SSO button when the server offers only local accounts', async () => {
  ssoEnabled = false
  const methods = page.waitForResponse((r) => r.url().endsWith('/api/auth/methods'))
  await page.goto(base + '/')
  await methods // count only after the methods answer arrived, or the check passes vacuously
  await page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
  if (await page.getByRole('link', { name: /Sign in with/ }).count()) throw new Error('SSO link rendered without oidc')
})
await browser.close()
if (failures) { console.log(`${failures} failed`); process.exit(1) }
console.log('all login-sso checks passed')
