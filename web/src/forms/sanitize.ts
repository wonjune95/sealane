/* eslint-disable @typescript-eslint/no-explicit-any */
import { fieldHelp } from './fieldHelp'
import { fieldExample } from './fieldExample'
import type { RefKind } from '../api/references'

// Field names that reference other cluster resources → render with ReferenceWidget
// (dropdown + existence warning). `host`/`subset` are scalars; `gateways` is an
// array of strings so the widget goes on its items. `hosts` (plural match-hosts) is
// deliberately excluded — those are arbitrary domains, not service refs.
const REF_FIELD: Record<string, RefKind> = {
  host: 'service',
  subset: 'subset',
  gateways: 'gateway',
  gateway: 'gateway',
}

// Arrays of reference objects where the referencing field is `name` inside each
// item ("name" alone is too generic to map globally). Covers the Gateway API
// kinds that use the auto-form (GRPC/TCP/TLS routes — HTTPRoute has a curated form).
const NESTED_REF: Record<string, RefKind> = {
  parentRefs: 'gatewayName', // bare name — ns goes in the sibling namespace field
  backendRefs: 'service',
}

// Fields that are optional in the API but shouldn't be skipped thoughtlessly —
// rendered with a "(recommended)" badge instead of "(optional)" (e.g. exportTo: the default
// is export-to-all-namespaces, which is rarely what a multi-tenant cluster wants).
const RECOMMENDED_FIELD = new Set(['exportTo'])

// Istio CRD schemas mark `required` on nested objects but never at the spec top
// level, so without help every top-level field renders "(optional)". This overlay
// injects the practically-required fields (same judgment as the YAML starter
// templates' "(required)" comments); nested levels keep the CRD's own `required`.
const REQUIRED_TOP: Record<string, string[]> = {
  'virtualservices.networking.istio.io': ['hosts'],
  'destinationrules.networking.istio.io': ['host'],
  'gateways.networking.istio.io': ['selector', 'servers'],
  'serviceentries.networking.istio.io': ['hosts', 'ports'],
}

// Transforms a K8s openAPIV3Schema into something rjsf can render without
// crashing. K8s/Istio schemas carry several constructs rjsf chokes on; we strip
// or down-convert them, accepting looser client validation (the server dry-run is
// the real validator). English descriptions are dropped (Istio docs) and replaced
// with curated Korean ui:help on known fields.
export function sanitizeSchema(input: any, typeId = ''): { jsonSchema: any; uiSchema: any } {
  const jsonSchema = structuredClone(input ?? { type: 'object' })
  const uiSchema: any = {}
  walk(jsonSchema, uiSchema, typeId)
  const extra = REQUIRED_TOP[typeId]?.filter((f) => jsonSchema.properties?.[f])
  if (extra?.length) jsonSchema.required = [...new Set([...(jsonSchema.required ?? []), ...extra])]
  return { jsonSchema, uiSchema }
}

function walk(node: any, ui: any, typeId: string): void {
  if (!node || typeof node !== 'object') return

  // int-or-string → plain string input (avoid oneOf, which rjsf renders poorly)
  if (node['x-kubernetes-int-or-string']) {
    for (const k of Object.keys(node)) delete node[k]
    node.type = 'string'
    return
  }
  // unbounded object → free JSON in a textarea
  if (node['x-kubernetes-preserve-unknown-fields']) {
    for (const k of Object.keys(node)) delete node[k]
    node.type = 'string'
    ui['ui:widget'] = 'textarea'
    return
  }

  // Drop keywords rjsf/AJV can't render or that clutter the UI.
  delete node.nullable // OpenAPI-only; not JSON Schema draft-07
  delete node.description // Istio English docs — field names remain
  // K8s "exactly one of" constraints use oneOf/anyOf/not with empty variants
  // that crash rjsf's combinator selector. Strip them — server dry-run still
  // enforces the real rule.
  delete node.oneOf
  delete node.anyOf
  delete node.allOf
  delete node.not
  for (const k of Object.keys(node)) {
    if (k.startsWith('x-kubernetes-')) delete node[k]
  }
  if (node.additionalProperties === true) delete node.additionalProperties

  if (node.properties && typeof node.properties === 'object') {
    for (const key of Object.keys(node.properties)) {
      const childUi: any = {}
      walk(node.properties[key], childUi, typeId)
      const help = fieldHelp(typeId, key)
      if (help) childUi['ui:help'] = help
      // ref widget + example placeholder both target the input — which for an
      // array-of-strings field is each item, not the array container.
      const refKind = REF_FIELD[key]
      const example = fieldExample(typeId, key)
      if (refKind || example) {
        const extra: any = {}
        if (refKind) { extra['ui:widget'] = 'reference'; extra['ui:options'] = { refKind } }
        if (example) extra['ui:placeholder'] = example
        if (node.properties[key]?.type === 'array') childUi.items = { ...childUi.items, ...extra }
        else Object.assign(childUi, extra)
      }
      if (RECOMMENDED_FIELD.has(key)) {
        childUi['ui:options'] = { ...childUi['ui:options'], recommended: true }
      }
      const nestedRef = NESTED_REF[key]
      if (nestedRef && node.properties[key]?.type === 'array') {
        childUi.items = {
          ...childUi.items,
          name: { ...childUi.items?.name, 'ui:widget': 'reference', 'ui:options': { refKind: nestedRef } },
        }
      }
      if (Object.keys(childUi).length > 0) ui[key] = childUi
    }
  }
  if (node.items && typeof node.items === 'object') {
    const itemUi: any = {}
    walk(node.items, itemUi, typeId)
    if (Object.keys(itemUi).length > 0) ui.items = itemUi
  }
  // additionalProperties as a schema = map type (e.g. labels) → recurse
  if (node.additionalProperties && typeof node.additionalProperties === 'object') {
    walk(node.additionalProperties, {}, typeId)
  }
}
