// Prints the license files of every npm package the web UI ships with (dependencies, not build
// tools): part two of THIRD_PARTY_NOTICES. Run from web/ after npm ci. See hack/third-party-notices.sh.
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

const lock = JSON.parse(readFileSync('package-lock.json', 'utf8'))
console.log('== npm packages bundled into the web UI')
for (const [path, pkg] of Object.entries(lock.packages).sort(([a], [b]) => a.localeCompare(b))) {
  // "" is the app itself; dev entries are build and test tools; optional ones may not be installed.
  if (!path || pkg.dev || !existsSync(path)) continue
  const name = path.slice(path.lastIndexOf('node_modules/') + 'node_modules/'.length)
  console.log(`\n----- ${name}@${pkg.version} (${pkg.license ?? 'license in the files below'})`)
  const files = readdirSync(path).filter((f) => /^(licen[cs]e|copying|notice)/i.test(f)).sort()
  for (const f of files) console.log(`--- ${f}\n${readFileSync(join(path, f), 'utf8')}`)
  if (!files.length) {
    const meta = JSON.parse(readFileSync(join(path, 'package.json'), 'utf8'))
    const repo = typeof meta.repository === 'string' ? meta.repository : meta.repository?.url
    console.log(`(no license file in the package; it declares ${meta.license ?? 'no license'}${repo ? `, source ${repo}` : ''})`)
  }
}
