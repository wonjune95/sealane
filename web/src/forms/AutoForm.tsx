import { useMemo, useState, type ReactNode } from 'react'
import Form from '@rjsf/core'
import { customizeValidator } from '@rjsf/validator-ajv8'
import type { IChangeEvent } from '@rjsf/core'
import { useSchema } from '../api/schema'
import { sanitizeSchema } from './sanitize'
import { CollapsibleObjectField } from './CollapsibleObjectField'
import { FieldTemplate } from './FieldTemplate'
import { ArrayFieldTemplate, ArrayFieldItemTemplate } from './ArrayFieldTemplate'
import { widgets } from './widgets'
import { ErrorBoundary } from '../components/ErrorBoundary'
import { t } from '../i18n'

/* eslint-disable @typescript-eslint/no-explicit-any */

// AJV strict off: K8s schemas carry formats/keywords AJV would otherwise reject
// and crash the form (Gemini review ②).
const validator = customizeValidator({ ajvOptionsOverrides: { strict: false } })

// AutoForm renders a schema-driven form for a kind's spec. rjsf owns the form
// state: formData is seeded once (a stable value, never re-fed) so the parent's
// YAML round-trip can't reset an edit in progress. onChange only flows OUT to the
// parent (for the YAML tab / apply). It remounts when the Form tab reopens,
// re-seeding from the current spec.
export function AutoForm({
  type,
  spec,
  ns,
  onChange,
  header,
}: {
  type: string
  spec: any
  ns: string
  onChange: (spec: any) => void
  header?: ReactNode // metadata inputs (name/namespace) — rendered in the same panel as the spec form
}) {
  const schema = useSchema(type, true)
  const [initial] = useState<any>(() => spec ?? {}) // captured once; never updated
  const prepared = useMemo(() => (schema.data ? sanitizeSchema(schema.data, type) : null), [schema.data, type])

  if (schema.isLoading) return <Box>{t('form.schemaLoading')}</Box>
  if (schema.isError || !prepared) return <Box>{t('form.schemaUnavailable')}</Box>

  return (
    <div className="rjsf-compact panel p-5 shadow-sm">
      {header}
      <ErrorBoundary fallback={<div className="text-sm text-amber-600 dark:text-amber-400">{t('form.renderFailed')}</div>}>
        <Form
          schema={prepared.jsonSchema}
          uiSchema={{ ...prepared.uiSchema, 'ui:submitButtonOptions': { norender: true } }}
          formData={initial}
          formContext={{ ns }}
          validator={validator}
          widgets={widgets}
          templates={{ ObjectFieldTemplate: CollapsibleObjectField, FieldTemplate, ArrayFieldTemplate, ArrayFieldItemTemplate }}
          liveValidate={false}
          showErrorList={false}
          onChange={(e: IChangeEvent) => onChange(e.formData)}
        />
      </ErrorBoundary>
    </div>
  )
}

function Box({ children }: { children: ReactNode }) {
  return <div className="panel p-6 text-muted shadow-sm">{children}</div>
}
