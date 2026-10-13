import { useEffect, useState, type ReactNode } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useCapabilities } from './api/capabilities'
import { ApiError, apiPost, getCluster, setCluster } from './api/client'
import { useClusters } from './api/clusters'
import { Layout } from './components/Layout'
import { Guard } from './pages/Guard'
import { Login } from './pages/Login'
import { Home } from './pages/Home'
import { Overview } from './pages/Overview'
import { FlowMap } from './pages/FlowMap'
import { RouteCheck } from './pages/RouteCheck'
import { ResourceList } from './pages/ResourceList'
import { ResourceEdit } from './pages/ResourceEdit'
import { Settings } from './pages/Settings'
import { Icon } from './components/icons'
import { t } from './i18n'

export function App() {
  // Resolve a persisted selection before sending requests to a remote cluster.
  // Layout is mounted only after capabilities succeed, so recovery must live here.
  const clusters = useClusters()
  const selectedCluster = getCluster()
  const remoteSelected = !!selectedCluster && selectedCluster !== 'local'
  const missingCluster =
    remoteSelected && !!clusters.data && !clusters.data.some((c) => c.name === selectedCluster)
  const { data, isLoading, isError, error } = useCapabilities(
    !remoteSelected || (!!clusters.data && !missingCluster),
  )
  const location = useLocation()

  useEffect(() => {
    // Return to the overview so a remote editor cannot reopen a local resource.
    if (missingCluster) setCluster('', '/')
  }, [missingCluster])

  // 401 → no session (or it expired) — show the local-account login.
  if (
    (isError && error instanceof ApiError && error.status === 401) ||
    (remoteSelected && clusters.error instanceof ApiError && clusters.error.status === 401)
  )
    return <Login />
  if (missingCluster || (remoteSelected && clusters.isPending))
    return <Centered>{t('app.checkingCluster')}</Centered>
  if (remoteSelected && clusters.isError) {
    return <ConnectionError message={t('app.clustersFailed', { message: clusters.error.message })} />
  }
  if (isLoading) return <Centered>{t('common.loading')}</Centered>
  if (isError) return <ConnectionError message={error.message} />

  // Initial password (admin/admin) still in use → nothing else is reachable until it changes.
  if (data?.mustChangePassword) return <ForcePasswordChange />

  // No routing CRDs at all → install-guide screen.
  const noCRDs = data && !data.httpRouteInstalled && !data.virtualServiceInstalled
  if (noCRDs && location.pathname !== '/guard') return <Navigate to="/guard" replace />

  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/overview" element={<Overview />} />
        <Route path="/flowmap" element={<FlowMap />} />
        <Route path="/routecheck" element={<RouteCheck />} />
        <Route path="/resources/:type" element={<ResourceList key={location.pathname} />} />
        <Route path="/resources/:type/new" element={<ResourceEdit key={location.pathname} />} />
        <Route path="/resources/:type/:ns/:name" element={<ResourceEdit key={location.pathname} />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/guard" element={<Guard />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}

function ConnectionError({ message }: { message: string }) {
  const remoteSelected = !!getCluster() && getCluster() !== 'local'
  return (
    <Centered>
      <span className="block text-base font-semibold text-strong">{t('app.connectFailed')}</span>
      <span className="mt-2 block">{message}</span>
      <span className="mt-5 flex flex-wrap justify-center gap-2">
        <button className="btn-ghost" onClick={() => window.location.reload()}>
          {t('common.retry')}
        </button>
        {remoteSelected && (
          <button className="btn-primary" onClick={() => setCluster('', '/')}>
            {t('app.backToLocal')}
          </button>
        )}
      </span>
    </Centered>
  )
}

function Centered({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-5 px-6 text-center text-sm text-muted">
      <img src="/favicon.svg" alt="" className="h-11 w-11" />
      <div role="status">{children}</div>
    </div>
  )
}

// Forced initial password change — a full reload refetches capabilities on success.
function ForcePasswordChange() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const canSubmit = current !== '' && next.length >= 8 && next === confirm && !busy

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!canSubmit) return
    setBusy(true)
    setError('')
    try {
      await apiPost('/api/account/password', { currentPassword: current, newPassword: next })
      window.location.reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50 px-4 dark:bg-slate-950">
      <form onSubmit={submit} className="panel w-full max-w-sm space-y-4 rounded-2xl p-8">
        <img src="/favicon.svg" alt="" className="mb-4 h-10 w-10" />
        <h1 className="text-xl font-semibold text-strong">{t('pw.title')}</h1>
        <p className="text-sm text-muted">{t('pw.intro')}</p>
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-muted">{t('pw.current')}</span>
          <input
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            className="input-base w-full"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-muted">{t('pw.new')}</span>
          <input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            className="input-base w-full"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-muted">{t('pw.confirm')}</span>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            className="input-base w-full"
          />
        </label>
        {confirm !== '' && next !== confirm && (
          <p className="text-sm text-red-600 dark:text-red-400">{t('pw.mismatch')}</p>
        )}
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <button type="submit" disabled={!canSubmit} className="btn-primary w-full disabled:opacity-50">
          {busy ? t('pw.busy') : t('pw.submit')}
          <Icon name="arrow" className="h-4 w-4" />
        </button>
      </form>
    </div>
  )
}
