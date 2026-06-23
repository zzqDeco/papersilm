# Workspace Tool Contract

This document records the user-visible contract for the Eino-native workspace
tools. It is intentionally scoped to behavior that clients, tests, and release
smoke checks can rely on.

## Tool Roles

- `workspace_list_files`, `workspace_read_file`, and `workspace_search` are
  read-only tools. They do not require permission in `confirm` mode.
- `workspace_replace_text` is the preferred tool for localized edits. It takes
  `path`, exact `old_text`, `new_text`, and an optional `summary`.
- `workspace_write_file` creates or replaces a whole file. Agents should use it
  for new files or intentional whole-file rewrites, not for small replacements.
- `workspace_run_command` runs a shell command from the workspace root and
  returns structured stdout, stderr, cwd, exit code, and truncation metadata.

## Permission Semantics

Side-effect tools are tool-scoped permission requests in `confirm` mode:

- `workspace_replace_text` uses `Tool=workspace_edit` and `Operation=replace`.
- `workspace_write_file` uses `Tool=workspace_edit` and `Operation=write`.
- `workspace_run_command` uses `Tool=workspace_command` and `Operation=shell`.

`accept-once` resumes only the active interrupt. `accept-session` writes a
session-local allow rule, never global config. Rules keep the operation attached
to the request, so allowing `replace` for a path does not automatically allow a
whole-file `write` for the same path.

Default session scopes are:

- file edits: path scope
- shell commands: command-prefix scope
- one-time decisions: node scope

## Edit Preview And Apply

Edit permission requests include a diff preview. The preview stores the file
hash observed at approval time and the content required to apply the edit.

In the Eino workspace tool apply path, `papersilm` revalidates before writing:

- whole-file writes fail with `status=conflict` if the file changed after the
  preview was generated;
- localized replacements fail with `status=conflict` if the file changed, if
  `old_text` is missing, or if `old_text` appears more than once;
- conflict results set `changed=false` and include `conflict=true`.

When a conflict is returned, the agent should re-read the file, regenerate the
edit preview, and ask again if the next attempt still has side effects.
Older planned approval paths can still surface stale previews as run errors;
clients should treat the `status=conflict` contract as specific to
`workspace_replace_text` and `workspace_write_file` tool results.

## Command Results

Shell command failures are tool results unless the command cannot be started,
the workspace path is unsafe, or the context is cancelled.

For normal process exits:

- `status=completed` means exit code `0`;
- `status=failed` means a non-zero or signaled exit;
- `exit_code`, `cwd`, `stdout`, `stderr`, `stdout_truncated`, and
  `stderr_truncated` describe what happened.

The agent should continue reasoning from a `status=failed` tool result instead
of treating it as an Eino runtime failure.

## Tool Call Records

The public protocol shape remains stable. Detailed workspace tool metadata is
persisted in the session `tool_calls.jsonl` file. Clients reading that session
artifact may see:

- `target_path`
- `command`
- `cwd`
- `exit_code`
- `stdout_truncated`
- `stderr_truncated`
- `changed`
- `conflict`
- `tool_result_status`

Full diff, command output, feedback, and tool-call metadata belong in transcript
and session files rather than the main TUI timeline. Current stream events and
transcript entries do not promise these detailed fields.

## Release Smoke Checklist

Before release-facing changes, run:

```bash
bash scripts/tui-smoke.sh
go test ./...
go vet ./...
go build -o bin/papersilm ./cmd/papersilm
bash scripts/workflow-static-check.sh
git diff --check
```

Manual smoke should include:

- `papersilm -p "search current workspace for README" --permission-mode auto`
- `papersilm -p "update \`README.md\` replace \`typo\` with \`type\`" --permission-mode confirm`
- `papersilm -p "run command \`printf stdout; printf stderr >&2; exit 7\`" --permission-mode confirm --output-format json`
- TUI approval, reject with feedback, and accept-session for a command prefix
