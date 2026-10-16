import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useCapabilities } from '../api/capabilities'
import { ApiError } from '../api/client'
import { useNamespaces } from '../api/namespaces'
import { useServices } from '../api/services'
import { checkRoute, type RouteMatchResult } from '../api/routematch'
import { sendRequestTest, usePods, type RequestTestResult } from '../api/requesttest'
import { Icon } from '../components/icons'
import { t } from '../i18n'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
type KV = { key: string; value: string; on: boolean }
type ResTab = 'body' | 'headers' | 'route'

// Extract the host/path the simulator needs from a single URL line (Istio hosts carry no port).
function parseUrl(raw: string) {
  try {
    const u = new URL(raw.includes('://') ? raw : `http://${raw}`)
    const port = Number(u.port) || (u.protocol === 'https:' ? 443 : 80)
    return { hostname: u.hostname, port, path: u.pathname + u.search, href: u.toString() }
  } catch {
    return null
  }
}

// Compose one request and check it two ways: where the config sends it (Check route)
// and how it actually responds (Send, opt-in).
export function RouteCheck() {
  const caps = useCapabilities()
  const testerOn = !!caps.data?.requestTester && caps.data.role === 'admin'

  const [method, setMethod] = useState('GET')
  const [url, setUrl] = useState('')
  const [headers, setHeaders] = useState<KV[]>([{ key: '', value: '', on: true }])
  const [body, setBody] = useState('')
  const [ns, setNs] = useState('')
  const [pod, setPod] = useState('')
  const [pickNs, setPickNs] = useState('')
  const [pickSvc, setPickSvc] = useState('')
  const [pickPort, setPickPort] = useState(0)
  const [container, setContainer] = useState('')

  const [resTab, setResTab] = useState<ResTab>('route')
  // reqPort: the port the check ran with, so the pill does not follow later URL edits.
  const [route, setRoute] = useState<(RouteMatchResult & { reqPort: number }) | null>(null)
  const [res, setRes] = useState<RequestTestResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'check' | 'send'>('')

  const parsed = useMemo(() => parseUrl(url.trim()), [url])
  const namespaces = useNamespaces()
  const services = useServices(pickNs)
  const svc = services.data?.find((x) => x.name === pickSvc)
  // Point the URL at a Service port, keeping whatever path is already typed.
  const fillUrl = (name: string, port: number) => {
    const keep = parseUrl(url.trim())
    setUrl(`http://${name}.${pickNs}.svc.cluster.local:${port}${keep ? keep.path : '/'}`)
  }
  const activeHeaders = headers.filter((h) => h.on && h.key.trim())
  const hasBody = method !== 'GET' && method !== 'HEAD'

  const check = async () => {
    if (!parsed || busy) return
    setBusy('check')
    setError('')
    try {
      const result = await checkRoute({
        host: parsed.hostname,
        path: parsed.path,
        method,
        port: parsed.port,
        headers: activeHeaders.map(({ key, value }) => ({ key, value })),
      })
      setRoute({ ...result, reqPort: parsed.port })
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
        eyebrow={t('eyebrow.routecheck')}
        title={t('rt.title')}
        description={t('rt.description')}
      />

      {/* URL bar */}
      <div className="panel flex flex-wrap items-center gap-2 rounded-xl p-3">
        <select
          value={method}
          onChange={(e) => setMethod(e.target.value)}
          aria-label={t('rt.method')}
          className="input-base !w-24 shrink-0 font-semibold text-accent"
        >
          {METHODS.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
        <input
          aria-label={t('rt.url')}
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
          {busy === 'check' ? t('rt.checking') : t('rt.check')}
        </button>
        <button
          onClick={send}
          disabled={!canSend}
          title={
            !testerOn
              ? t('rt.testerOffTitle')
              : !ns || !pod
                ? t('rt.pickPodTitle')
                : ''
          }
          className="btn-primary shrink-0 disabled:opacity-40"
        >
          <Icon name="send" className="h-4 w-4" />
          {busy === 'send' ? t('rt.sending') : t('rt.send')}
        </button>
        <div className="flex basis-full flex-wrap gap-2">
          <select
            value={pickNs}
            onChange={(e) => {
              setPickNs(e.target.value)
              setPickSvc('')
            }}
            aria-label={t('rt.pickNamespace')}
            className="input-base !w-full sm:!w-56 shrink-0"
          >
            <option value="">{t('rt.pickNamespace')}</option>
            {(namespaces.data ?? []).map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
          <select
            value={pickSvc}
            onChange={(e) => {
              const next = services.data?.find((x) => x.name === e.target.value)
              setPickSvc(e.target.value)
              if (next?.ports.length) {
                setPickPort(next.ports[0])
                fillUrl(next.name, next.ports[0])
              }
            }}
            disabled={!pickNs}
            aria-label={t('rt.pickService')}
            className="input-base min-w-0 basis-full sm:basis-0 flex-1 disabled:opacity-50"
          >
            <option value="">{t('rt.pickService')}</option>
            {(services.data ?? []).map((x) => (
              <option key={x.name} value={x.name}>
                {x.name}
              </option>
            ))}
          </select>
          {svc && svc.ports.length > 1 && (
            <select
              value={pickPort}
              onChange={(e) => {
                setPickPort(Number(e.target.value))
                fillUrl(svc.name, Number(e.target.value))
              }}
              aria-label={t('rt.pickPort')}
              className="input-base !w-28 shrink-0"
            >
              {svc.ports.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
      {url && !parsed && <p className="text-sm text-red-600 dark:text-red-400">{t('rt.badUrl')}</p>}
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {/* Request editor: pick the source first, then fill headers and body below */}
      <div className="panel rounded-xl">
        <Section title={t('rt.source')} hint={testerOn ? undefined : t('rt.testerOffHint')}>
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
          title={t('rt.headers')}
          hint={activeHeaders.length ? t('rt.headersActive', { n: activeHeaders.length }) : undefined}
          bordered
        >
          <HeaderEditor headers={headers} setHeaders={setHeaders} />
        </Section>

        {hasBody && (
          <Section title={t('rt.body')} bordered>
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder={t('rt.bodyPlaceholder')}
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
            title={t('rt.emptyTitle')}
            description={t('rt.emptyDescription')}
          />
        </div>
      )}
      {/* Response */}
      {(route || res) && (
        <div className="panel rounded-xl">
          <Tabs
            tabs={[
              ...(res ? [{ id: 'body' as const, label: t('rt.tab.body') }] : []),
              ...(res?.headers
                ? [{ id: 'headers' as const, label: t('rt.tab.headers'), count: Object.keys(res.headers).length }]
                : []),
              ...(route ? [{ id: 'route' as const, label: t('rt.tab.route') }] : []),
            ]}
            active={resTab}
            onChange={(id) => setResTab(id as ResTab)}
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
            {resTab === 'route' && route && <RouteResult result={route} reqPort={route.reqPort} />}
          </div>
        </div>
      )}
    </div>
  )
}

// One block of the request editor (title + content), stacked on one screen instead of tabs.
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
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={`-mb-px border-b-2 px-3 py-2.5 text-sm transition ${
            active === tab.id
              ? 'border-accent font-medium text-accent'
              : 'border-transparent text-muted hover:text-strong'
          }`}
        >
          {tab.label}
          {tab.count ? <span className="ml-1.5 text-xs text-faint">{tab.count}</span> : null}
          {tab.hint ? (
            <span className="ml-1.5 max-w-[10rem] truncate align-bottom text-xs text-faint">{tab.hint}</span>
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
            aria-label={t('rt.header.use', { n: i + 1 })}
            title={t('rt.header.useTitle')}
            className="h-4 w-4 shrink-0 accent-blue-600"
          />
          <input
            value={h.key}
            onChange={(e) => patch(i, { key: e.target.value })}
            aria-label={t('rt.header.name', { n: i + 1 })}
            placeholder={t('rt.header.namePlaceholder')}
            autoComplete="off"
            className="input-base min-w-0 flex-1 font-mono text-xs"
          />
          <input
            value={h.value}
            onChange={(e) => patch(i, { value: e.target.value })}
            aria-label={t('rt.header.value', { n: i + 1 })}
            placeholder={t('rt.header.valuePlaceholder')}
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
            aria-label={t('rt.header.delete', { n: i + 1 })}
            title={t('common.delete')}
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
        {t('rt.header.add')}
      </button>
    </div>
  )
}

// Source pod picker. When the feature is off the block stays and explains how to enable it.
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
          {/* en value of rt.testerOffAfter starts with a space on purpose (ko value does not) */}
          <strong className="text-strong">{t('rt.testerOffStrong')}</strong>
          {t('rt.testerOffAfter')}
        </p>
        {isAdmin ? (
          <details className="text-xs">
            <summary className="cursor-pointer font-medium text-accent">{t('rt.adminGuide')}</summary>
            <div className="mt-3 space-y-2">
              <p className="text-xs text-muted">{t('rt.testerDisabledHint')}</p>
              <pre className="panel-soft overflow-x-auto rounded-lg p-3 text-xs">
                <code>
                  helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.8.0 -n sealane
                  --reuse-values --set requestTester.enabled=true
                </code>
              </pre>
            </div>
          </details>
        ) : (
          <p className="text-xs text-muted">{t('rt.adminOnly')}</p>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted">
        {t('rt.sendsFromPod')}{' '}
        <span className="text-amber-600 dark:text-amber-400">{t('rt.realTraffic')}</span>
      </p>
      <div className="flex flex-wrap gap-2">
        <select
          value={ns}
          onChange={(e) => setNs(e.target.value)}
          aria-label={t('rt.sourceNamespace')}
          className="input-base !w-full sm:!w-56 shrink-0"
        >
          <option value="">{t('rt.selectNamespace')}</option>
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
          aria-label={t('rt.sourcePod')}
          className="input-base min-w-0 basis-full sm:basis-0 flex-1 disabled:opacity-50"
        >
          <option value="">{pods.isLoading ? t('common.loading') : t('rt.selectPod')}</option>
          {(pods.data ?? []).map((p) => (
            <option key={p.name} value={p.name}>
              {p.name}
              {p.mesh ? ` · ${t('rt.mesh')}` : ''}
            </option>
          ))}
        </select>
        {selected && selected.containers.length > 1 && (
          <select
            value={container}
            onChange={(e) => setContainer(e.target.value)}
            aria-label={t('rt.sourceContainer')}
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
          {t('rt.noSidecar')}
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
        <span className="chip bg-red-500/10 text-red-600 dark:text-red-400">{t('rt.noResponse')}</span>
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
  // Envoy's own 503 (no upstream service time): an empty body means no cluster for this
  // host:port; "no healthy upstream" means the cluster exists but has no ready endpoints.
  const envoy503 =
    res.status === 503 && res.headers?.server === 'envoy' && !res.headers['x-envoy-upstream-service-time']
  const hint =
    res.hint ||
    (envoy503 && !res.body
      ? t('routecheck.hint503NoUpstream')
      : envoy503 && res.body?.includes('no healthy upstream')
        ? t('routecheck.hint503NoHealthy')
        : '')

  return (
    <div className="space-y-3">
      {hint && (
        <p className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-2.5 text-xs text-strong">
          {hint}
        </p>
      )}
      {res.error && <p className="text-sm text-red-600 dark:text-red-400">{res.error}</p>}
      {res.headers?.['x-envoy-upstream-service-time'] && (
        <p className="text-xs text-muted">
          {t('rt.upstreamTime', { ms: res.headers['x-envoy-upstream-service-time'] })}
        </p>
      )}
      {pretty && (
        <pre className="panel-soft max-h-96 overflow-auto rounded-lg p-3 text-xs text-strong">{pretty}</pre>
      )}
      {res.truncated && <p className="text-xs text-muted">{t('rt.truncated')}</p>}
    </div>
  )
}

function formatBytes(n: number) {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`
}

function RouteResult({ result, reqPort }: { result: RouteMatchResult; reqPort: number }) {
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
          {result.matched ? t('rt.matched') : t('rt.notMatched')}
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
                    {d.subsetOk === false ? ` · ${t('rt.subsetMissing')}` : ''}
                  </span>
                )}
                {d.weight ? (
                  <span className="chip bg-gray-500/10 text-gray-500 dark:text-slate-400">
                    weight {d.weight}
                  </span>
                ) : null}
                {d.external ? (
                  <span className="chip bg-gray-500/10 text-gray-500 dark:text-slate-400">{t('rt.externalHost')}</span>
                ) : d.exists ? (
                  <span
                    className={`chip ${d.endpoints > 0 ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-red-500/10 text-red-600 dark:text-red-400'}`}
                  >
                    {t('flow.status.endpoints', { n: d.endpoints })}
                  </span>
                ) : (
                  <span className="chip bg-red-500/10 text-red-600 dark:text-red-400">{t('rt.noService')}</span>
                )}
                {d.exists && !d.portOk && (
                  <span className="chip bg-red-500/10 text-red-600 dark:text-red-400">
                    {t('routecheck.portMissing', { port: d.port || reqPort, ports: (d.servicePorts ?? []).join(', ') })}
                  </span>
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
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-faint">{t('rt.evaluation')}</h4>
        {result.candidates.length === 0 ? (
          <p className="text-sm text-muted">{t('rt.noCandidates')}</p>
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
                    {c.matched ? t('rt.match') : t('rt.noMatch')}
                  </span>
                  <Link
                    to={`/resources/${c.typeId}/${c.namespace}/${c.name}`}
                    className="font-medium text-strong hover:text-accent"
                  >
                    {c.kind} · {c.namespace}/{c.name}
                  </Link>
                  <span className="text-xs text-muted">rule[{c.ruleIndex}]</span>
                  {c.winner && <span className="chip bg-accent text-white">{t('rt.selected')}</span>}
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
