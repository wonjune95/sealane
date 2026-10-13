import { lookup } from '../i18n'

// Light-gray example placeholders for common Istio/Gateway API fields, so an empty
// form shows the expected shape ("oh, that's the format") instead of a blank box.
// Entries live in the dictionaries as "example.<key>". Same fallback chain as
// fieldHelp: "{typeId}/{field}" then global "{field}"; locale first, then English.
// Only curated fields get an example — a wrong guess is worse than none.
export function fieldExample(typeId: string, field: string): string | undefined {
  return lookup(`example.${typeId}/${field}`) ?? lookup(`example.${field}`)
}
