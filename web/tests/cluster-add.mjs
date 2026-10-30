import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH ? { executablePath: process.env.SEALANE_CHROMIUM_PATH } : {}),
})
const kubeconfig = `apiVersion: v1
kind: Config
clusters:
- {name: app, cluster: {server: "https://app:6443"}}
- {name: cicd, cluster: {server: "https://cicd:6443"}}
users:
- {name: a, user: {token: ta}}
- {name: c, user: {token: tc}}
contexts:
- {name: nks_app-cluster, context: {cluster: app, user: a}}
- {name: cicd, context: {cluster: cicd, user: c}}
current-context: cicd
`
const puts = []
const deletes = []
let listFails = 2 // the first list and react-query's one retry fail, to check the error and retry
let cleanupFails = true // the first account removal fails, to check the dialog stays
const context = await browser.newContext({ viewport: { width: 1280, height: 1000 }, locale: 'en-US' })
await context.route('**/api/**', async (route) => {
  const url = new URL(route.request().url())
  if (!url.pathname.startsWith('/api/')) return route.continue()
  if (url.pathname === '/api/capabilities')
    return route.fulfill({ json: { httpRouteInstalled: true, virtualServiceInstalled: true, namespaceListAllowed: true, user: 'admin', role: 'admin' } })
  if (url.pathname.startsWith('/api/clusters/') && route.request().method() === 'PUT') {
    puts.push({ path: url.pathname, body: route.request().postDataJSON() })
    const setup = route.request().postDataJSON().setup
    return route.fulfill({ json: { connected: true, version: 'v1.34.3', ...(setup ? { account: 'sealane-remote/sealane' } : {}) } })
  }
  if (url.pathname.startsWith('/api/clusters/') && route.request().method() === 'DELETE') {
    if (url.search === '?cleanup=true' && cleanupFails) {
      cleanupFails = false
      return route.fulfill({ status: 502, json: { error: { reason: 'CleanupFailed', message: 'cluster unreachable' } } })
    }
    deletes.push(url.pathname + url.search)
    return route.fulfill({ status: 204 })
  }
  if (url.pathname === '/api/clusters' && url.search === '?status=true' && listFails > 0) {
    listFails--
    return route.fulfill({ status: 500, json: { error: { reason: 'Internal', message: 'secret unreadable' } } })
  }
  if (url.pathname === '/api/clusters')
    return route.fulfill({
      json: [{ name: 'local', connected: true }, { name: 'own', dedicated: true, connected: true }, { name: 'pasted', connected: true }],
    })
  return route.fulfill({ json: [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
try {
  await page.goto(`${base}/settings`)
  const box = page.getByRole('textbox', { name: 'kubeconfig' })
  await box.waitFor()
  // A failed list says so, with a retry, instead of looking empty.
  const alert = page.getByRole('alert').filter({ hasText: 'Could not load the list: secret unreadable' })
  await alert.waitFor()
  await alert.getByRole('button', { name: 'Retry' }).click()
  await alert.waitFor({ state: 'detached' })
  const name = page.getByRole('textbox', { name: 'Name', exact: true })
  const contextSelect = page.getByRole('combobox', { name: 'Context' })
  const setup = page.getByRole('checkbox', { name: /Create a dedicated Sealane account/ })

  // A whole kubeconfig: the current context is preselected and names the cluster.
  await box.fill(kubeconfig)
  assert.equal(await contextSelect.inputValue(), 'cicd')
  assert.equal(await name.inputValue(), 'cicd')
  assert.ok(await setup.isChecked(), 'the dedicated account is the default')
  await page.getByText(/Sealane stores that account's token, not your credentials/).waitFor()

  // Picking another context renames until the name is typed by hand.
  await contextSelect.selectOption('nks_app-cluster')
  assert.equal(await name.inputValue(), 'nks_app-cluster')
  await page.getByRole('button', { name: 'Register cluster' }).click()
  await page.getByRole('status').filter({ hasText: 'with the account sealane-remote/sealane' }).waitFor()
  assert.equal(puts[0].path, '/api/clusters/nks_app-cluster')
  assert.deepEqual({ ...puts[0].body, kubeconfig: undefined }, { kubeconfig: undefined, context: 'nks_app-cluster', setup: true })
  assert.equal(puts[0].body.kubeconfig, kubeconfig)
  assert.equal(await box.inputValue(), '', 'the form clears after registering')

  // Loading a file fills the box; a typed name and an unchecked box go through as they are.
  await page.locator('input[type="file"]').setInputFiles({ name: 'config', mimeType: 'text/plain', buffer: Buffer.from(kubeconfig) })
  await page.waitForFunction(() => document.querySelector('textarea')?.value.includes('current-context: cicd'))
  await name.fill('prod')
  await contextSelect.selectOption('nks_app-cluster')
  assert.equal(await name.inputValue(), 'prod')
  await setup.uncheck()
  await page.getByText('The kubeconfig is stored as it is').waitFor()
  await page.getByRole('button', { name: 'Register cluster' }).click()
  await page.getByRole('status').filter({ hasText: 'Registered cluster prod (v1.34.3)' }).waitFor()
  assert.equal(puts[1].path, '/api/clusters/prod')
  assert.equal(puts[1].body.setup, false)
  assert.equal(puts[1].body.context, 'nks_app-cluster')

  // Deleting a cluster registered with Sealane's account removes that account by default.
  const row = (n) => page.getByRole('row').filter({ has: page.getByRole('cell', { name: n, exact: true }) })
  await row('own').getByRole('button', { name: 'Delete' }).click()
  const dialog = page.getByRole('dialog', { name: 'Delete cluster own?' })
  const cleanup = dialog.getByRole('checkbox', { name: /Also remove Sealane's account/ })
  assert.ok(await cleanup.isChecked())
  // The cluster could not be reached: the dialog stays open with the reason, nothing is deleted.
  await dialog.getByRole('button', { name: 'Delete' }).click()
  await dialog.getByText('cluster unreachable').waitFor()
  assert.equal(deletes.length, 0)
  await dialog.getByRole('button', { name: 'Delete' }).click()
  await page.getByRole('status').filter({ hasText: 'Deleted cluster own.' }).waitFor()
  assert.equal(deletes[0], '/api/clusters/own?cleanup=true')
  // A cluster registered with pasted credentials says they stay valid; nothing to clean up.
  await row('pasted').getByRole('button', { name: 'Delete' }).click()
  const plain = page.getByRole('dialog', { name: 'Delete cluster pasted?' })
  await plain.getByText(/stay valid on the cluster/).waitFor()
  assert.equal(await plain.getByRole('checkbox').count(), 0)
  await plain.getByRole('button', { name: 'Delete' }).click()
  await page.getByRole('status').filter({ hasText: 'Deleted cluster pasted.' }).waitFor()
  assert.equal(deletes[1], '/api/clusters/pasted')
  assert.deepEqual(errors, [])
  console.log('cluster-add: ok')
} finally {
  await browser.close()
}
