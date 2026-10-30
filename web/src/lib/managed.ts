import type { ManagedBy } from '../api/resources'
import { t } from '../i18n'

const TOOLS: Record<ManagedBy['tool'], string> = { argocd: 'Argo CD', flux: 'Flux', helm: 'Helm' }

export const managedTool = (m: ManagedBy) => TOOLS[m.tool]

export const managedText = (m: ManagedBy) =>
  t(m.tool === 'helm' ? 'managed.helm' : 'managed.gitops', { tool: TOOLS[m.tool], name: m.name })
