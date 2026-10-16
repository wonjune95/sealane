import { useCallback, useEffect, useState } from 'react'
import { useCapabilities } from '../api/capabilities'
import { useClusters, type Cluster } from '../api/clusters'
import { apiDelete, apiGet, apiPost, apiPut, errorText, getCluster, setCluster } from '../api/client'
import { useToast } from '../components/Toast'
import { PageHeader } from '../components/PageHeader'
import { Icon } from '../components/icons'
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

// Admin-only multi-cluster management: register by pasting a kubeconfig (the backend stores it in a Secret).
function ClustersPanel() {
  const toast = useToast()
  const clusters = useClusters(true)
  const [name, setName] = useState('')
  const [kubeconfig, setKubeconfig] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name || !kubeconfig || busy) return
    setBusy(true)
    try {
      const res = await apiPut<{ connected: boolean; version?: string; error?: string }>(
        `/api/clusters/${encodeURIComponent(name)}`,
        { kubeconfig },
      )
      if (res.connected) {
        toast(
          'success',
          res.version
            ? t('settings.cluster.registered', { name, version: res.version })
            : t('settings.cluster.registeredNoVersion', { name }),
        )
      } else {
        toast('error', t('settings.cluster.savedButFailed', { error: res.error ?? '' }))
      }
      setName('')
      setKubeconfig('')
      void clusters.refetch()
    } catch (err) {
      toast('error', errorText(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (target: string) => {
    if (!window.confirm(t('settings.cluster.confirmDelete', { name: target }))) return
    try {
      await apiDelete(`/api/clusters/${encodeURIComponent(target)}`)
      if (getCluster() === target) {
        setCluster('')
        return
      } // deleted the current cluster → reload on local
      toast('success', t('settings.cluster.deleted', { name: target }))
      void clusters.refetch()
    } catch (err) {
      toast('error', errorText(err))
    }
  }

  return (
    <div className="panel space-y-3 rounded-xl p-5">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-strong">
        <Icon name="network" className="h-4 w-4 text-accent" />
        {t('settings.cluster.title')}
      </h3>
      <p className="text-xs text-muted">{t('settings.cluster.intro')}</p>
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
                  onClick={() => remove(c.name)}
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

      <form onSubmit={submit} className="space-y-2 border-t border-gray-200 pt-3 dark:border-slate-700">
        <p className="text-xs font-medium text-muted">{t('settings.cluster.formTitle')}</p>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('settings.cluster.namePlaceholder')}
          autoComplete="off"
          className="input-base w-full"
        />
        <textarea
          value={kubeconfig}
          onChange={(e) => setKubeconfig(e.target.value)}
          placeholder={t('settings.cluster.kubeconfigPlaceholder')}
          rows={6}
          spellCheck={false}
          className="input-base w-full font-mono text-xs"
        />
        <button
          type="submit"
          disabled={!name || !kubeconfig || busy}
          className="btn-primary disabled:opacity-50"
        >
          {busy ? t('settings.cluster.connecting') : t('settings.cluster.register')}
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
  const [name, setName] = useState('')
  const [role, setRole] = useState('viewer')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const reload = useCallback(() => {
    apiGet<Account[]>('/api/accounts')
      .then(setAccounts)
      .catch((err) => {
        toast('error', errorText(err))
      })
  }, [toast])
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
      {accounts === null ? (
        <p className="text-sm text-muted">{t('common.loading')}</p>
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
