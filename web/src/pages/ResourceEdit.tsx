import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams, Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { load as yamlLoad, dump as yamlDump } from 'js-yaml'
import { useResource, type ResourceDetail } from '../api/resources'
import { useResourceTypes, type ResourceType } from '../api/resourceTypes'
import { useGateways } from '../api/gateways'
import { useServices } from '../api/services'
import { useAccess } from '../api/access'
import { apiGet, apiPost, apiPut, apiDelete, ApiError } from '../api/client'
import { YamlEditor } from '../components/YamlEditor'
import { DangerConfirm } from '../components/DangerConfirm'
import { DiffModal } from '../components/DiffModal'
import { cleanManifest } from '../lib/download'
import { findFocusLine } from '../lib/yamlFocus'
import { FORMS, type FormIntegration } from '../forms/registry'
import { AutoForm } from '../forms/AutoForm'
import { useToast } from '../components/Toast'
import { KindBadge } from '../components/KindBadge'
import { PageHeader } from '../components/PageHeader'
import { Icon } from '../components/icons'
import { t } from '../i18n'

/* eslint-disable @typescript-eslint/no-explicit-any */

// Starter templates with localized comments shown in the YAML tab for new resources.
// (Editing via the Form tab re-serializes and drops comments — Gemini ④.)
// A function so the comments are translated at use, not at module load.
const richTemplates = (): Record<string, string> => ({
  'httproutes.gateway.networking.k8s.io': `apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
  namespace: default
spec:
  parentRefs:            # ${t('edit.tpl.hrParentRefs')}
    - name: demo-gw
  hostnames:             # ${t('edit.tpl.hrHostnames')}
    - example.com
  rules:
    - matches:           # ${t('edit.tpl.hrMatches')}
        - path:
            type: PathPrefix   # PathPrefix | Exact
            value: /
      backendRefs:       # ${t('edit.tpl.hrBackendRefs')}
        - name: my-svc
          port: 80
`,
  'virtualservices.networking.istio.io': `apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: my-virtualservice
  namespace: default
spec:
  hosts:                 # ${t('edit.tpl.vsHosts')}
    - ''
  # gateways:            # ${t('edit.tpl.vsGateways')}
  #   - my-gateway
  http:                  # ${t('edit.tpl.vsHttp')}
    - route:
        - destination:
            host: ''     # ${t('edit.tpl.vsHost')}
            port:
              number: 80
          # weight: 100  # ${t('edit.tpl.vsWeight')}
`,
  'destinationrules.networking.istio.io': `apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: my-destinationrule
  namespace: default
spec:
  host: ''               # ${t('edit.tpl.drHost')}
  # trafficPolicy:       # ${t('edit.tpl.drTrafficPolicy')}
  #   loadBalancer:
  #     simple: ROUND_ROBIN
  subsets:               # ${t('edit.tpl.drSubsets')}
    - name: v1
      labels:
        version: v1
`,
  'gateways.networking.istio.io': `apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: default
spec:
  selector:              # ${t('edit.tpl.gwSelector')}
    istio: ingressgateway
  servers:               # ${t('edit.tpl.gwServers')}
    - port:
        number: 80
        name: http
        protocol: HTTP
      hosts:
        - ''             # ${t('edit.tpl.gwHosts')}
`,
  'serviceentries.networking.istio.io': `apiVersion: networking.istio.io/v1
kind: ServiceEntry
metadata:
  name: my-serviceentry
  namespace: default
spec:
  hosts:                 # ${t('edit.tpl.seHosts')}
    - ''
  ports:                 # ${t('edit.tpl.sePorts')}
    - number: 443
      name: https
      protocol: TLS
  resolution: DNS        # DNS | STATIC | NONE
  location: MESH_EXTERNAL  # ${t('edit.tpl.seLocation')}
`,
})

function genericTemplate(rt: ResourceType): string {
  const obj: any = {
    apiVersion: `${rt.group}/${rt.version}`,
    kind: rt.kind,
    metadata: rt.namespaced
      ? { name: `my-${rt.kind.toLowerCase()}`, namespace: 'default' }
      : { name: `my-${rt.kind.toLowerCase()}` },
    spec: {},
  }
  return yamlDump(obj)
}

type Tab = 'form' | 'yaml'

// Mid-edit YAML can have a list typed as a map (`http: {route: []}`); the
// representability checks assume the schema shape and may throw — treat that
// as "not representable" instead of unmounting the editor with unsaved text.
function safeRepresentable(curated: FormIntegration, obj: any): boolean {
  try {
    return curated.isRepresentable(obj)
  } catch {
    return false
  }
}

export function ResourceEdit() {
  const { type = '', ns: nsParam, name: nameParam } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()

  const types = useResourceTypes()
  const rt = types.data?.find((x) => x.typeId === type)
  const curated = FORMS[type]
  // Auto-form applies when there's no curated form but the kind is schema-capable.
  const hasAuto = !curated && !!rt?.formCapable

  const isEdit = Boolean(nsParam && nameParam)
  const ns = nsParam ?? ''
  const name = nameParam ?? ''
  const resource = useResource(type, ns, name, isEdit)
  // ?focus=<path>&value=<v>: a finding pointing at a field (e.g. spec.http[0].route[0].destination.subset)
  const [searchParams] = useSearchParams()
  const focusPath = searchParams.get('focus')
  const focusValue = searchParams.get('value') ?? undefined

  const [text, setText] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('yaml')
  const [focus, setFocus] = useState<{ path: string; line: number | null } | null>(null)
  const [busy, setBusy] = useState(false)
  const [pending, setPending] = useState<{ action: string; name: string; run: () => void } | null>(null)
  const [preview, setPreview] = useState<{ oldYaml: string; newYaml: string } | null>(null)
  const [conflict, setConflict] = useState<{ mineYaml: string; latestYaml: string; latestRV: string } | null>(
    null,
  )

  /* eslint-disable react-hooks/set-state-in-effect --
     one-time editor init: text/tab can only be seeded after the resource query
     resolves, and the `text !== null` guard makes this run exactly once. */
  useEffect(() => {
    if (text !== null || !rt) return
    let initial: string
    if (!isEdit) initial = richTemplates()[type] ?? genericTemplate(rt)
    else if (resource.data) initial = yamlDump(resource.data.raw)
    else return
    setText(initial)
    if (curated) {
      try {
        if (curated.isRepresentable(yamlLoad(initial))) setTab('form')
      } catch {
        /* stay yaml */
      }
    } else if (hasAuto) {
      setTab('form')
    }
    if (focusPath) {
      setTab('yaml')
      setFocus({ path: focusPath, line: findFocusLine(initial, focusPath, focusValue) })
    }
  }, [isEdit, type, rt, resource.data, text, curated, hasAuto, focusPath, focusValue])
  /* eslint-enable react-hooks/set-state-in-effect */

  let obj: any = null
  let parseErr: string | null = null
  if (text !== null) {
    try {
      obj = yamlLoad(text)
    } catch (e) {
      parseErr = (e as Error).message
    }
  }
  // Curated form requires a representable object; auto-form handles any spec.
  const formAvailable = !parseErr && !!obj && (curated ? safeRepresentable(curated, obj) : hasAuto)
  const formNs = obj?.metadata?.namespace || 'default'
  const gateways = useGateways(curated?.needsGateways && tab === 'form' ? formNs : '')
  const services = useServices(curated?.needsServices && tab === 'form' ? formNs : '')

  // Permission-aware UI: SSAR for the verbs this page can perform. When the user
  // lacks the verb, the form/buttons go read-only instead of failing on apply.
  const access = useAccess(
    type,
    isEdit ? ns : formNs,
    isEdit ? name : '',
    isEdit ? 'update,delete' : 'create',
    !!rt,
  )
  const readOnly = isEdit && access.data?.update === false
  const noDelete = isEdit && access.data?.delete === false
  const noCreate = !isEdit && access.data?.create === false
  const cantApply = readOnly || noCreate

  // A read-only resource forces the YAML tab (editor locked) — no point showing a form.
  const shownTab: Tab = readOnly && tab === 'form' ? 'yaml' : tab

  function switchTab(next: Tab) {
    if (next === 'form' && !formAvailable) {
      toast(
        'info',
        parseErr
          ? t('edit.parseErrorNoForm')
          : !curated && !hasAuto
            ? t('edit.noForm', { kind: rt?.kind ?? type })
            : t('edit.advancedYamlOnly'),
      )
      return
    }
    if (next === 'form') setFocus(null) // the form re-serializes the YAML; the line would go stale
    setTab(next)
  }

  // dangerous kinds require typed confirmation before a real mutation.
  function guard(action: string, targetName: string, run: () => void) {
    if (rt?.dangerous) setPending({ action, name: targetName, run })
    else run()
  }

  // Pre-check shared by the form and YAML apply paths: parse + name/namespace required.
  function parseBody(): any | null {
    let body: any
    try {
      body = yamlLoad(text ?? '')
    } catch (e) {
      toast('error', t('edit.parseError', { message: (e as Error).message }))
      return null
    }
    if (!body?.metadata?.name) {
      toast('error', t('edit.nameRequired'))
      return null
    }
    if (rt?.namespaced && !body.metadata.namespace) {
      toast('error', t('edit.namespaceRequired'))
      return null
    }
    return body
  }

  async function doApply(dryRun: boolean) {
    const body = parseBody()
    if (body === null) return
    setBusy(true)
    try {
      const q = `?dryRun=${dryRun}`
      if (isEdit) await apiPut(`/api/resources/${type}/${ns}/${name}${q}`, body)
      else await apiPost(`/api/resources/${type}${q}`, body)
      if (dryRun) toast('success', t('edit.validated'))
      else {
        toast('success', isEdit ? t('edit.updated') : t('edit.created'))
        queryClient.invalidateQueries({ queryKey: ['resources', type] })
        queryClient.invalidateQueries({ queryKey: ['audit'] })
        navigate(`/resources/${type}`)
      }
    } catch (e) {
      // 409 = someone else changed the object since we loaded it. Don't silently
      // clobber: show "mine vs latest" diff and let the user decide (Gemini gap #2).
      if (!dryRun && isEdit && e instanceof ApiError && e.status === 409) {
        try {
          const latest = await apiGet<ResourceDetail>(`/api/resources/${type}/${ns}/${name}`)
          setConflict({
            mineYaml: yamlDump(cleanManifest(body)),
            latestYaml: yamlDump(cleanManifest(latest.raw)),
            latestRV: latest.resourceVersion,
          })
          return
        } catch {
          /* fall through to generic error */
        }
      }
      toast('error', e instanceof ApiError ? `${e.status} ${e.reason}: ${e.message}` : String(e))
    } finally {
      setBusy(false)
    }
  }

  // Re-apply my edits on top of the latest serverside version (last-write-wins
  // after a human reviewed the diff). Grafts the fresh resourceVersion in.
  async function forceOverwrite(latestRV: string) {
    const body = parseBody()
    if (body === null) return
    if (body.metadata) body.metadata.resourceVersion = latestRV
    setBusy(true)
    try {
      await apiPut(`/api/resources/${type}/${ns}/${name}`, body)
      toast('success', t('edit.overwritten'))
      queryClient.invalidateQueries({ queryKey: ['resources', type] })
      queryClient.invalidateQueries({ queryKey: ['audit'] })
      navigate(`/resources/${type}`)
    } catch (e) {
      toast('error', e instanceof ApiError ? `${e.status} ${e.reason}: ${e.message}` : String(e))
    } finally {
      setBusy(false)
    }
  }

  function apply(dryRun: boolean) {
    if (dryRun) {
      void doApply(true)
      return
    } // "Validate" button: validate only
    void openPreview() // "Apply" button: dry-run → diff preview → confirm
  }

  // Runs a dry-run to get the would-be-applied object, then opens a diff preview
  // (kubectl-diff style) against the current object before the real apply.
  async function openPreview() {
    const body = parseBody()
    if (body === null) return
    setBusy(true)
    try {
      const q = '?dryRun=true'
      const result = isEdit
        ? await apiPut<ResourceDetail>(`/api/resources/${type}/${ns}/${name}${q}`, body)
        : await apiPost<ResourceDetail>(`/api/resources/${type}${q}`, body)
      setPreview({
        oldYaml: isEdit && resource.data ? yamlDump(cleanManifest(resource.data.raw)) : '',
        newYaml: yamlDump(cleanManifest(result.raw)),
      })
    } catch (e) {
      toast('error', e instanceof ApiError ? `${e.status} ${e.reason}: ${e.message}` : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function doDelete() {
    setBusy(true)
    try {
      await apiDelete(`/api/resources/${type}/${ns}/${name}`)
      toast('success', t('list.deleted'))
      queryClient.invalidateQueries({ queryKey: ['resources', type] })
      queryClient.invalidateQueries({ queryKey: ['audit'] })
      navigate(`/resources/${type}`)
    } catch (e) {
      toast('error', e instanceof ApiError ? `${e.status} ${e.reason}: ${e.message}` : String(e))
    } finally {
      setBusy(false)
    }
  }

  function del() {
    if (rt?.dangerous) {
      guard(t('common.delete'), name, () => void doDelete())
      return
    }
    if (window.confirm(t('list.confirmDelete', { kind: rt?.kind ?? type, ns, name }))) void doDelete()
  }

  if (!rt && types.isLoading) return <div className="text-muted">{t('common.loading')}</div>
  if (!rt) return <div className="text-red-600 dark:text-red-400">{t('edit.unknownType', { type })}</div>
  if (isEdit && resource.isLoading) return <div className="text-muted">{t('common.loading')}</div>
  if (isEdit && resource.isError)
    return (
      <div className="text-red-600 dark:text-red-400">
        {t('list.loadFailed', { message: (resource.error as Error).message })}
      </div>
    )
  if (text === null) return <div className="text-muted">{t('edit.preparing')}</div>

  return (
    <div className="resource-editor mx-auto space-y-5">
      <PageHeader
        eyebrow={isEdit ? 'Resource editor' : 'New resource'}
        title={isEdit ? name : t('edit.newTitle', { kind: rt.kind })}
        description={
          <span className="inline-flex flex-wrap items-center gap-2">
            <KindBadge kind={rt.kind} category={rt.category} />
            {isEdit ? ns : t('edit.newDescription')}
            {rt.dangerous && (
              <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
                <Icon name="warning" className="h-3.5 w-3.5" />
                {t('edit.highRisk')}
              </span>
            )}
          </span>
        }
        actions={
          <Link to={`/resources/${type}`} className="btn-ghost">
            <Icon name="back" className="h-4 w-4" />
            {t('edit.backToList')}
          </Link>
        }
      />

      {readOnly && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          {/* readOnlyBefore/After carry their own spaces around the <strong> */}
          {t('edit.readOnlyBefore')}
          <strong>{t('edit.readOnlyStrong')}</strong>
          {t('edit.readOnlyAfter')}
        </div>
      )}
      {noCreate && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          {/* noCreateAfter carries its own leading space after the <strong> */}
          <strong>{t('edit.noCreateStrong', { kind: rt.kind })}</strong>
          {t('edit.noCreateAfter')}
        </div>
      )}

      <div className="flex items-center gap-1 border-b border-base">
        <TabButton
          active={shownTab === 'form'}
          disabled={!formAvailable || readOnly}
          onClick={() => switchTab('form')}
        >
          {t('edit.tabForm')}
        </TabButton>
        <TabButton active={shownTab === 'yaml'} onClick={() => switchTab('yaml')}>
          YAML
        </TabButton>
        {curated && !formAvailable && !parseErr && (
          <span className="ml-3 self-center text-xs text-amber-600 dark:text-amber-400">
            {t('edit.advancedDetected')}
          </span>
        )}
        {shownTab === 'form' && formAvailable && (
          <span className="ml-auto self-center pb-1 text-xs text-faint">
            <span className="text-red-500">*</span> {t('edit.legendRequired')} ·{' '}
            <span className="text-amber-600 dark:text-amber-400">{t('form.recommended')}</span>{' '}
            {t('edit.legendRecommended')} · {t('form.optional')} {t('edit.legendOptional')}
          </span>
        )}
      </div>

      {shownTab === 'form' && formAvailable && curated ? (
        curated.render({
          model: curated.toModel(obj),
          onChange: (m: any) => setText(yamlDump(curated.fromModel(obj, m))),
          gateways: gateways.data ?? [],
          services: services.data ?? [],
          lockIdentity: isEdit,
        })
      ) : shownTab === 'form' && formAvailable && hasAuto ? (
        <AutoForm
          type={type}
          spec={obj?.spec ?? {}}
          ns={formNs}
          onChange={(newSpec: any) => setText(yamlDump({ ...obj, spec: newSpec }))}
          header={
            // The auto-form renders only spec, so name/namespace are collected here (required).
            <div className="mb-5 grid grid-cols-1 sm:grid-cols-2 gap-4 border-b border-base pb-5">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-muted">
                  Name<span className="text-red-500">*</span>
                </span>
                <input
                  className="input-base disabled:bg-gray-100 dark:disabled:bg-slate-800"
                  value={obj?.metadata?.name ?? ''}
                  disabled={isEdit}
                  onChange={(e) =>
                    setText(yamlDump({ ...obj, metadata: { ...obj?.metadata, name: e.target.value } }))
                  }
                />
              </label>
              {rt.namespaced && (
                <label className="block">
                  <span className="mb-1 block text-xs font-medium text-muted">
                    Namespace<span className="text-red-500">*</span>
                  </span>
                  <input
                    className="input-base disabled:bg-gray-100 dark:disabled:bg-slate-800"
                    value={obj?.metadata?.namespace ?? ''}
                    disabled={isEdit}
                    onChange={(e) =>
                      setText(yamlDump({ ...obj, metadata: { ...obj?.metadata, namespace: e.target.value } }))
                    }
                  />
                </label>
              )}
            </div>
          }
        />
      ) : (
        <>
          {focus && (
            <div
              role="status"
              className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm ${
                focus.line
                  ? 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
                  : 'border-red-500/20 bg-red-500/5 text-red-600 dark:text-red-400'
              }`}
            >
              <span className="min-w-0 flex-1 break-all font-mono text-xs">
                {t(focus.line ? 'editor.focusBanner' : 'editor.focusNotFound', { path: focus.path })}
              </span>
              <button onClick={() => setFocus(null)} className="btn-ghost text-xs">
                {t('editor.focusDismiss')}
              </button>
            </div>
          )}
          <YamlEditor value={text} onChange={setText} readOnly={readOnly} focusLine={focus?.line ?? undefined} />
        </>
      )}

      <div className="panel sticky bottom-0 z-10 mt-4 flex flex-wrap items-center gap-2 px-4 py-3 shadow-lg">
        <button
          onClick={() => apply(true)}
          disabled={busy || cantApply}
          className="btn-ghost text-sm disabled:opacity-50"
        >
          <Icon name="check" className="h-4 w-4" />
          {t('edit.validate')}
        </button>
        <button
          onClick={() => apply(false)}
          disabled={busy || cantApply}
          className="btn-primary text-sm disabled:opacity-50"
        >
          <Icon name="plus" className="h-4 w-4" />
          {t('edit.apply')}
        </button>
        {isEdit && (
          <button
            onClick={del}
            disabled={busy || noDelete}
            title={noDelete ? t('edit.noDeletePermission') : undefined}
            className="ml-auto rounded-md border border-red-300 px-3 py-1.5 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-500/40 dark:text-red-400 dark:hover:bg-red-500/10"
          >
            {t('common.delete')}
          </button>
        )}
      </div>

      {pending && (
        <DangerConfirm
          name={pending.name}
          action={pending.action}
          onConfirm={() => {
            const run = pending.run
            setPending(null)
            run()
          }}
          onCancel={() => setPending(null)}
        />
      )}

      {preview && (
        <DiffModal
          oldYaml={preview.oldYaml}
          newYaml={preview.newYaml}
          name={obj?.metadata?.name ?? name}
          dangerous={!!rt.dangerous}
          busy={busy}
          onConfirm={() => {
            setPreview(null)
            void doApply(false)
          }}
          onCancel={() => setPreview(null)}
        />
      )}

      {conflict && (
        <DiffModal
          title={t('edit.conflictTitle')}
          subtitle={t('edit.conflictSubtitle')}
          confirmLabel={t('edit.conflictConfirm')}
          warn
          oldYaml={conflict.latestYaml}
          newYaml={conflict.mineYaml}
          name={obj?.metadata?.name ?? name}
          dangerous={false}
          busy={busy}
          onConfirm={() => {
            const rv = conflict.latestRV
            setConflict(null)
            void forceOverwrite(rv)
          }}
          onCancel={() => setConflict(null)}
        />
      )}
    </div>
  )
}

function TabButton({
  active,
  disabled,
  onClick,
  children,
}: {
  active: boolean
  disabled?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`-mb-px border-b-2 px-4 py-2 text-sm ${active ? 'border-accent font-medium text-accent' : 'border-transparent text-muted hover:text-strong'} disabled:cursor-not-allowed disabled:opacity-40`}
    >
      {children}
    </button>
  )
}
