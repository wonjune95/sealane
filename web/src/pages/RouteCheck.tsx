import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useCapabilities } from '../api/capabilities'
import { ApiError } from '../api/client'
import { useNamespaces } from '../api/namespaces'
import { checkRoute, type RouteMatchResult } from '../api/routematch'
import { sendRequestTest, usePods, type RequestTestResult } from '../api/requesttest'
import { Icon } from '../components/icons'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
type KV = { key: string; value: string; on: boolean }
type ResTab = 'body' | 'headers' | 'route'

// URL 한 줄에서 시뮬레이터가 쓸 호스트/경로를 뽑는다 (Istio 호스트에는 포트가 없다).
function parseUrl(raw: string) {
  try {
    const u = new URL(raw.includes('://') ? raw : `http://${raw}`)
    return { hostname: u.hostname, path: u.pathname + u.search, href: u.toString() }
  } catch {
    return null
  }
}

// 요청 하나를 작성해 두 가지로 확인한다 — 설정상 어디로 가는지(경로 확인),
// 그리고 실제로 어떻게 응답하는지(보내기, opt-in).
export function RouteCheck() {
  const caps = useCapabilities()
  const testerOn = !!caps.data?.requestTester && caps.data.role === 'admin'

  const [method, setMethod] = useState('GET')
  const [url, setUrl] = useState('')
  const [headers, setHeaders] = useState<KV[]>([{ key: '', value: '', on: true }])
  const [body, setBody] = useState('')
  const [ns, setNs] = useState('')
  const [pod, setPod] = useState('')
  const [container, setContainer] = useState('')

  const [resTab, setResTab] = useState<ResTab>('route')
  const [route, setRoute] = useState<RouteMatchResult | null>(null)
  const [res, setRes] = useState<RequestTestResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'check' | 'send'>('')

  const parsed = useMemo(() => parseUrl(url.trim()), [url])
  const activeHeaders = headers.filter((h) => h.on && h.key.trim())
  const hasBody = method !== 'GET' && method !== 'HEAD'

  const check = async () => {
    if (!parsed || busy) return
    setBusy('check')
    setError('')
    try {
      setRoute(
        await checkRoute({
          host: parsed.hostname,
          path: parsed.path,
          method,
          headers: activeHeaders.map(({ key, value }) => ({ key, value })),
        }),
      )
      setResTab('route')
    } catch (err) {
      setRoute(null)
      setError(err instanceof ApiError ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  const send = async () => {
    if (!parsed || !ns || !pod || !container || busy) return
    setBusy('send')
    setError('')
    try {
      const h: Record<string, string> = {}
      for (const x of activeHeaders) h[x.key.trim()] = x.value.trim()
      setRes(
        await sendRequestTest({
          namespace: ns,
          pod,
          container,
          method,
          url: parsed.href,
          headers: h,
          body: hasBody && body ? body : undefined,
        }),
      )
      setResTab('body')
    } catch (err) {
      setRes(null)
      setError(err instanceof ApiError ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  const canSend = testerOn && !!parsed && !!ns && !!pod && !!container && !busy

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Request workbench"
        title="요청 콘솔"
        description="요청 조건에 맞는 라우팅 경로를 확인하고, 선택한 파드에서 실제 응답을 검증하세요."
      />

      {/* URL 바 */}
      <div className="panel flex flex-wrap items-center gap-2 rounded-xl p-3">
        <select
          value={method}
          onChange={(e) => setMethod(e.target.value)}
          aria-label="HTTP 메서드"
          className="input-base !w-24 shrink-0 font-semibold text-accent"
        >
          {METHODS.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
        <input
          aria-label="요청 URL"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') void check()
          }}
          placeholder="http://shop.example.com/api/orders"
          autoComplete="off"
          spellCheck={false}
          className="input-base min-w-0 basis-full sm:basis-0 flex-1 font-mono text-sm"
        />
        <button
          onClick={check}
          disabled={!parsed || !!busy}
          className="btn-ghost shrink-0 disabled:opacity-50"
        >
          <Icon name="route" className="h-4 w-4" />
          {busy === 'check' ? '계산 중…' : '경로 확인'}
        </button>
        <button
          onClick={send}
          disabled={!canSend}
          title={
            !testerOn
              ? '요청 테스터가 꺼져 있습니다 — 아래 출발 항목 참고'
              : !ns || !pod
                ? '아래 출발에서 파드를 고르세요'
                : ''
          }
          className="btn-primary shrink-0 disabled:opacity-40"
        >
          <Icon name="send" className="h-4 w-4" />
          {busy === 'send' ? '보내는 중…' : '보내기'}
        </button>
      </div>
      {url && !parsed && <p className="text-sm text-red-600 dark:text-red-400">URL 형식을 확인하세요.</p>}
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {/* 요청 편집 — 출발지를 먼저 정하고 헤더·본문을 아래에서 채운다 */}
      <div className="panel rounded-xl">
        <Section title="출발" hint={testerOn ? undefined : '요청 테스터 꺼짐 — 경로 확인만 가능'}>
          <SourcePicker
            enabled={testerOn}
            isAdmin={caps.data?.role === 'admin'}
            ns={ns}
            setNs={(v) => {
              setNs(v)
              setPod('')
            }}
            pod={pod}
            setPod={setPod}
            container={container}
            setContainer={setContainer}
          />
        </Section>

        <Section
          title="헤더"
          hint={activeHeaders.length ? `${activeHeaders.length}개 사용` : undefined}
          bordered
        >
          <HeaderEditor headers={headers} setHeaders={setHeaders} />
        </Section>

        {hasBody && (
          <Section title="본문" bordered>
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder='{"name": "값"}'
              rows={5}
              spellCheck={false}
              className="input-base w-full font-mono text-xs"
            />
          </Section>
        )}
      </div>

      {!route && !res && !busy && !error && (
        <div className="panel">
          <EmptyState
            icon="terminal"
            title="요청을 입력하면 분석 결과가 표시됩니다"
            description="URL을 입력하고 경로 확인을 눌러 매칭되는 규칙과 목적지를 살펴보세요."
          />
        </div>
      )}
      {/* 응답 */}
      {(route || res) && (
        <div className="panel rounded-xl">
          <Tabs
            tabs={[
              ...(res ? [{ id: 'body' as const, label: '응답 본문' }] : []),
              ...(res?.headers
                ? [{ id: 'headers' as const, label: '응답 헤더', count: Object.keys(res.headers).length }]
                : []),
              ...(route ? [{ id: 'route' as const, label: '경로 분석' }] : []),
            ]}
            active={resTab}
            onChange={(t) => setResTab(t as ResTab)}
            right={res ? <ResponseStatus res={res} /> : undefined}
          />
          <div className="p-4">
            {resTab === 'body' && res && <ResponseBody res={res} />}
            {resTab === 'headers' && res?.headers && (
              <dl className="grid grid-cols-[minmax(9rem,auto)_1fr] gap-x-4 gap-y-1.5 font-mono text-xs">
                {Object.entries(res.headers).map(([k, v]) => (
                  <div key={k} className="contents">
                    <dt className="truncate text-muted">{k}</dt>
                    <dd className="break-all text-strong">{v}</dd>
                  </div>
                ))}
              </dl>
            )}
            {resTab === 'route' && route && <RouteResult result={route} />}
          </div>
        </div>
      )}
    </div>
  )
}

// 요청 편집 패널의 한 구획 (제목 + 내용). 탭 대신 한 화면에 쌓는다.
function Section({
  title,
  hint,
  bordered,
  children,
}: {
  title: string
  hint?: string
  bordered?: boolean
  children: ReactNode
}) {
  return (
    <section
      className={`space-y-2.5 p-4 ${bordered ? 'border-t border-gray-200 dark:border-slate-700' : ''}`}
    >
      <div className="flex items-baseline gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-faint">{title}</h3>
        {hint && <span className="text-xs text-muted">{hint}</span>}
      </div>
      {children}
    </section>
  )
}

function Tabs({
  tabs,
  active,
  onChange,
  right,
}: {
  tabs: { id: string; label: string; count?: number; hint?: string }[]
  active: string
  onChange: (id: string) => void
  right?: ReactNode
}) {
  return (
    <div className="flex items-center gap-1 border-b border-base px-2">
      {tabs.map((t) => (
        <button
          key={t.id}
          onClick={() => onChange(t.id)}
          className={`-mb-px border-b-2 px-3 py-2.5 text-sm transition ${
            active === t.id
              ? 'border-accent font-medium text-accent'
              : 'border-transparent text-muted hover:text-strong'
          }`}
        >
          {t.label}
          {t.count ? <span className="ml-1.5 text-xs text-faint">{t.count}</span> : null}
          {t.hint ? (
            <span className="ml-1.5 max-w-[10rem] truncate align-bottom text-xs text-faint">{t.hint}</span>
          ) : null}
        </button>
      ))}
      {right && <div className="ml-auto pr-2">{right}</div>}
    </div>
  )
}

function HeaderEditor({
  headers,
  setHeaders,
}: {
  headers: KV[]
  setHeaders: (f: (p: KV[]) => KV[]) => void
}) {
  const patch = (i: number, p: Partial<KV>) =>
    setHeaders((prev) => prev.map((h, j) => (i === j ? { ...h, ...p } : h)))
  return (
    <div className="space-y-2">
      {headers.map((h, i) => (
        <div key={i} className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={h.on}
            onChange={(e) => patch(i, { on: e.target.checked })}
            aria-label={`헤더 ${i + 1} 사용`}
            title="이 헤더 사용"
            className="h-4 w-4 shrink-0 accent-blue-600"
          />
          <input
            value={h.key}
            onChange={(e) => patch(i, { key: e.target.value })}
            aria-label={`헤더 ${i + 1} 이름`}
            placeholder="이름 (예: x-user)"
            autoComplete="off"
            className="input-base min-w-0 flex-1 font-mono text-xs"
          />
          <input
            value={h.value}
            onChange={(e) => patch(i, { value: e.target.value })}
            aria-label={`헤더 ${i + 1} 값`}
            placeholder="값 (예: beta)"
            autoComplete="off"
            className="input-base min-w-0 flex-1 font-mono text-xs"
          />
          <button
            type="button"
            onClick={() =>
              setHeaders((prev) =>
                prev.length === 1 ? [{ key: '', value: '', on: true }] : prev.filter((_, j) => j !== i),
              )
            }
            className="btn-ghost shrink-0 px-2.5"
            aria-label={`헤더 ${i + 1} 삭제`}
            title="삭제"
          >
            <Icon name="close" className="h-4 w-4" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={() => setHeaders((prev) => [...prev, { key: '', value: '', on: true }])}
        className="text-xs text-accent hover:underline"
      >
        + 헤더 추가
      </button>
    </div>
  )
}

// 출발 파드 선택. 기능이 꺼져 있어도 탭은 남기고 켜는 방법을 안내한다.
function SourcePicker({
  enabled,
  isAdmin,
  ns,
  setNs,
  pod,
  setPod,
  container,
  setContainer,
}: {
  enabled: boolean
  isAdmin: boolean
  ns: string
  setNs: (v: string) => void
  pod: string
  setPod: (v: string) => void
  container: string
  setContainer: (v: string) => void
}) {
  const namespaces = useNamespaces()
  const pods = usePods(ns, enabled && ns !== '')
  const selected = pods.data?.find((p) => p.name === pod)

  useEffect(() => {
    if (!selected) return
    if (!selected.containers.includes(container)) {
      setContainer(selected.containers.find((c) => c !== 'istio-proxy') ?? selected.containers[0] ?? '')
    }
  }, [selected, container, setContainer])

  if (!enabled) {
    return (
      <div className="space-y-2 text-sm">
        <p className="text-muted">
          <strong className="text-strong">실제 요청 보내기</strong>는 꺼져 있습니다. 켜면 고른 파드 안에서
          요청을 보내 사이드카를 통과한 실제 응답(라우팅·인가·mTLS 결과)을 확인할 수 있습니다.
        </p>
        {isAdmin ? (
          <details className="text-xs">
            <summary className="cursor-pointer font-medium text-accent">관리자 활성화 안내</summary>
            <div className="mt-3 space-y-2">
              <p className="text-xs text-muted">
                배포 설정에서 요청 테스터를 활성화할 수 있습니다. ServiceAccount에 pods/exec 권한이
                추가됩니다.
              </p>
              <pre className="panel-soft overflow-x-auto rounded-lg p-3 text-xs">
                helm upgrade sealane charts/sealane -n sealane --reuse-values --set
                requestTester.enabled=true
              </pre>
            </div>
          </details>
        ) : (
          <p className="text-xs text-muted">admin만 사용할 수 있는 기능입니다.</p>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted">
        고른 파드 안에서 요청을 보냅니다.{' '}
        <span className="text-amber-600 dark:text-amber-400">실제 트래픽이라 부작용이 있을 수 있습니다.</span>
      </p>
      <div className="flex flex-wrap gap-2">
        <select
          value={ns}
          onChange={(e) => setNs(e.target.value)}
          aria-label="출발 네임스페이스"
          className="input-base !w-full sm:!w-56 shrink-0"
        >
          <option value="">네임스페이스 선택</option>
          {(namespaces.data ?? []).map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
        <select
          value={pod}
          onChange={(e) => setPod(e.target.value)}
          disabled={!ns}
          aria-label="출발 파드"
          className="input-base min-w-0 basis-full sm:basis-0 flex-1 disabled:opacity-50"
        >
          <option value="">{pods.isLoading ? '불러오는 중…' : '출발 파드 선택'}</option>
          {(pods.data ?? []).map((p) => (
            <option key={p.name} value={p.name}>
              {p.name}
              {p.mesh ? ' · 메시' : ''}
            </option>
          ))}
        </select>
        {selected && selected.containers.length > 1 && (
          <select
            value={container}
            onChange={(e) => setContainer(e.target.value)}
            aria-label="출발 컨테이너"
            className="input-base !w-full sm:!w-44 shrink-0"
          >
            {selected.containers.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        )}
      </div>
      {selected && !selected.mesh && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          이 파드에는 사이드카(istio-proxy)가 없습니다 — Istio 라우팅을 거치지 않으므로 VirtualService
          검증에는 쓸 수 없습니다.
        </p>
      )}
    </div>
  )
}

function ResponseStatus({ res }: { res: RequestTestResult }) {
  const ok = !!res.status && res.status < 400
  return (
    <div className="flex items-center gap-2 text-xs">
      {res.status ? (
        <span
          className={`chip ${ok ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-red-500/10 text-red-600 dark:text-red-400'}`}
        >
          {res.status} {res.statusText}
        </span>
      ) : (
        <span className="chip bg-red-500/10 text-red-600 dark:text-red-400">응답 없음</span>
      )}
      <span className="text-muted">{res.durationMs}ms</span>
      {res.body && <span className="text-muted">{formatBytes(res.body.length)}</span>}
    </div>
  )
}

function ResponseBody({ res }: { res: RequestTestResult }) {
  const pretty = useMemo(() => {
    if (!res.body) return ''
    try {
      return JSON.stringify(JSON.parse(res.body), null, 2)
    } catch {
      return res.body
    }
  }, [res.body])

  return (
    <div className="space-y-3">
      {res.hint && (
        <p className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-2.5 text-xs text-strong">
          {res.hint}
        </p>
      )}
      {res.error && <p className="text-sm text-red-600 dark:text-red-400">{res.error}</p>}
      {res.headers?.['x-envoy-upstream-service-time'] && (
        <p className="text-xs text-muted">
          업스트림 처리 {res.headers['x-envoy-upstream-service-time']}ms · Envoy 경유
        </p>
      )}
      {pretty && (
        <pre className="panel-soft max-h-96 overflow-auto rounded-lg p-3 text-xs text-strong">{pretty}</pre>
      )}
      {res.truncated && <p className="text-xs text-muted">응답이 너무 커서 잘렸습니다.</p>}
    </div>
  )
}

function formatBytes(n: number) {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`
}

function RouteResult({ result }: { result: RouteMatchResult }) {
  return (
    <div className="space-y-4">
      <div
        className={`rounded-xl border p-4 ${result.matched ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-amber-500/30 bg-amber-500/5'}`}
      >
        <div className="flex items-center gap-2 text-sm font-medium">
          <Icon
            name={result.matched ? 'check' : 'warning'}
            className={`h-4 w-4 ${result.matched ? 'text-emerald-500' : 'text-amber-500'}`}
          />
          {result.matched ? '매치되는 라우트를 찾았습니다' : '매치되는 라우트가 없습니다'}
        </div>
        {result.destinations.length > 0 && (
          <div className="mt-3 space-y-2">
            {result.destinations.map((d, i) => (
              <div
                key={i}
                className="flex flex-wrap items-center gap-2 rounded-lg bg-white/70 px-3 py-2 text-sm dark:bg-slate-900/60"
              >
                <span className="font-medium text-strong">
                  {d.host}
                  {d.port ? <span className="font-normal text-muted">:{d.port}</span> : null}
                </span>
                {d.subset && (
                  <span
                    className={`chip ${d.subsetOk === false ? 'bg-red-500/10 text-red-600 dark:text-red-400' : 'bg-accent-soft text-accent'}`}
                  >
                    subset {d.subset}
                    {d.subsetOk === false ? ' · DR에 정의 없음' : ''}
                  </span>
                )}
                {d.weight ? (
                  <span className="chip bg-gray-500/10 text-gray-500 dark:text-slate-400">
                    weight {d.weight}
                  </span>
                ) : null}
                {d.external ? (
                  <span className="chip bg-gray-500/10 text-gray-500 dark:text-slate-400">외부 호스트</span>
                ) : d.exists ? (
                  <span
                    className={`chip ${d.endpoints > 0 ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-red-500/10 text-red-600 dark:text-red-400'}`}
                  >
                    엔드포인트 {d.endpoints}
                  </span>
                ) : (
                  <span className="chip bg-red-500/10 text-red-600 dark:text-red-400">서비스 없음</span>
                )}
              </div>
            ))}
          </div>
        )}
        {result.notes?.map((n, i) => (
          <p key={i} className="mt-2 text-xs text-muted">
            · {n}
          </p>
        ))}
      </div>

      <div>
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-faint">평가 과정</h4>
        {result.candidates.length === 0 ? (
          <p className="text-sm text-muted">이 호스트를 다루는 라우트가 없습니다.</p>
        ) : (
          <ul className="space-y-2">
            {result.candidates.map((c, i) => (
              <li
                key={i}
                className={`rounded-lg border p-3 text-sm ${c.winner ? 'border-accent bg-accent-soft' : c.matched ? 'border-base' : 'border-base opacity-70'}`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`chip ${c.matched ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-gray-500/10 text-gray-500 dark:text-slate-400'}`}
                  >
                    {c.matched ? '매치' : '불매치'}
                  </span>
                  <Link
                    to={`/resources/${c.typeId}/${c.namespace}/${c.name}`}
                    className="font-medium text-strong hover:text-accent"
                  >
                    {c.kind} · {c.namespace}/{c.name}
                  </Link>
                  <span className="text-xs text-muted">rule[{c.ruleIndex}]</span>
                  {c.winner && <span className="chip bg-accent text-white">선택됨</span>}
                </div>
                <p className="mt-1 text-xs text-muted">{c.reason}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
