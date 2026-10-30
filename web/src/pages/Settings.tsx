import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { load as yamlLoad } from 'js-yaml'
import { useCapabilities } from '../api/capabilities'
import { useClusters, type Cluster } from '../api/clusters'
import { apiDelete, apiGet, apiPost, apiPut, errorText, getCluster, setCluster } from '../api/client'
import { useToast } from '../components/Toast'
import { PageHeader } from '../components/PageHeader'
import { Icon } from '../components/icons'
import { Modal } from '../components/Modal'
import { t } from '../i18n'

// My account info + password change.
export function Settings() {
  const caps = useCapabilities()
  const toast = useToast()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)

  const mismatch = confirm !== '' && next !== confirm
  const canSubmit = current !== '' && next.length >= 8 && next === confirm && !busy

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!canSubmit) return
    setBusy(true)
    try {
      await apiPost('/api/account/password', { currentPassword: current, newPassword: next })
      toast('success', t('settings.passwordChanged'))
      setCurrent('')
      setNext('')
      setConfirm('')
    } catch (err) {
      toast('error', errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow={t('eyebrow.settings')}
        title={t('settings.title')}
        description={t('settings.description')}
      />
      <div className="grid items-start gap-5 xl:grid-cols-2">
        <div className="space-y-5">
        <div className="panel rounded-xl p-5">
          <h3 className="mb-4 flex items-center gap-2 text-sm font-semibold text-strong">
            <Icon name="user" className="h-4 w-4 text-accent" />
            {t('settings.myAccount')}
          </h3>
          <dl className="grid grid-cols-[6rem_1fr] gap-y-1.5 text-sm">
            <dt className="text-muted">{t('login.username')}</dt>
            <dd className="text-strong">{caps.data?.user ?? '—'}</dd>
            <dt className="text-muted">{t('settings.role')}</dt>
            <dd className="text-strong">{caps.data?.role ?? '—'}</dd>
          </dl>
        </div>

        {caps.data?.authProvider === 'oidc' ? (
          <div className="panel rounded-xl p-5">
            <p className="text-sm text-muted">{t('settings.ssoAccount', { name: caps.data.authProviderName ?? 'SSO' })}</p>
          </div>
        ) : (
        <form onSubmit={submit} className="panel space-y-3 rounded-xl p-5">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-strong">
            <Icon name="lock" className="h-4 w-4 text-accent" />
            {t('pw.submit')}
          </h3>
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
          {mismatch && (
            <p className="text-sm text-red-600 dark:text-red-400">{t('pw.mismatch')}</p>
          )}
          <button type="submit" disabled={!canSubmit} className="btn-primary disabled:opacity-50">
            {busy ? t('pw.busy') : t('pw.submit')}
          </button>
        </form>
        )}
        </div>
        {caps.data?.role === 'admin' ? (
          <div className="space-y-5">
            <AccountsPanel me={caps.data.user ?? ''} />
            <ClustersPanel />
          </div>
        ) : (
          <p className="text-xs text-muted">{t('settings.askAdmin')}</p>
        )}
      </div>
      <p className="text-xs text-faint">{t('settings.version', { version: caps.data?.appVersion ?? 'dev' })}</p>
    </div>
  )
}

// Admin-only multi-cluster management. Register with any kubeconfig (pasted or loaded from a file,
// a whole ~/.kube/config is fine): pick a context, and by default Sealane uses those credentials once
// to create its own limited account on the cluster and stores that account's token instead.
const clusterName = (context: string) =>
  context.replace(/[^-._a-zA-Z0-9]/g, '-').replace(/^[^a-zA-Z0-9]+/, '').slice(0, 64)

function kubeconfigContexts(text: string): { names: string[]; current: string } {
  try {
    const doc = yamlLoad(text) as { contexts?: { name?: string }[]; 'current-context'?: string } | null
    const names = (doc?.contexts ?? []).map((c) => c.name ?? '').filter(Boolean)
    return { names, current: doc?.['current-context'] ?? names[0] ?? '' }
  } catch {
    return { names: [], current: '' }
  }
}

function ClustersPanel() {
  const toast = useToast()
  const clusters = useClusters(true)
  const [name, setName] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [kubeconfig, setKubeconfig] = useState('')
  const [picked, setPicked] = useState('')
  const [setup, setSetup] = useState(true)
  const [busy, setBusy] = useState(false)
  const [deleting, setDeleting] = useState<Cluster | null>(null)
  const file = useRef<HTMLInputElement>(null)
  const contexts = useMemo(() => kubeconfigContexts(kubeconfig), [kubeconfig])
  const context = contexts.names.includes(picked) ? picked : contexts.current
  const shownName = nameTouched ? name : clusterName(context)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!shownName || !kubeconfig || busy) return
    setBusy(true)
    try {
      const res = await apiPut<{ connected: boolean; version?: string; error?: string; account?: string }>(
        `/api/clusters/${encodeURIComponent(shownName)}`,
        { kubeconfig, context, setup },
      )
      if (res.connected) {
        toast(
          'success',
          res.account
            ? t('settings.cluster.registeredWithAccount', { name: shownName, version: res.version ?? '', account: res.account })
            : res.version
              ? t('settings.cluster.registered', { name: shownName, version: res.version })
              : t('settings.cluster.registeredNoVersion', { name: shownName }),
        )
      } else {
        toast('error', t('settings.cluster.savedButFailed', { error: res.error ?? '' }))
      }
      setName('')
      setNameTouched(false)
      setKubeconfig('')
      setPicked('')
      void clusters.refetch()
    } catch (err) {
      toast('error', errorText(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (target: string, cleanup: boolean) => {
    await apiDelete(`/api/clusters/${encodeURIComponent(target)}${cleanup ? '?cleanup=true' : ''}`)
    setDeleting(null)
    if (getCluster() === target) {
      setCluster('')
      return
    } // deleted the current cluster → reload on local
    toast('success', t('settings.cluster.deleted', { name: target }))
    void clusters.refetch()
  }

  return (
    <div className="panel space-y-3 rounded-xl p-5">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-strong">
        <Icon name="network" className="h-4 w-4 text-accent" />
        {t('settings.cluster.title')}
      </h3>
      <p className="text-xs text-muted">{t('settings.cluster.intro')}</p>
      {clusters.isError && (
        <LoadError message={errorText(clusters.error)} onRetry={() => void clusters.refetch()} />
      )}
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-200 text-left text-xs text-muted dark:border-slate-700">
            <th className="py-1.5 font-medium">{t('common.name')}</th>
            <th className="py-1.5 font-medium">{t('settings.cluster.status')}</th>
            <th className="py-1.5" />
          </tr>
        </thead>
        <tbody>
          {(clusters.data ?? ([{ name: 'local' }] as Cluster[])).map((c) => (
            <tr key={c.name} className="border-b border-gray-100 last:border-0 dark:border-slate-800">
              <td className="py-1.5 text-strong">
                {c.name}
                {c.name === 'local' && <span className="ml-1 text-xs text-muted">{t('settings.cluster.thisCluster')}</span>}
              </td>
              <td className="py-1.5 text-xs">
                {c.connected === undefined ? null : c.connected ? (
                  <span className="inline-flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> {t('settings.cluster.connected')}
                    {c.version ? ` · ${c.version}` : ''}
                  </span>
                ) : (
                  <span
                    className="inline-flex items-center gap-1.5 text-red-600 dark:text-red-400"
                    title={c.error}
                  >
                    <span className="h-1.5 w-1.5 rounded-full bg-red-500" /> {t('settings.cluster.failed')}
                  </span>
                )}
              </td>
              <td className="py-1.5 text-right">
                <button
                  onClick={() => setDeleting(c)}
                  disabled={c.name === 'local'}
                  className="text-xs text-red-600 hover:underline disabled:cursor-not-allowed disabled:opacity-40 dark:text-red-400"
                >
                  {t('common.delete')}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {deleting && (
        <DeleteClusterDialog cluster={deleting} onDelete={remove} onCancel={() => setDeleting(null)} />
      )}

      <form onSubmit={submit} className="space-y-2 border-t border-gray-200 pt-3 dark:border-slate-700">
        <div className="flex items-center justify-between gap-2">
          <p className="text-xs font-medium text-muted">{t('settings.cluster.formTitle')}</p>
          <button type="button" className="btn-ghost px-2 py-1 text-xs" onClick={() => file.current?.click()}>
            <Icon name="download" className="h-3.5 w-3.5 rotate-180" />
            {t('settings.cluster.loadFile')}
          </button>
          <input
            ref={file}
            type="file"
            className="hidden"
            aria-label={t('settings.cluster.loadFile')}
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) void f.text().then(setKubeconfig)
              e.target.value = ''
            }}
          />
        </div>
        <textarea
          value={kubeconfig}
          onChange={(e) => setKubeconfig(e.target.value)}
          placeholder={t('settings.cluster.kubeconfigPlaceholder')}
          aria-label={t('settings.cluster.kubeconfigLabel')}
          rows={6}
          spellCheck={false}
          className="input-base w-full font-mono text-xs"
        />
        {contexts.names.length > 1 && (
          <label className="block">
            <span className="mb-1 block text-xs text-muted">{t('settings.cluster.context')}</span>
            <select value={context} onChange={(e) => setPicked(e.target.value)} className="input-base w-full">
              {contexts.names.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="block">
          <span className="mb-1 block text-xs text-muted">{t('common.name')}</span>
          <input
            value={shownName}
            onChange={(e) => {
              setName(e.target.value)
              setNameTouched(true)
            }}
            placeholder={t('settings.cluster.namePlaceholder')}
            autoComplete="off"
            className="input-base w-full"
          />
        </label>
        <label className="flex items-start gap-2 text-xs">
          <input
            type="checkbox"
            checked={setup}
            onChange={(e) => setSetup(e.target.checked)}
            className="mt-0.5 rounded border-base text-accent focus:ring-accent"
          />
          <span>
            <span className="font-medium text-strong">{t('settings.cluster.setup')}</span>
            <span className="mt-0.5 block text-muted">
              {setup ? t('settings.cluster.setupHelp') : t('settings.cluster.asIsHelp')}
            </span>
          </span>
        </label>
        <button
          type="submit"
          disabled={!shownName || !kubeconfig || busy}
          className="btn-primary disabled:opacity-50"
        >
          {busy ? t(setup ? 'settings.cluster.settingUp' : 'settings.cluster.connecting') : t('settings.cluster.register')}
        </button>
      </form>
    </div>
  )
}

type Account = { name: string; role: string }

// Admin-only account management: list/add/change role or password/delete (the backend patches the ConfigMap).
function AccountsPanel({ me }: { me: string }) {
  const toast = useToast()
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [loadError, setLoadError] = useState('')
  const [name, setName] = useState('')
  const [role, setRole] = useState('viewer')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const reload = useCallback(() => {
    apiGet<Account[]>('/api/accounts')
      .then((list) => {
        setAccounts(list)
        setLoadError('')
      })
      .catch((err) => setLoadError(errorText(err))) // keep the last list on screen
  }, [])
  useEffect(reload, [reload])

  const exists = accounts?.some((a) => a.name === name) ?? false
  const canSubmit = name !== '' && (password !== '' || exists) && !busy

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!canSubmit) return
    setBusy(true)
    try {
      await apiPut(`/api/accounts/${encodeURIComponent(name)}`, { role, password })
      toast(
        'success',
        exists ? t('settings.account.updated', { name }) : t('settings.account.added', { name }),
      )
      setName('')
      setPassword('')
      setRole('viewer')
      reload()
    } catch (err) {
      toast('error', errorText(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (target: string) => {
    if (!window.confirm(t('settings.account.confirmDelete', { name: target }))) return
    try {
      await apiDelete(`/api/accounts/${encodeURIComponent(target)}`)
      toast('success', t('settings.account.deleted', { name: target }))
      reload()
    } catch (err) {
      toast('error', errorText(err))
    }
  }

  return (
    <div className="panel space-y-3 rounded-xl p-5">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-strong">
        <Icon name="user" className="h-4 w-4 text-accent" />
        {t('settings.account.title')}
      </h3>
      {loadError && <LoadError message={loadError} onRetry={reload} />}
      {accounts === null ? (
        !loadError && <p className="text-sm text-muted">{t('common.loading')}</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-200 text-left text-xs text-muted dark:border-slate-700">
              <th className="py-1.5 font-medium">{t('login.username')}</th>
              <th className="py-1.5 font-medium">{t('settings.role')}</th>
              <th className="py-1.5" />
            </tr>
          </thead>
          <tbody>
            {accounts.map((a) => (
              <tr key={a.name} className="border-b border-gray-100 last:border-0 dark:border-slate-800">
                <td className="py-1.5 text-strong">
                  {a.name}
                  {a.name === me && <span className="ml-1 text-xs text-muted">{t('settings.account.me')}</span>}
                </td>
                <td className="py-1.5">{a.role}</td>
                <td className="py-1.5 text-right">
                  <button
                    onClick={() => remove(a.name)}
                    disabled={a.name === me || a.name === 'admin'}
                    className="text-xs text-red-600 hover:underline disabled:cursor-not-allowed disabled:opacity-40 dark:text-red-400"
                  >
                    {t('common.delete')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <form onSubmit={submit} className="space-y-2 border-t border-gray-200 pt-3 dark:border-slate-700">
        <p className="text-xs font-medium text-muted">{t('settings.account.formTitle')}</p>
        <div className="flex gap-2">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('login.username')}
            autoComplete="off"
            className="input-base min-w-0 flex-1"
          />
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            className="input-base !w-32 shrink-0"
          >
            <option value="admin">admin</option>
            <option value="editor">editor</option>
            <option value="viewer">viewer</option>
          </select>
        </div>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder={exists ? t('settings.account.passwordKeep') : t('settings.account.passwordNew')}
          autoComplete="new-password"
          className="input-base w-full"
        />
        <button type="submit" disabled={!canSubmit} className="btn-primary disabled:opacity-50">
          {busy ? t('settings.account.saving') : exists ? t('settings.account.update') : t('settings.account.add')}
        </button>
      </form>
    </div>
  )
}

// A list that failed to load says so, with the reason and a retry, instead of looking empty.
function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex items-center justify-between gap-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-500/10 dark:text-red-300">
      <span>{t('settings.loadFailed', { message })}</span>
      <button type="button" onClick={onRetry} className="btn-ghost shrink-0 px-2 py-1 text-xs">
        {t('common.retry')}
      </button>
    </div>
  )
}

// Deleting a cluster forgets its kubeconfig. A cluster registered with Sealane's own account can also
// have that account removed, which revokes its token; pasted credentials stay valid either way.
function DeleteClusterDialog({
  cluster,
  onDelete,
  onCancel,
}: {
  cluster: Cluster
  onDelete: (name: string, cleanup: boolean) => Promise<void>
  onCancel: () => void
}) {
  const [cleanup, setCleanup] = useState(!!cluster.dedicated)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const confirm = async () => {
    setBusy(true)
    setError('')
    try {
      await onDelete(cluster.name, cleanup)
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }
  return (
    <Modal label={t('settings.cluster.confirmDelete', { name: cluster.name })} onCancel={onCancel} busy={busy}>
      <div className="panel mx-auto w-full max-w-md space-y-3 rounded-xl p-6 shadow-xl">
        <h3 className="text-base font-semibold text-strong">{t('settings.cluster.confirmDelete', { name: cluster.name })}</h3>
        {cluster.dedicated ? (
          <label className="flex items-start gap-2 text-sm">
            <input type="checkbox" className="mt-0.5" checked={cleanup} onChange={(e) => setCleanup(e.target.checked)} />
            <span>
              {t('settings.cluster.cleanup')}
              <span className="mt-0.5 block text-xs text-muted">{t('settings.cluster.cleanupHelp')}</span>
            </span>
          </label>
        ) : (
          <p className="text-sm text-muted">{t('settings.cluster.deleteKeepsAccess')}</p>
        )}
        {cluster.dedicated && !cleanup && <p className="text-xs text-muted">{t('settings.cluster.deleteKeepsAccount')}</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2">
          <button type="button" onClick={onCancel} disabled={busy} className="btn-ghost text-sm">
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onClick={() => void confirm()}
            disabled={busy}
            className="rounded bg-red-600 px-3 py-1.5 text-sm text-white hover:bg-red-700 disabled:opacity-40"
          >
            {t('common.delete')}
          </button>
        </div>
      </div>
    </Modal>
  )
}
