// A session that the server ends (account changed, signed out elsewhere) returns the app to the
// sign-in page; a cluster that refuses Sealane's own credentials does not.
import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const capabilities = { httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true, user: 'alice', role: 'editor' }
const err = (status, code, message) => ({ status, json: { error: { reason: 'x', message, code } } })

async function run(listAnswer) {
  const context = await browser.newContext({ locale: 'en-US' })
  let sessionValid = true
  let capabilityCalls = 0
  await context.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/capabilities') {
      capabilityCalls++
      return route.fulfill(sessionValid ? { json: capabilities } : err(401, 'session-required', 'login required'))
    }
    if (path === '/api/clusters') return route.fulfill({ json: [{ name: 'local' }] })
    if (path === '/api/auth/methods') return route.fulfill({ json: { local: true } })
    if (path.startsWith('/api/resources/')) {
      const answer = listAnswer()
      if (answer.json?.error?.code === 'session-revoked') sessionValid = false
      return route.fulfill(answer)
    }
    return route.fulfill({ json: [] })
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto(`${base}/resources/virtualservices.networking.istio.io`)
  return { page, errors, calls: () => capabilityCalls, context }
}

try {
  // The account changed while the page was open: the next call ends the session and shows sign-in.
  const revoked = await run(() => err(401, 'session-revoked', 'your session ended'))
  await revoked.page.getByRole('button', { name: 'Sign in' }).waitFor({ timeout: 15000 })
  await revoked.page.waitForTimeout(1500)
  assert.ok(revoked.calls() <= 4, `capabilities re-checked ${revoked.calls()} times: no loop`)
  assert.deepEqual(revoked.errors, [])
  await revoked.context.close()

  // The cluster refused Sealane's credentials: an error, not the sign-in page.
  const upstream = await run(() => err(502, 'cluster-auth-failed', 'the cluster rejected the token'))
  await upstream.page.getByText(/rejected Sealane's credentials/).first().waitFor({ timeout: 15000 })
  assert.equal(await upstream.page.getByRole('button', { name: 'Sign in' }).count(), 0)
  assert.deepEqual(upstream.errors, [])
  await upstream.context.close()
  console.log('session-ended: ok')
} finally {
  await browser.close()
}
