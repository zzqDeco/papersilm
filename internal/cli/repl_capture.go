package cli

import (
	"bytes"
	"context"
	"strings"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type capturedCommandResult struct {
	Text      string
	Pane      bool
	PaneTitle string
}

func executePromptText(ctx context.Context, svc *core.Service, session *protocol.SessionSnapshot, prompt string) (string, error) {
	buf := &bytes.Buffer{}
	out := NewOutputWriter(buf, protocol.OutputFormatText)
	result, err := svc.Execute(ctx, protocol.ClientRequest{
		Task:           prompt,
		PermissionMode: session.Meta.PermissionMode,
		Language:       session.Meta.Language,
		Style:          session.Meta.Style,
		SessionID:      session.Meta.SessionID,
	})
	if err != nil {
		return "", err
	}
	if err := out.PrintResult(result); err != nil {
		return "", err
	}
	*session = result.Session
	return strings.TrimSpace(buf.String()), nil
}

func executeSlashCommandText(
	ctx context.Context,
	svc *core.Service,
	store *storage.Store,
	session *protocol.SessionSnapshot,
	line string,
) (capturedCommandResult, error) {
	buf := &bytes.Buffer{}
	out := NewOutputWriter(buf, protocol.OutputFormatText)
	next := *session
	if err := handleSlash(ctx, svc, store, &next, out, line); err != nil {
		return capturedCommandResult{}, err
	}
	*session = next
	title, pane := classifyPaneCommand(line)
	return capturedCommandResult{
		Text:      strings.TrimSpace(buf.String()),
		Pane:      pane,
		PaneTitle: title,
	}, nil
}

func classifyPaneCommand(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "/help":
		return "Help", true
	case trimmed == "/tasks":
		return "Tasks", true
	case strings.HasPrefix(trimmed, "/task show "):
		return "Task", true
	case trimmed == "/workspace show" || strings.HasPrefix(trimmed, "/workspace files") || strings.HasPrefix(trimmed, "/workspace search ") || trimmed == "/workspace sessions":
		return "Workspace", true
	case strings.HasPrefix(trimmed, "/paper "):
		return "Workspace", true
	case strings.HasPrefix(trimmed, "/skill list"):
		return "Skills", true
	case strings.HasPrefix(trimmed, "/skill show "):
		return "Skill Run", true
	case trimmed == "/export":
		return "Artifacts", true
	case trimmed == "/source list":
		return "Sources", true
	case strings.HasPrefix(trimmed, "/open "):
		return "File", true
	default:
		return "", false
	}
}
