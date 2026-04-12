# papersilm

`papersilm` is a paper-focused document agent CLI for distilling one or more paper sources into high-signal outputs:

- single-paper digests
- multi-paper comparisons
- persistent session artifacts that future GUI tools can reuse

V1 is intentionally CLI-first and core-first: the headless core, session store, event protocol, and artifacts are the product surface. The current release does not include GUI, OCR, or package-manager distribution.

## Installation

### Build from source

```bash
git clone https://github.com/zzqDeco/papersilm.git
cd papersilm
go build -o ./bin/papersilm ./cmd/papersilm
```

### Install with Go

After `v0.1.0` is tagged:

```bash
go install github.com/zzqDeco/papersilm/cmd/papersilm@v0.1.0
```

### Download release binaries

The `v0.1.0` release is designed to ship these artifacts:

- `papersilm_v0.1.0_darwin_arm64.tar.gz`
- `papersilm_v0.1.0_darwin_amd64.tar.gz`
- `papersilm_v0.1.0_linux_arm64.tar.gz`
- `papersilm_v0.1.0_linux_amd64.tar.gz`
- `papersilm_v0.1.0_windows_arm64.zip`
- `papersilm_v0.1.0_windows_amd64.zip`
- `checksums.txt`

## Quick Start

Initialize config:

```bash
papersilm --config-init
```

The config file is written to `~/.papersilm/config.yaml`.

If no external provider is configured, `papersilm` falls back to a local deterministic tool-calling model so `plan`, `confirm`, `approve`, and `run` still work end-to-end.

## CLI Modes

`papersilm` supports three execution modes:

- `plan`: inspect sources and output the proposed DAG only
- `confirm`: stop after plan and wait for approval
- `auto`: run through to final artifacts

The current planner compiles work into an explicit DAG with role-scoped worker nodes such as `paper_summary_worker`, `experiment_worker`, `math_reasoner_worker`, and compare workers. `json` and `stream-json` outputs expose the full DAG so future GUI clients can consume the same execution graph.

## Usage

Print mode with one paper:

```bash
papersilm -p "plan current paper" \
  --source /path/to/paper.pdf \
  --permission-mode plan \
  --output-format json
```

Print mode with multiple sources:

```bash
papersilm -p "compare these papers" \
  --source /path/to/a.pdf \
  --source https://arxiv.org/abs/1706.03762 \
  --permission-mode auto
```

You can also pass a raw paper ID or an AlphaXiv URL. For arXiv-capable inputs, `papersilm` now prefers `AlphaXiv overview -> AlphaXiv full text -> arXiv PDF fallback`:

```bash
papersilm -p "summarize this paper" \
  --source 1706.03762 \
  --permission-mode auto

papersilm -p "explain equation 3 in this paper" \
  --source https://alphaxiv.org/overview/1706.03762 \
  --permission-mode auto
```

Interactive mode:

```bash
papersilm
```

Useful slash commands:

- `/source add <uri>`
- `/source replace <uri>`
- `/source list`
- `/plan [task]`
- `/run`
- `/approve`
- `/lang <zh|en|both>`
- `/style <distill|ultra|reviewer>`
- `/export`
- `/exit`

## Version

`papersilm version` prints build metadata:

```text
version=v0.1.0
commit=<git-sha>
date=<build-date>
```

Release builds inject these fields with linker flags so downloaded binaries remain traceable to a tag and commit.

## Current Scope

This repository currently includes:

- headless `pkg/core` and `pkg/protocol`
- local session storage and artifact persistence
- CLI with REPL and `-p/--print` modes
- internal tool registry
- source normalization and inspection
- AlphaXiv-first lookup for arXiv-compatible sources
- explicit DAG planning and execution state
- role-scoped multi-worker execution with parallel ready-node batches
- worker-composed single-paper distillation
- digest-driven paper comparison and final synthesis
- `plan | confirm | auto` permission flow

## Release Notes

`v0.1.0` is the first public CLI release. It is intentionally scoped as:

- GitHub Release + multi-platform binaries
- `go install` support
- no Homebrew formula
- no codesign or notarization
- no OCR
- no GUI build

## Documentation

- `doc/` contains project-level technical documentation.
- `doc/src/` mirrors non-test source paths with per-file technical docs such as `doc/src/internal/pipeline/service.go.plan.md`.
- `plan/` is reserved for current actionable implementation plans and currently contains only a placeholder `README.md`.

## Development

Run tests:

```bash
go test ./...
```

Build the CLI:

```bash
go build ./cmd/papersilm
```
