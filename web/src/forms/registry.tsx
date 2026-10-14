import type { ReactNode } from 'react'
import type { GatewayRef } from '../api/gateways'
import type { ServiceRef } from '../api/services'
import { HTTPRouteForm } from './HTTPRouteForm'
import { isHTTPRepresentable, toHTTPForm, fromHTTPForm } from './httproute'
import { VirtualServiceForm } from './VirtualServiceForm'
import { isVSRepresentable, toVSForm, fromVSForm } from './virtualservice'
import { DestinationRuleForm } from './DestinationRuleForm'
import { isDRRepresentable, toDRForm, fromDRForm } from './destinationrule'

/* eslint-disable @typescript-eslint/no-explicit-any */

export interface FormRenderProps {
  model: any
  onChange: (m: any) => void
  gateways: GatewayRef[]
  services: ServiceRef[]
  lockIdentity: boolean
}

// A FormIntegration adds a structured Form tab for one kind. Kinds without an
// entry are YAML-only (the sync guard already handles this).
export interface FormIntegration {
  isRepresentable: (obj: any) => boolean
  toModel: (obj: any) => any
  fromModel: (base: any, model: any) => any
  needsGateways?: boolean
  needsServices?: boolean
  render: (p: FormRenderProps) => ReactNode
}

export const FORMS: Record<string, FormIntegration> = {
  'httproutes.gateway.networking.k8s.io': {
    isRepresentable: isHTTPRepresentable,
    toModel: toHTTPForm,
    fromModel: fromHTTPForm,
    needsGateways: true,
    needsServices: true,
    render: (p) => (
      <HTTPRouteForm
        model={p.model}
        onChange={p.onChange}
        gateways={p.gateways}
        services={p.services}
        lockIdentity={p.lockIdentity}
      />
    ),
  },
  'virtualservices.networking.istio.io': {
    isRepresentable: isVSRepresentable,
    toModel: toVSForm,
    fromModel: fromVSForm,
    needsGateways: true,
    needsServices: true,
    render: (p) => (
      <VirtualServiceForm
        model={p.model}
        onChange={p.onChange}
        gateways={p.gateways}
        services={p.services}
        lockIdentity={p.lockIdentity}
      />
    ),
  },
  'destinationrules.networking.istio.io': {
    isRepresentable: isDRRepresentable,
    toModel: toDRForm,
    fromModel: fromDRForm,
    needsServices: true,
    render: (p) => (
      <DestinationRuleForm
        model={p.model}
        onChange={p.onChange}
        services={p.services}
        lockIdentity={p.lockIdentity}
      />
    ),
  },
}
