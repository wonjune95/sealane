#!/usr/bin/env bash
# Prints the license files of every Go module compiled into the server: part one of the
# THIRD_PARTY_NOTICES file that ships in the image (/licenses) and with each release.
# Needs the module cache (go mod download). See hack/third-party-notices.sh.
set -euo pipefail
echo "== Go modules compiled into the sealane binary"
go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}@{{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/server | sort -u |
  while read -r mod dir; do
    echo
    echo "----- $mod"
    files=$(find "$dir" -maxdepth 1 -type f \( -iname 'licen[cs]e*' -o -iname 'copying*' -o -iname 'notice*' -o -iname 'patents*' \) | sort)
    [ -n "$files" ] || { echo "(no license file in the module; see https://pkg.go.dev/${mod%@*}?tab=licenses)"; continue; }
    for f in $files; do
      echo "--- $(basename "$f")"
      cat "$f"
    done
  done
