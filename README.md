# papersilm

`papersilm` is a paper-focused document agent CLI. It supports one or more paper sources, builds a plan first, and then produces:

- single-paper digests
- multi-paper comparisons
- persistent session artifacts for future GUI reuse

## Current Scope

This repository contains the V1 core-first implementation:

- headless `pkg/core` and `pkg/protocol`
- local session storage and artifact persistence
- CLI with REPL and `-p/--print` modes
- internal tool registry
- source normalization and inspection
- heuristic single-paper distillation
- digest-driven paper comparison
- `plan | confirm | auto` permission flow

## CLI Modes

`papersilm` supports three execution modes:

- `plan`: inspect sources and output the proposed tool plan only
- `confirm`: stop after plan and wait for approval
- `auto`: run through to final artifacts

## Usage

Initialize config:

```bash
papersilm --config-init
```

Plan a single paper:

```bash
papersilm -p "plan current paper" \
  --source /path/to/paper.pdf \
  --permission-mode plan \
  --output-format json
```

Compare multiple papers:

```bash
papersilm -p "compare these papers" \
  --source /path/to/a.pdf \
  --source https://arxiv.org/abs/1706.03762 \
  --permission-mode auto
```

Interactive mode:

```bash
papersilm
```

Then use slash commands:

- `/source add <uri>`
- `/plan [task]`
- `/approve`
- `/run [task]`
- `/lang <zh|en|both>`
- `/style <distill|ultra|reviewer>`
- `/export`

## Future GUI Direction

The CLI is intentionally built on a headless core. Future GUI integration should consume the same session store, artifacts, and `stream-json` event protocol instead of re-implementing document logic.

## Notes

- V1 accepts local PDFs and arXiv `abs` / `pdf` URLs.
- Provider-backed LLM execution is scaffolded; the current default execution path is heuristic and does not require API keys.
