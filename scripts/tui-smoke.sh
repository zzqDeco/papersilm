#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run() {
  printf '+ %q' "$1"
  shift
  for arg in "$@"; do
    printf ' %q' "$arg"
  done
  printf '\n'
  "$@"
}

run "visual/pty gate" go test ./internal/cli -run 'TestTUIVisual(Golden|Invariants)|TestTUIPTYSmokeScenarios|TestPTYScreenBufferHonorsEraseAndWrap' -count=1
run "cli tui packages" go test ./internal/cli ./internal/cli/tui
run "all tests" go test ./...
run "vet" go vet ./...
run "build" go build -o bin/papersilm ./cmd/papersilm
run "diff check" git diff --check
