# papersilm

`papersilm` is a paper-focused document agent CLI. It is designed to ingest one or more paper sources, inspect them, produce a plan, and then generate single-paper digests and cross-paper comparisons.

## Status

This repository contains the V1 core-first implementation skeleton:

- headless core and protocol packages
- session storage and artifact persistence
- CLI with REPL and print modes
- internal tool registry
- source normalization and plan/run flows

Provider-backed LLM execution requires runtime configuration.

