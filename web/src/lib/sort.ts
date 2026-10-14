export const PAGE = 50
export type SortDir = 'asc' | 'desc'
export interface SortState {
  key: string
  dir: SortDir
}
export function toggleSort(s: SortState | null, key: string): SortState {
  if (!s || s.key !== key) return { key, dir: 'asc' }
  return { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
}
const UNIT: Record<string, number> = { d: 86400, h: 3600, m: 60, s: 1 }
// "3d22h" -> seconds; "" -> -1; unknown tokens ignored.
export function parseAge(age: string): number {
  if (!age) return -1
  let n = 0
  for (const [, v, u] of age.matchAll(/(\d+)([a-z])/g)) n += Number(v) * (UNIT[u] ?? 0)
  return n
}
// Array.prototype.sort is stable; null keeps the order as served.
export function sortBy<T>(rows: T[], s: SortState | null, get: (r: T, key: string) => string | number): T[] {
  if (!s) return rows
  const m = s.dir === 'asc' ? 1 : -1
  return [...rows].sort((a, b) => {
    const x = get(a, s.key)
    const y = get(b, s.key)
    return (typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y))) * m
  })
}
