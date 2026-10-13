// Real OIDC round trip against a running Sealane + the Dex demo (examples/dex-demo.yaml).
// Not a fixture test: SEALANE_URL must point at an instance with oidc.enabled=true.
import { chromium } from 'playwright'

const base = process.env.SEALANE_URL ?? 'http://localhost:8080'
const provider = process.env.SEALANE_SSO_NAME ?? 'Dex'
const browser = await chromium.launch({ executablePath: process.env.SEALANE_CHROMIUM_PATH || undefined })
// Desktop viewport: the header "Sign out" button is hidden below the sm breakpoint.
const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1280, height: 800 } })
page.setDefaultTimeout(20000)

await page.goto(base + '/')
await page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
await page.getByRole('link', { name: `Sign in with ${provider}`, exact: true }).click()
// Dex: a single mock connector logs in immediately; older versions show a connector list first.
// Wait for the signed-in header, not a page heading: a cluster without Gateway API/Istio lands on /guard.
const account = page.getByRole('link', { name: /Account settings for/ })
const connectorButton = page.getByRole('link', { name: /Log in with Example/i })
await Promise.race([account.waitFor(), connectorButton.first().waitFor()])
if (await connectorButton.count()) await connectorButton.first().click()
await account.waitFor()
const user = await account.getAttribute('aria-label')
if (!/kilgore@kilgore\.trout/.test(user ?? '')) throw new Error('unexpected user: ' + user)
console.log('PASS signed in through', provider, 'as', user)

await page.goto(base + '/settings')
await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor()
await page.getByText(`You signed in through ${provider}`, { exact: false }).waitFor()
if (await page.getByRole('heading', { name: 'Change password' }).count()) throw new Error('password panel shown for an SSO account')
if (!(await page.getByRole('heading', { name: 'Accounts' }).count())) throw new Error('admin account panel missing (role mapping failed)')
console.log('PASS settings page hides password change and shows admin panels')

await page.getByRole('button', { name: 'Sign out', exact: true }).click()
await page.getByRole('heading', { name: 'Sign in to your workspace' }).waitFor()
console.log('PASS sign-out returns to the login page')
await browser.close()
