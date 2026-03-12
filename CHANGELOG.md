# Changelog

All notable changes to this project will be documented in this file.

## [v0.1.0] - Unreleased

### Added

- Paper-focused CLI with REPL and `-p/--print` modes
- Real `plan -> approve -> run` flow built on Eino ADK primitives
- Multi-source paper attachment, inspection, single-paper distillation, and comparison
- Session persistence, checkpoints, artifacts, and `stream-json` output support
- Release metadata command via `papersilm version`
- GitHub Actions CI and tag-driven release automation

### Packaging

- Multi-platform binaries for macOS, Linux, and Windows
- `go install` support via `github.com/zzqDeco/papersilm/cmd/papersilm`
