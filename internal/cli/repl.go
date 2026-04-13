package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func RunREPL(ctx context.Context, svc *core.Service, store *storage.Store, current protocol.SessionSnapshot, out *OutputWriter) error {
	reader := bufio.NewReader(os.Stdin)
	session := current
	if session.Meta.SessionID == "" {
		meta, err := svc.NewSession(protocol.PermissionModeConfirm, "zh", "distill")
		if err != nil {
			return err
		}
		session, err = store.Snapshot(meta.SessionID)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stdout, "papersilm session=%s\nType /help for commands.\n", session.Meta.SessionID)
	for {
		fmt.Fprint(os.Stdout, "papersilm> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" {
			return nil
		}
		if strings.HasPrefix(line, "/") {
			if err := handleSlash(ctx, svc, store, &session, out, line); err != nil {
				fmt.Fprintf(os.Stdout, "error: %v\n", err)
			}
			continue
		}
		req := protocol.ClientRequest{
			Task:           line,
			PermissionMode: session.Meta.PermissionMode,
			Language:       session.Meta.Language,
			Style:          session.Meta.Style,
			SessionID:      session.Meta.SessionID,
		}
		result, err := svc.Execute(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stdout, "error: %v\n", err)
			continue
		}
		if err := out.PrintResult(result); err != nil {
			return err
		}
		session = result.Session
	}
}

func handleSlash(ctx context.Context, svc *core.Service, store *storage.Store, session *protocol.SessionSnapshot, out *OutputWriter, line string) error {
	if strings.HasPrefix(line, "/workspace") {
		return handleWorkspaceCommand(svc, session, out, line)
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/help":
		_, err := fmt.Fprintln(os.Stdout, "/help, /plan [task], /approve, /run, /tasks, /task show|run|approve, /lang <zh|en|both>, /style <distill|ultra|reviewer>, /source add|replace|list|remove, /workspace list|show|note add|annotation add, /session name <name>, /export, /clear, /exit")
		return err
	case "/clear":
		_, err := fmt.Fprintln(os.Stdout, strings.Repeat("-", 72))
		return err
	case "/lang":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /lang <zh|en|both>")
		}
		session.Meta.Language = fields[1]
		if err := store.SaveMeta(session.Meta); err != nil {
			return err
		}
		return store.InvalidatePlanState(session.Meta.SessionID)
	case "/style":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /style <distill|ultra|reviewer>")
		}
		session.Meta.Style = fields[1]
		if err := store.SaveMeta(session.Meta); err != nil {
			return err
		}
		return store.InvalidatePlanState(session.Meta.SessionID)
	case "/session":
		if len(fields) < 3 || fields[1] != "name" {
			return fmt.Errorf("usage: /session name <name>")
		}
		session.Meta.Name = strings.Join(fields[2:], " ")
		return store.SaveMeta(session.Meta)
	case "/source":
		return handleSourceCommand(ctx, svc, store, session, out, fields)
	case "/plan":
		task := "inspect current sources and prepare plan"
		if len(fields) > 1 {
			task = strings.Join(fields[1:], " ")
		}
		result, err := svc.Execute(ctx, protocol.ClientRequest{
			Task:           task,
			PermissionMode: protocol.PermissionModePlan,
			Language:       session.Meta.Language,
			Style:          session.Meta.Style,
			SessionID:      session.Meta.SessionID,
		})
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	case "/run":
		result, err := svc.RunPlanned(ctx, session.Meta.SessionID, session.Meta.Language, session.Meta.Style)
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	case "/tasks":
		board, err := svc.LoadTaskBoard(session.Meta.SessionID)
		if err != nil {
			return err
		}
		session.TaskBoard = board
		return out.PrintTaskBoard(board)
	case "/task":
		return handleTaskCommand(ctx, svc, session, out, fields)
	case "/approve":
		result, err := svc.Approve(ctx, session.Meta.SessionID, true, "")
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	case "/export":
		for _, artifact := range session.Artifacts {
			fmt.Fprintf(os.Stdout, "- %s: %s\n", artifact.ArtifactID, artifact.Paths["markdown"])
		}
		return nil
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
}

func handleSourceCommand(ctx context.Context, svc *core.Service, store *storage.Store, session *protocol.SessionSnapshot, out *OutputWriter, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("usage: /source add|replace|list|remove ...")
	}
	switch fields[1] {
	case "list":
		for _, src := range session.Sources {
			fmt.Fprintf(os.Stdout, "- %s %s (%s)\n", src.PaperID, src.URI, src.Status)
		}
		return nil
	case "add":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /source add <uri>")
		}
		snapshot, err := svc.AttachSources(ctx, session.Meta.SessionID, []string{strings.Join(fields[2:], " ")}, false)
		if err != nil {
			return err
		}
		*session = snapshot
		return out.PrintResult(protocol.RunResult{Session: snapshot})
	case "replace":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /source replace <uri>")
		}
		snapshot, err := svc.AttachSources(ctx, session.Meta.SessionID, []string{strings.Join(fields[2:], " ")}, true)
		if err != nil {
			return err
		}
		*session = snapshot
		return out.PrintResult(protocol.RunResult{Session: snapshot})
	case "remove":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /source remove <paper_id>")
		}
		id := fields[2]
		filtered := make([]protocol.PaperRef, 0, len(session.Sources))
		for _, src := range session.Sources {
			if src.PaperID != id {
				filtered = append(filtered, src)
			}
		}
		if err := store.DeleteWorkspaceState(session.Meta.SessionID, id); err != nil {
			return err
		}
		if err := store.SaveSources(session.Meta.SessionID, filtered); err != nil {
			return err
		}
		if err := store.InvalidatePlanState(session.Meta.SessionID); err != nil {
			return err
		}
		snapshot, err := store.Snapshot(session.Meta.SessionID)
		if err != nil {
			return err
		}
		*session = snapshot
		return nil
	default:
		return fmt.Errorf("unknown /source action: %s", fields[1])
	}
}

func handleWorkspaceCommand(svc *core.Service, session *protocol.SessionSnapshot, out *OutputWriter, line string) error {
	head, body := splitWorkspaceCommand(line)
	fields := strings.Fields(head)
	if len(fields) < 2 {
		return fmt.Errorf("usage: /workspace list|show|note add|annotation add ...")
	}

	switch fields[1] {
	case "list":
		workspaces, err := svc.LoadWorkspaces(session.Meta.SessionID)
		if err != nil {
			return err
		}
		session.Workspaces = workspaces
		return out.PrintWorkspaceList(workspaces)
	case "show":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /workspace show <paper_id>")
		}
		workspaces, err := svc.LoadWorkspaces(session.Meta.SessionID)
		if err != nil {
			return err
		}
		session.Workspaces = workspaces
		workspace, ok := findWorkspace(workspaces, fields[2])
		if !ok {
			return fmt.Errorf("workspace not found: %s", fields[2])
		}
		return out.PrintWorkspace(*workspace)
	case "note":
		if len(fields) < 4 || fields[2] != "add" {
			return fmt.Errorf("usage: /workspace note add <paper_id> :: <body>")
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("usage: /workspace note add <paper_id> :: <body>")
		}
		snapshot, err := svc.AddWorkspaceNote(session.Meta.SessionID, fields[3], body)
		if err != nil {
			return err
		}
		*session = snapshot
		workspace, ok := findWorkspace(snapshot.Workspaces, fields[3])
		if !ok {
			return fmt.Errorf("workspace not found: %s", fields[3])
		}
		return out.PrintWorkspace(*workspace)
	case "annotation":
		if len(fields) < 6 || fields[2] != "add" {
			return fmt.Errorf("usage: /workspace annotation add <paper_id> page|snippet|section <value> :: <body>")
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("usage: /workspace annotation add <paper_id> page|snippet|section <value> :: <body>")
		}
		anchor, err := parseWorkspaceAnchor(fields[4], fields[5:])
		if err != nil {
			return err
		}
		snapshot, err := svc.AddWorkspaceAnnotation(session.Meta.SessionID, fields[3], anchor, body)
		if err != nil {
			return err
		}
		*session = snapshot
		workspace, ok := findWorkspace(snapshot.Workspaces, fields[3])
		if !ok {
			return fmt.Errorf("workspace not found: %s", fields[3])
		}
		return out.PrintWorkspace(*workspace)
	default:
		return fmt.Errorf("unknown /workspace action: %s", fields[1])
	}
}

func handleTaskCommand(ctx context.Context, svc *core.Service, session *protocol.SessionSnapshot, out *OutputWriter, fields []string) error {
	if len(fields) < 3 {
		return fmt.Errorf("usage: /task show|run|approve <id>")
	}
	switch fields[1] {
	case "show":
		board, err := svc.LoadTaskBoard(session.Meta.SessionID)
		if err != nil {
			return err
		}
		session.TaskBoard = board
		task, ok := findTaskByID(board, fields[2])
		if !ok {
			return fmt.Errorf("task not found: %s", fields[2])
		}
		return out.PrintTaskCard(task)
	case "run":
		result, err := svc.RunTask(ctx, session.Meta.SessionID, fields[2], session.Meta.Language, session.Meta.Style)
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	case "approve":
		result, err := svc.ApproveTask(ctx, session.Meta.SessionID, fields[2], true, "")
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	default:
		return fmt.Errorf("unknown /task action: %s", fields[1])
	}
}

func splitWorkspaceCommand(line string) (string, string) {
	parts := strings.SplitN(line, "::", 2)
	head := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return head, ""
	}
	return head, strings.TrimSpace(parts[1])
}

func parseWorkspaceAnchor(kind string, parts []string) (protocol.AnchorRef, error) {
	switch kind {
	case string(protocol.AnchorKindPage):
		if len(parts) != 1 {
			return protocol.AnchorRef{}, fmt.Errorf("usage: /workspace annotation add <paper_id> page <n> :: <body>")
		}
		page, err := strconv.Atoi(parts[0])
		if err != nil || page <= 0 {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation page must be a positive integer")
		}
		return protocol.AnchorRef{Kind: protocol.AnchorKindPage, Page: page}, nil
	case string(protocol.AnchorKindSnippet):
		value := strings.TrimSpace(strings.Join(parts, " "))
		if value == "" {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation snippet is required")
		}
		return protocol.AnchorRef{Kind: protocol.AnchorKindSnippet, Snippet: value}, nil
	case string(protocol.AnchorKindSection):
		value := strings.TrimSpace(strings.Join(parts, " "))
		if value == "" {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation section is required")
		}
		return protocol.AnchorRef{Kind: protocol.AnchorKindSection, Section: value}, nil
	default:
		return protocol.AnchorRef{}, fmt.Errorf("unsupported workspace anchor kind: %s", kind)
	}
}

func findWorkspace(workspaces []protocol.PaperWorkspace, paperID string) (*protocol.PaperWorkspace, bool) {
	for idx := range workspaces {
		if workspaces[idx].PaperID == paperID {
			return &workspaces[idx], true
		}
	}
	return nil, false
}

func findTaskByID(board *protocol.TaskBoard, taskID string) (protocol.TaskCard, bool) {
	if board == nil {
		return protocol.TaskCard{}, false
	}
	for _, task := range board.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return protocol.TaskCard{}, false
}
