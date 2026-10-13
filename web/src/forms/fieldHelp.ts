import { lookup } from '../i18n'

// Help text for common Istio/Gateway API fields, shown under each input. Entries
// live in the dictionaries as "help.<key>". Lookup is a fallback chain (Gemini ②):
// "{typeId}/{field}" for collision-prone fields, else the global "{field}";
// lookup() tries the active locale, then English.
export function fieldHelp(typeId: string, field: string): string | undefined {
  return lookup(`help.${typeId}/${field}`) ?? lookup(`help.${field}`)
}
