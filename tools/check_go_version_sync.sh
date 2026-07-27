#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_mod="$repo_root/backend/go.mod"

go_version=$(awk '$1 == "go" { print $2; exit }' "$go_mod")
if [ -z "$go_version" ]; then
  echo "ERROR: unable to read the Go version from backend/go.mod" >&2
  exit 1
fi

expected="ARG GOLANG_IMAGE=golang:${go_version}-alpine"
status=0

for dockerfile in deploy/Dockerfile deploy/backend/Dockerfile; do
  if ! grep -Fqx "$expected" "$repo_root/$dockerfile"; then
    actual=$(grep '^ARG GOLANG_IMAGE=' "$repo_root/$dockerfile" || true)
    echo "ERROR: $dockerfile is out of sync with backend/go.mod" >&2
    echo "  expected: $expected" >&2
    echo "  actual:   ${actual:-<missing>}" >&2
    status=1
  fi
done

exit "$status"
