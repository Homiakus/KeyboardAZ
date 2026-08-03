#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"

"$root/tests/run_native_firmware_tests.sh"
(
  cd "$root/go-app"
  gofmt_files="$(gofmt -l .)"
  if [[ -n "$gofmt_files" ]]; then
    echo "gofmt required:" >&2
    echo "$gofmt_files" >&2
    exit 1
  fi
  go test -race ./...
  go vet ./...
  go test -bench=. -benchmem ./textinput
)
