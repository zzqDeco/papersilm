#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export PAPERSILM_PROCESS_TUI_SMOKE=1

go test ./internal/cli -run TestTUIProcessRuntimeE2E -count=1 -timeout=90s
