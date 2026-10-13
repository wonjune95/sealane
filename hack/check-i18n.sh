#!/usr/bin/env bash
# Fails when Korean text appears outside the ko locale files, or when en/ko keys differ.
set -euo pipefail
cd "$(dirname "$0")/.."

leaks=$(git grep -n -P '[\x{AC00}-\x{D7A3}]' -- . ':!web/src/i18n/ko.json' ':!README.ko.md' ':!docs/superpowers/' || true)
if [ -n "$leaks" ]; then
  echo "Korean text outside locale files (move it to web/src/i18n/ko.json or translate it):"
  echo "$leaks"
  echo "files: $(echo "$leaks" | cut -d: -f1 | sort -u | wc -l), lines: $(echo "$leaks" | wc -l)"
  exit 1
fi

node -e '
const en = require("./web/src/i18n/en.json"), ko = require("./web/src/i18n/ko.json");
const missing = Object.keys(en).filter((k) => !(k in ko));
const extra = Object.keys(ko).filter((k) => !(k in en));
if (missing.length || extra.length) {
  console.error("i18n key mismatch", { missingInKo: missing, extraInKo: extra });
  process.exit(1);
}
console.log(`i18n ok: ${Object.keys(en).length} keys`);
'
