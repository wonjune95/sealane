import { Link, useParams } from 'react-router-dom'
import { useServiceMesh } from '../api/mesh'
import { useCapabilities } from '../api/capabilities'
import type { Ref } from '../api/analyze'
import { PageHeader } from '../components/PageHeader'
import { FindingList } from '../components/FindingList'
import { KindBadge } from '../components/KindBadge'
import { Icon } from '../components/icons'
import { t } from '../i18n'

const SCOPE = {
  workload: t('services.scope.workload'),
  namespace: t('services.scope.namespace'),
  mesh: t('services.scope.mesh'),
}

function Group({ title, items }: { title: string; items: (Ref & { tag?: string })[] }) {
  return (
    <section className="panel">
      <h2 className="border-b border-base px-3 py-2 text-sm font-semibold">{title}</h2>
      {items.length === 0 ? (
        <p className="px-3 py-2 text-sm text-faint">{t('services.none')}</p>
      ) : (
        <ul className="divide-y divide-slate-100 dark:divide-slate-800">
          {items.map((r) => (
            <li key={`${r.typeId}/${r.namespace}/${r.name}`} className="flex items-center gap-2 px-3 py-1.5 text-sm">
              <KindBadge kind={r.kind} />
              <Link to={`/resources/${r.typeId}/${r.namespace || '-'}/${r.name}`} className="font-medium text-accent hover:underline">
                {r.name}
              </Link>
              {r.namespace && <span className="text-muted">{r.namespace}</span>}
              {r.tag && <span className="ml-auto rounded-full border border-base px-2 py-0.5 text-xs text-muted">{r.tag}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function ServiceDetail() {
  const { ns = '', name = '' } = useParams()
  const { data, isLoading, error } = useServiceMesh(ns, name)
  const caps = useCapabilities().data
  const canWrite = (caps?.role === 'admin' || caps?.role === 'editor') && !!caps?.virtualServiceInstalled
  const port = data?.ports[0]
  const q = `new?ns=${encodeURIComponent(ns)}&host=${encodeURIComponent(name)}${port ? `&port=${port}` : ''}`

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <PageHeader
        eyebrow={t('eyebrow.service')}
        title={name}
        description={
          data &&
          `${ns} · ${data.ports.join(', ') || '—'} · ${t('services.endpoints', { n: data.endpoints })}`
        }
        actions={
          <>
            {canWrite && (
              <>
                <Link to={`/resources/virtualservices.networking.istio.io/${q}`} className="btn-ghost">
                  {t('services.createVS')}
                </Link>
                <Link to={`/resources/destinationrules.networking.istio.io/${q}`} className="btn-ghost">
                  {t('services.createDR')}
                </Link>
              </>
            )}
            <Link to="/services" className="btn-ghost">
              <Icon name="back" className="h-4 w-4" /> {t('services.back')}
            </Link>
          </>
        }
      />
      {error && (
        <div className="flex items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-600 dark:text-red-400" role="alert">
          <Icon name="warning" />
          {t('services.loadFailed', { message: error.message })}
        </div>
      )}
      {isLoading && (
        <div className="p-12 text-center text-sm text-muted" role="status">
          {t('common.loading')}
        </div>
      )}
      {data && (
        <>
          <h2 className="text-sm font-semibold">{t('services.findings')}</h2>
          <FindingList findings={data.findings} emptyTitle={t('services.noFindings')} emptyHint="" />
          <Group
            title={t('services.virtualServices')}
            items={data.virtualServices.map((r) => ({
              ...r,
              tag: t(r.match === 'host' ? 'services.matchHost' : 'services.matchDestination'),
            }))}
          />
          <Group title={t('services.destinationRules')} items={data.destinationRules} />
          <Group title={t('services.httpRoutes')} items={data.httpRoutes} />
          <Group title={t('services.gateways')} items={data.gateways} />
          <Group
            title={t('services.policies')}
            items={data.policies.map((r) => ({ ...r, tag: SCOPE[r.scope] }))}
          />
        </>
      )}
    </div>
  )
}
