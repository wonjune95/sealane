import { useMemo } from 'react'
import CodeMirror, { Decoration, EditorView, StateField, ViewPlugin, type EditorState } from '@uiw/react-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { indentationMarkers } from '@replit/codemirror-indentation-markers'
import { useTheme } from '../ui/theme'
import { t } from '../i18n'

// In YAML indentation is syntax, so draw a vertical guide (|) at every level to
// make a one-column misalignment obvious. The block holding the cursor is emphasized.
const whitespaceExtensions = [
  indentationMarkers({
    highlightActiveBlock: true,
    thickness: 1.5,
    colors: {
      light: '#cbd5e1',
      dark: '#475569',
      activeLight: '#64748b',
      activeDark: '#94a3b8',
    },
  }),
]

// Highlights (class cm-focus-line, fades via CSS) and centers 1-based `line` once.
function focusLineExtension(line: number) {
  const pos = (s: EditorState) => (line <= s.doc.lines ? s.doc.line(line).from : null)
  return [
    StateField.define({
      create: (s) => {
        const p = pos(s)
        return p === null ? Decoration.none : Decoration.set([Decoration.line({ class: 'cm-focus-line' }).range(p)])
      },
      update: (d, tr) => d.map(tr.changes),
      provide: (f) => EditorView.decorations.from(f),
    }),
    ViewPlugin.define((view) => {
      // a plugin may not dispatch while it is being constructed
      const timer = setTimeout(() => {
        const p = pos(view.state)
        if (p !== null) view.dispatch({ effects: EditorView.scrollIntoView(p, { y: 'center' }) })
      })
      return { destroy: () => clearTimeout(timer) }
    }),
  ]
}

export function YamlEditor({
  value,
  onChange,
  readOnly = false,
  focusLine,
}: {
  value: string
  onChange?: (v: string) => void
  readOnly?: boolean
  focusLine?: number
}) {
  const { theme } = useTheme()
  const focusExt = useMemo(() => (focusLine ? focusLineExtension(focusLine) : []), [focusLine])
  return (
    <div className="panel overflow-hidden rounded-xl">
      <CodeMirror
        value={value}
        height="480px"
        style={{ fontSize: 13 }}
        theme={theme === 'dark' ? 'dark' : 'light'}
        extensions={[yaml(), ...whitespaceExtensions, focusExt]}
        editable={!readOnly}
        onChange={onChange}
        aria-label={t(readOnly ? 'editor.yamlLabelReadOnly' : 'editor.yamlLabel')}
      />
    </div>
  )
}
