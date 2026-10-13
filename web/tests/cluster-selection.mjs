import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH
    ? { executablePath: process.env.SEALANE_CHROMIUM_PATH }
    : {}),
})
const key = 'sealane-cluster'
const capabilities = {
  httpRouteInstalled: true,
  virtualServiceInstalled: true,
  istiodVersion: '1.30.2',
  istioApiVersion: 'v1',
  gatewayAPIVersion: 'v1',
  namespaceListAllowed: true,
  devMode: false,
  user: 'test-user',
  role: 'admin',
}

async function scenario({ registered = false, authenticated = true, failCapabilities = false, failList = false } = {}) {
  const context = await browser.newContext({ locale: 'en-US' })
  const requests = []
  const errors = []
  await context.addInitScript(({ key }) => {
    if (!sessionStorage.getItem('selection-test-seeded')) {
      localStorage.setItem(key, 'app-cluster')
      sessionStorage.setItem('selection-test-seeded', '1')
    }
  }, { key })
  await context.route('**/api/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return route.continue()
    requests.push({ path: url.pathname, cluster: url.searchParams.get('cluster'), method: request.method() })
    if (url.pathname === '/api/login') {
      authenticated = true
      return route.fulfill({ json: { ok: true } })
    }
    if (!authenticated) {
      return route.fulfill({ status: 401, json: { error: { reason: 'Unauthorized', message: 'login required' } } })
    }
    if (url.pathname === '/api/clusters') {
      if (failList) return route.fulfill({ status: 500, json: { error: { reason: 'Internal', message: 'list failed' } } })
      return route.fulfill({ json: registered ? [{ name: 'local' }, { name: 'app-cluster' }] : [{ name: 'local' }] })
    }
    if (url.pathname === '/api/capabilities') {
      if (failCapabilities && url.searchParams.get('cluster')) {
        return route.fulfill({ status: 500, json: { error: { reason: 'Internal', message: 'cannot reach the remote API server' } } })
      }
      return route.fulfill({ json: capabilities })
    }
    assert.equal(request.method(), 'GET', `Unexpected mutation: ${request.method()} ${url.pathname}`)
    return route.fulfill({ json: [] })
  })
  const page = await context.newPage()
  page.on('pageerror', (error) => errors.push(error.message))
  const waitFor = (predicate) => page.waitForFunction(predicate, null, { timeout: 15000 })
  const finish = async () => {
    assert.deepEqual(errors, [])
    assert.ok(requests.filter((r) => r.path === '/api/clusters').every((r) => r.cluster === null))
    await context.close()
  }
  return { page, requests, waitFor, finish }
}

try {
  // A removed selection is cleared before capabilities run, including deep links.
  {
    const s = await scenario()
    await s.page.goto(`${base}/resources/virtualservices.networking.istio.io/old-namespace/old-name`)
    await s.page.getByRole('heading', { name: 'Cluster overview', exact: true }).waitFor()
    assert.equal(new URL(s.page.url()).pathname, '/')
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), null)
    assert.ok(s.requests.filter((r) => r.path === '/api/capabilities').every((r) => r.cluster === null))
    await s.finish()
    console.log('PASS: removed selection recovers to the local overview before querying capabilities')
  }
  // An expired session can log in even with a removed selection.
  {
    const s = await scenario({ authenticated: false })
    await s.page.goto(base)
    await s.page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), 'app-cluster')
    await s.page.getByRole('textbox', { name: 'Username', exact: true }).fill('test-user')
    await s.page.getByLabel('Password', { exact: true }).fill('test-password')
    await s.page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await s.page.getByRole('heading', { name: 'Cluster overview', exact: true }).waitFor()
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), null)
    assert.ok(s.requests.filter((r) => r.path === '/api/login').every((r) => r.cluster === null))
    await s.finish()
    console.log('PASS: expired session logs in and recovers without a cluster parameter on login')
  }
  // Healthy registered selections continue to target the selected cluster.
  {
    const s = await scenario({ registered: true })
    await s.page.goto(base)
    await s.page.getByRole('heading', { name: 'Cluster overview', exact: true }).waitFor()
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), 'app-cluster')
    assert.ok(s.requests.filter((r) => r.path === '/api/capabilities').every((r) => r.cluster === 'app-cluster'))
    await s.finish()
    console.log('PASS: a registered cluster remains selected')
  }
  // Connection failures retain the selection and offer an explicit way back.
  {
    const s = await scenario({ registered: true, failCapabilities: true })
    await s.page.goto(`${base}/overview`)
    const back = s.page.getByRole('button', { name: 'Back to local cluster' })
    await back.waitFor()
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), 'app-cluster')
    await back.click()
    await s.page.getByRole('heading', { name: 'Cluster overview', exact: true }).waitFor()
    assert.equal(new URL(s.page.url()).pathname, '/')
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), null)
    await s.finish()
    console.log('PASS: an unreachable cluster is preserved until the user returns to local')
  }
  // An unavailable list is not proof that the saved cluster was removed.
  {
    const s = await scenario({ registered: true, failList: true })
    await s.page.goto(base)
    await s.page.getByText('Could not load clusters: list failed', { exact: true }).waitFor()
    assert.equal(await s.page.evaluate((key) => localStorage.getItem(key), key), 'app-cluster')
    assert.equal(s.requests.filter((r) => r.path === '/api/capabilities').length, 0)
    await s.finish()
    console.log('PASS: list errors retain the selection and do not query the remote cluster')
  }
} finally {
  await browser.close()
}
