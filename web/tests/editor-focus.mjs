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
  role: 'admin',
}
const types = [
  {
    typeId: TYPE, group: 'networking.istio.io', version: 'v1', kind: 'VirtualService',
    category: 'traffic', namespaced: true, installed: true, dangerous: false, formCapable: true,
  },
]
// Form-representable, so without ?focus= the editor would open on the Form tab.
const reviews = {
  apiVersion: 'networking.istio.io/v1',
  kind: 'VirtualService',
  metadata: { name: 'reviews', namespace: 'shop', resourceVersion: '7' },
  spec: {
    hosts: ['reviews'],
    http: [
      {
        match: [{ uri: { prefix: '/v2' } }],
        route: [
          { destination: { host: 'reviews', subset: 'v9' }, weight: 90 },
          { destination: { host: 'reviews', subset: 'v1' }, weight: 10 },
        ],
      },
    ],
  },
}
const fixtures = {
  '/api/clusters': [{ name: 'local' }],
  '/api/capabilities': capabilities,
  '/api/resourceTypes': types,
  [`/api/resources/${TYPE}/shop/reviews`]: {
    typeId: TYPE, kind: 'VirtualService', namespace: 'shop', name: 'reviews',
    summary: '', age: '1d', resourceVersion: '7', raw: reviews,
  },
}
const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
await context.route('**/api/**', async (interception) => {
  const path = new URL(interception.request().url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  if (path.startsWith('/api/access/')) return interception.fulfill({ json: { create: true, update: true, delete: true } })
  return interception.fulfill({ json: fixtures[path] ?? [] })
})
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
const yamlTab = page.getByRole('button', { name: 'YAML', exact: true })
const status = page.getByRole('status')

try {
  // findFocusLine on js-yaml-shaped text (module served by the dev server).
  await page.goto(`${base}/`)
  const cases = await page.evaluate(async () => {
    const { findFocusLine } = await import('/src/lib/yamlFocus.ts')
    const y = [
      'spec:', //                         1
      '  gateways:', //                   2
      '    - edge/public', //             3
      '  selector:', //                   4
      '    istio: ingressgateway', //     5
      '  tcp:', //                        6
      '    - match:', //                  7
      '        - port: 1', //             8
      '    - route:', //                  9
      '        - destination:', //        10
      '            host: a', //           11
      '        - destination:', //        12
      '            host: gone', //        13
      '  rules:', //                      14
      '    - backendRefs:', //            15
      '        - name: shop', //          16
      '        - name: ghost', //         17
      '  subsets:', //                    18
      '    - name: v1', //                19
      '      labels:', //                 20
      '        version: v1', //           21
    ].join('\n')
    return [
      findFocusLine(y, 'spec.gateways[0]', 'edge/public'),
      findFocusLine(y, 'spec.selector', 'istio=ingressgateway'),
      findFocusLine(y, 'spec.tcp[1].route[1].destination.host', 'gone'),
      findFocusLine(y, 'spec.rules[0].backendRefs[1].name', 'ghost'),
      findFocusLine(y, 'spec.subsets[0].name'),
      findFocusLine(y, 'spec.subsets[0].labels', 'v1'),
      findFocusLine(y, 'spec.tcp[2].route[0]'),
      findFocusLine(y, 'spec.http[0]'),
      findFocusLine(y, 'spec.version'), // only nested deeper: must not match
    ]
  })
  assert.deepEqual(cases, [3, 4, 13, 17, 19, 21, null, null, null])

  // (a) A finding link opens the YAML tab on the offending line.
  await page.goto(`${base}/resources/${TYPE}/shop/reviews?focus=${encodeURIComponent('spec.http[0].route[0].destination.subset')}&value=v9`)
  const line = page.locator('.cm-focus-line')
  await line.waitFor()
  assert.ok((await yamlTab.getAttribute('class')).includes('border-accent'), 'YAML tab is active')
  assert.match(await line.innerText(), /subset: v9/)
  assert.equal(await page.locator('.cm-focus-line').count(), 1)
  await status.getByText('Showing spec.http[0].route[0].destination.subset').waitFor()
  await status.getByRole('button', { name: 'Dismiss' }).click()
  assert.equal(await status.count(), 0)
  if (process.env.SEALANE_SCREENSHOT) {
    await page.goto(`${base}/resources/${TYPE}/shop/reviews?focus=${encodeURIComponent('spec.http[0].route[0].destination.subset')}&value=v9`)
    await line.waitFor()
    await page.screenshot({ path: process.env.SEALANE_SCREENSHOT })
  }

  // (b) A path that is not in the YAML says so and highlights nothing.
  await page.goto(`${base}/resources/${TYPE}/shop/reviews?focus=spec.tcp[0].route[0].destination.host`)
  await status.getByText('Could not find spec.tcp[0].route[0].destination.host in the YAML').waitFor()
  assert.equal(await page.locator('.cm-focus-line').count(), 0)
  assert.ok((await yamlTab.getAttribute('class')).includes('border-accent'), 'YAML tab is active')

  assert.deepEqual(errors, [])
  console.log('ok')
} finally {
  await browser.close()
}
