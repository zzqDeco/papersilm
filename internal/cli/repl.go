package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"papersilm/internal/storage"
	"papersilm/pkg/core"
	"papersilm/pkg/protocol"
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
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/help":
		_, err := fmt.Fprintln(os.Stdout, "/help, /plan [task], /approve, /run [task], /lang <zh|en|both>, /style <distill|ultra|reviewer>, /source add|replace|list|remove, /session name <name>, /export, /clear, /exit")
		return err
	case "/clear":
		_, err := fmt.Fprintln(os.Stdout, strings.Repeat("-", 72))
		return err
	case "/lang":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /lang <zh|en|both>")
		}
		session.Meta.Language = fields[1]
		return store.SaveMeta(session.Meta)
	case "/style":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /style <distill|ultra|reviewer>")
		}
		session.Meta.Style = fields[1]
		return store.SaveMeta(session.Meta)
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
		task := session.Meta.LastTask
		if len(fields) > 1 {
			task = strings.Join(fields[1:], " ")
		}
		if strings.TrimSpace(task) == "" {
			task = "distill current papers"
		}
		result, err := svc.Execute(ctx, protocol.ClientRequest{
			Task:           task,
			PermissionMode: protocol.PermissionModeAuto,
			Language:       session.Meta.Language,
			Style:          session.Meta.Style,
			SessionID:      session.Meta.SessionID,
		})
		if err != nil {
			return err
		}
		*session = result.Session
		return out.PrintResult(result)
	case "/approve":
		result, err := svc.Execute(ctx, protocol.ClientRequest{
			Task:           "/approve",
			PermissionMode: protocol.PermissionModeAuto,
			Language:       session.Meta.Language,
			Style:          session.Meta.Style,
			SessionID:      session.Meta.SessionID,
		})
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
		result, err := svc.Execute(ctx, protocol.ClientRequest{
			Task:           "attach source",
			Sources:        []string{strings.Join(fields[2:], " ")},
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
	case "replace":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /source replace <uri>")
		}
		if err := store.SaveSources(session.Meta.SessionID, nil); err != nil {
			return err
		}
		result, err := svc.Execute(ctx, protocol.ClientRequest{
			Task:           "attach source",
			Sources:        []string{strings.Join(fields[2:], " ")},
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
		if err := store.SaveSources(session.Meta.SessionID, filtered); err != nil {
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

