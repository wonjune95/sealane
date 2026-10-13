import assert from 'node:assert/strict'
import { chromium } from 'playwright'

const base = process.env.SEALANE_TEST_URL ?? 'http://127.0.0.1:5173'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SEALANE_CHROMIUM_PATH
    ? { executablePath: process.env.SEALANE_CHROMIUM_PATH }
    : {}),
})
const capabilities = {
  httpRouteInstalled: true,
  virtualServiceInstalled: true,
  namespaceListAllowed: true,
  devMode: false,
  user: 'test-user',
  role: 'viewer',
}
const gateway = (name, egress = false) => ({
  name,
  namespace: 'edge',
  kind: 'IstioGateway',
  typeId: 'gateways.networking.istio.io',
  hosts: ['shop.example.com'],
  egress,
})
const backend = (name, endpoints = 2) => ({
  name,
  namespace: 'apps',
  port: 8080,
  exists: true,
  endpoints,
})
const route = (name, gateways, backends, hosts = ['shop.example.com']) => ({
  name,
  namespace: 'apps',
  kind: 'VirtualService',
  typeId: 'virtualservices.networking.istio.io',
  hosts,
  gateways,
  backends,
})
const fixture = {
  gateways: [gateway('public-gateway'), gateway('outbound-gateway', true)],
  routes: [
    route('checkout-route', ['edge/public-gateway'], [backend('checkout')]),
    route('broken-route', ['edge/public-gateway'], [backend('unready', 0)]),
    route(
      'missing-parent-route',
      ['edge/missing-gateway'],
      [backend('orphan')],
    ),
    route(
      'dual-direction',
      ['edge/public-gateway', 'edge/outbound-gateway'],
      [
        {
          name: 'payments.example.com',
          port: 443,
          external: true,
          serviceEntry: 'payments',
          exists: false,
          endpoints: 0,
        },
      ],
    ),
  ],
  serviceEntries: [
    {
      name: 'payments',
      namespace: 'apps',
      typeId: 'serviceentries.networking.istio.io',
      hosts: ['payments.example.com'],
    },
    {
      name: 'public-api',
      namespace: 'apps',
      typeId: 'serviceentries.networking.istio.io',
      hosts: ['api.example.net'],
    },
  ],
}
let response = fixture
let failure = false
const errors = []
const context = await browser.newContext({
  viewport: { width: 1440, height: 1100 },
})
await context.route('**/api/**', async (interception) => {
  const request = interception.request(),
    path = new URL(request.url()).pathname
  if (!path.startsWith('/api/')) return interception.continue()
  assert.equal(
    request.method(),
    'GET',
    `Unexpected mutation: ${request.method()} ${path}`,
  )
  if (path === '/api/clusters')
    return interception.fulfill({ json: [{ name: 'local' }] })
  if (path === '/api/capabilities')
    return interception.fulfill({ json: capabilities })
  if (path === '/api/flowmap')
    return interception.fulfill(
      failure
        ? {
            status: 500,
            json: {
              error: { reason: 'Internal', message: '테스트 조회 오류' },
            },
          }
        : { json: response },
    )
  return interception.fulfill({ json: [] })
})
const page = await context.newPage()
page.on('pageerror', (error) => errors.push(error.message))
const cards = page.locator('.flow-node')
async function reload() {
  await page.goto(`${base}/flowmap`)
  await page
    .getByRole('heading', { name: '트래픽 흐름', exact: true })
    .waitFor()
}
try {
  await reload()
  await cards.filter({ hasText: 'checkout-route' }).waitFor()
  assert.equal(await cards.count(), 11)
  await page.getByRole('button', { name: '인그레스', exact: false }).click()
  await cards.filter({ hasText: 'dual-direction' }).waitFor()
  await page.getByRole('button', { name: '이그레스', exact: false }).click()
  await cards.filter({ hasText: 'dual-direction' }).waitFor()
  await cards.filter({ hasText: 'public-api' }).waitFor()
  assert.equal(await cards.filter({ hasText: 'public-gateway' }).count(), 0)
  assert.equal(
    await cards.filter({ hasText: 'payments.example.com' }).count(),
    1,
  )
  console.log(
    'PASS: mixed ingress/egress parents and direct ServiceEntry connections remain visible',
  )

  await page.getByRole('button', { name: '인그레스', exact: false }).click()
  await page.getByRole('textbox', { name: '흐름 검색' }).fill('checkout')
  assert.equal(await cards.count(), 4)
  assert.equal(await cards.filter({ hasText: 'public-gateway' }).count(), 1)
  await page
    .getByRole('combobox', { name: '흐름 네임스페이스' })
    .selectOption('apps')
  assert.equal(await cards.count(), 4)
  await page
    .getByRole('button', { name: '필터 초기화', exact: true })
    .first()
    .click()
  await page.getByRole('button', { name: '문제 경로', exact: false }).click()
  await cards.filter({ hasText: 'missing-gateway' }).waitFor()
  assert.equal(await cards.filter({ hasText: 'checkout-route' }).count(), 0)
  assert.equal(await cards.filter({ hasText: 'unready' }).count(), 1)
  console.log(
    'PASS: search, namespace, and problem filters keep related paths without sibling routes',
  )

  await page
    .getByRole('button', { name: '필터 초기화', exact: true })
    .first()
    .click()
  const checkout = cards.filter({ hasText: 'checkout-route' })
  await checkout.focus()
  await page.keyboard.press('Enter')
  const details = page.getByLabel('선택한 노드 상세')
  await details.waitFor()
  assert.equal(
    await details
      .getByRole('link', { name: '리소스 열기' })
      .getAttribute('href'),
    '/resources/virtualservices.networking.istio.io/apps/checkout-route',
  )
  assert.equal(
    await cards
      .filter({ hasText: 'broken-route' })
      .getAttribute('class')
      .then((value) => value.includes('is-dimmed')),
    true,
  )
  await page.keyboard.press('Escape')
  assert.equal(await details.count(), 0)
  // Zoom and drag must use the same coordinate system, and dragging must not select.
  await page.getByRole('button', { name: '흐름도 확대', exact: true }).click()
  const before = await checkout.boundingBox()
  await page.mouse.move(before.x + 45, before.y + 35)
  await page.mouse.down()
  await page.mouse.move(before.x + 75, before.y + 85, { steps: 8 })
  await page.mouse.up()
  const after = await checkout.boundingBox()
  assert.ok(Math.abs(after.y - before.y - 50) < 3)
  assert.equal(await details.count(), 0)
  await page.getByRole('button', { name: '배치 초기화' }).click()
  assert.equal(
    await checkout.evaluate((el) => el.style.transform),
    'translate(0px, 0px)',
  )
  console.log(
    'PASS: keyboard selection, focus-visible links, scaled drag, Escape and layout reset work',
  )

  await checkout.click()
  await page.getByRole('button', { name: '다크 모드', exact: true }).click()
  await page.getByRole('button', { name: '라이트 모드', exact: true }).click()
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.waitForTimeout(150)
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth,
      ),
      false,
      `Document overflow at ${width}`,
    )
    const viewport = page.locator('.flow-viewport')
    assert.ok(await viewport.evaluate((el) => el.scrollWidth >= el.clientWidth))
  }
  await page.setViewportSize({ width: 390, height: 1000 })
  await page.getByRole('button', { name: '화면에 맞춤' }).click()
  assert.equal(
    await page
      .locator('.flow-viewport')
      .evaluate((el) => el.scrollWidth > el.clientWidth + 1),
    false,
  )
  await page.emulateMedia({ reducedMotion: 'reduce' })
  assert.equal(
    await page
      .locator('.flow-edge-motion')
      .first()
      .evaluate((el) => getComputedStyle(el).animationName),
    'none',
  )
  console.log(
    'PASS: 320–1440px layouts contain horizontal scrolling; fit and reduced motion work',
  )

  await page.getByRole('textbox', { name: '흐름 검색' }).fill('no-such-host')
  await page.getByText('일치하는 경로가 없습니다', { exact: true }).waitFor()
  await page
    .getByRole('button', { name: '필터 초기화', exact: true })
    .first()
    .click()
  response = { gateways: [], routes: [], serviceEntries: [] }
  await reload()
  await page.getByText('인그레스 경로가 없습니다', { exact: true }).waitFor()
  failure = true
  await reload()
  await page.getByRole('alert').waitFor()
  assert.equal(
    await page.getByText('인그레스 경로가 없습니다', { exact: true }).count(),
    0,
  )
  failure = false
  response = fixture
  await page.getByRole('button', { name: '다시 시도', exact: true }).click()
  await cards.filter({ hasText: 'checkout-route' }).waitFor()
  assert.deepEqual(errors, [])
  console.log(
    'PASS: empty, unmatched, failed and recovered API states remain distinct; no runtime errors',
  )
} finally {
  await browser.close()
}
