import CodeMirror from '@uiw/react-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { indentationMarkers } from '@replit/codemirror-indentation-markers'
import { useTheme } from '../ui/theme'

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

export function YamlEditor({
  value,
  onChange,
  readOnly = false,
}: {
  value: string
  onChange?: (v: string) => void
  readOnly?: boolean
}) {
  const { theme } = useTheme()
  return (
    <div className="panel overflow-hidden rounded-xl">
      <CodeMirror
        value={value}
        height="480px"
        style={{ fontSize: 13 }}
        theme={theme === 'dark' ? 'dark' : 'light'}
        extensions={[yaml(), ...whitespaceExtensions]}
        editable={!readOnly}
        onChange={onChange}
      />
    </div>
  )
}
