import { t } from '../i18n'

export function Guard() {
  return (
    <div className="panel mx-auto max-w-2xl p-8 shadow-sm">
      <h2 className="mb-2 text-xl font-semibold text-strong">{t('guard.title')}</h2>
      <p className="mb-4 text-muted">
        {t('guard.introBefore')} <code className="rounded bg-gray-100 px-1 dark:bg-slate-800">HTTPRoute</code>
        {t('guard.introMiddle')} <code className="rounded bg-gray-100 px-1 dark:bg-slate-800">VirtualService</code>
        {t('guard.introAfter')}
      </p>
      <pre className="overflow-x-auto rounded bg-gray-900 p-4 text-sm leading-relaxed text-gray-100 dark:bg-slate-950">
{`# Gateway API CRDs (HTTPRoute)
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

${t('guard.orIstio')}
istioctl install`}
      </pre>
      <p className="mt-4 text-sm text-muted">{t('guard.reload')}</p>
    </div>
  )
}
