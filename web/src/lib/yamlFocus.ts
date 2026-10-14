// findFocusLine returns the 1-based line of `path` (e.g. spec.http[0].route[1].destination.subset)
// in a YAML text, or null when the path cannot be followed. With `value`, a key line that does
// not contain it yields the first following line in its block that does (else the key line).
// ponytail: a text walk by indentation, not a YAML parser — fits what js-yaml dump emits
// (block style, two-space indent, quoted-free keys); flow style, anchors or `- - ` nesting break it.
export function findFocusLine(yaml: string, path: string, value?: string): number | null {
  const lines = yaml.split('\n')
  const shape = (s: string) => {
    const m = /^( *)((?:- )*)(.*)$/.exec(s)!
    return { ind: m[1].length, col: m[1].length + m[2].length, dash: m[2] !== '', rest: m[3] }
  }
  const skip = (s: string) => s.trim() === '' || s.trim().startsWith('#')
  let at = -1 // line of the last matched segment
  let start = 0 // first line to search
  let col = 0 // column the next key starts at
  for (const [, key, idx] of path.matchAll(/([^.[\]]+)|\[(\d+)\]/g)) {
    let found = -1
    if (key !== undefined) {
      for (let i = start; i < lines.length; i++) {
        if (skip(lines[i])) continue
        const l = shape(lines[i])
        if (i > start && l.ind < col) break // left the block
        if (l.col === col && (l.rest === `${key}:` || l.rest.startsWith(`${key}: `))) {
          found = i
          break
        }
      }
      if (found < 0) return null
      start = found + 1
      col += 2
    } else {
      let n = Number(idx)
      let dashCol = -1
      for (let i = start; i < lines.length; i++) {
        if (skip(lines[i])) continue
        const l = shape(lines[i])
        if (dashCol < 0) {
          if (!l.dash || (l.ind !== col && l.ind !== col - 2)) return null // not a block sequence
          dashCol = l.ind
        }
        if (l.ind < dashCol || (l.ind === dashCol && !l.dash)) break
        if (l.ind === dashCol && n-- === 0) {
          found = i
          break
        }
      }
      if (found < 0) return null
      start = found // the item's first key sits on the dash line
      col = dashCol + 2
    }
    at = found
  }
  if (at < 0) return null
  if (value && !lines[at].includes(value)) {
    const ind = shape(lines[at]).col
    for (let i = at + 1; i < lines.length && (skip(lines[i]) || shape(lines[i]).ind > ind); i++) {
      if (lines[i].includes(value)) return i + 1
    }
  }
  return at + 1
}
