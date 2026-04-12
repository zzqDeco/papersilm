package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type OutputWriter struct {
	w      io.Writer
	format protocol.OutputFormat
}

func NewOutputWriter(w io.Writer, format protocol.OutputFormat) *OutputWriter {
	return &OutputWriter{w: w, format: format}
}

func (o *OutputWriter) Emit(event protocol.StreamEvent) error {
	switch o.format {
	case protocol.OutputFormatStreamJSON:
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(o.w, string(raw))
		return err
	case protocol.OutputFormatText:
		if strings.TrimSpace(event.Message) != "" {
			_, err := fmt.Fprintf(o.w, "[%s] %s\n", event.Type, event.Message)
			return err
		}
	default:
	}
	return nil
}

func (o *OutputWriter) PrintResult(result protocol.RunResult) error {
	switch o.format {
	case protocol.OutputFormatJSON:
		raw, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(o.w, string(raw))
		return err
	case protocol.OutputFormatStreamJSON:
		return nil
	case protocol.OutputFormatText:
		if result.Approval != nil {
			if err := o.printApproval(*result.Approval); err != nil {
				return err
			}
		}
		if result.Plan != nil && result.Comparison == nil && len(result.Digests) == 0 {
			return o.printPlan(*result.Plan)
		}
		if result.Comparison != nil {
			if _, err := fmt.Fprintln(o.w, result.Comparison.Markdown); err != nil {
				return err
			}
		}
		if result.Comparison == nil {
			if len(result.Digests) == 0 && result.Plan == nil && len(result.Session.Sources) > 0 {
				if _, err := fmt.Fprintln(o.w, "Sources:"); err != nil {
					return err
				}
				for _, src := range result.Session.Sources {
					if _, err := fmt.Fprintf(o.w, "- %s (%s)\n", src.PaperID, src.URI); err != nil {
						return err
					}
				}
			}
			for i, digest := range result.Digests {
				if i > 0 {
					if _, err := fmt.Fprintln(o.w, "---"); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(o.w, digest.Markdown); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (o *OutputWriter) PrintWorkspaceList(workspaces []protocol.PaperWorkspace) error {
	switch o.format {
	case protocol.OutputFormatJSON, protocol.OutputFormatStreamJSON:
		raw, err := json.MarshalIndent(workspaces, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(o.w, string(raw))
		return err
	case protocol.OutputFormatText:
		if len(workspaces) == 0 {
			_, err := fmt.Fprintln(o.w, "No workspaces.")
			return err
		}
		if _, err := fmt.Fprintln(o.w, "Workspaces:"); err != nil {
			return err
		}
		for _, workspace := range workspaces {
			digest := "no"
			if workspace.Digest != nil {
				digest = "yes"
			}
			if _, err := fmt.Fprintf(
				o.w,
				"- %s | digest=%s | notes=%d | annotations=%d | resources=%d | similar=%d\n",
				workspace.PaperID,
				digest,
				len(workspace.Notes),
				len(workspace.Annotations),
				len(workspace.Resources),
				len(workspace.Similar),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *OutputWriter) PrintWorkspace(workspace protocol.PaperWorkspace) error {
	switch o.format {
	case protocol.OutputFormatJSON, protocol.OutputFormatStreamJSON:
		raw, err := json.MarshalIndent(workspace, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(o.w, string(raw))
		return err
	case protocol.OutputFormatText:
		if _, err := fmt.Fprintf(o.w, "Workspace: %s\n", workspace.PaperID); err != nil {
			return err
		}
		if workspace.Source != nil {
			if _, err := fmt.Fprintf(o.w, "Source: %s (%s)\n", workspace.Source.URI, workspace.Source.Status); err != nil {
				return err
			}
		}
		if workspace.Digest != nil {
			if _, err := fmt.Fprintf(o.w, "Digest: %s\n", workspace.Digest.Title); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintln(o.w, "Digest: <none>"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(o.w, "\nNotes:"); err != nil {
			return err
		}
		if len(workspace.Notes) == 0 {
			if _, err := fmt.Fprintln(o.w, "- <none>"); err != nil {
				return err
			}
		}
		for _, note := range workspace.Notes {
			if _, err := fmt.Fprintf(o.w, "- %s: %s\n  %s\n", note.ID, note.Title, note.Body); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(o.w, "\nAnnotations:"); err != nil {
			return err
		}
		if len(workspace.Annotations) == 0 {
			if _, err := fmt.Fprintln(o.w, "- <none>"); err != nil {
				return err
			}
		}
		for _, annotation := range workspace.Annotations {
			if _, err := fmt.Fprintf(o.w, "- %s [%s]: %s\n  %s\n", annotation.ID, formatAnchor(annotation.Anchor), annotation.Title, annotation.Body); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(o.w, "\nResources:"); err != nil {
			return err
		}
		if len(workspace.Resources) == 0 {
			if _, err := fmt.Fprintln(o.w, "- <none>"); err != nil {
				return err
			}
		}
		for _, resource := range workspace.Resources {
			if _, err := fmt.Fprintf(o.w, "- [%s] %s -> %s\n", resource.Kind, resource.Title, resource.URI); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(o.w, "\nSimilar:"); err != nil {
			return err
		}
		if len(workspace.Similar) == 0 {
			if _, err := fmt.Fprintln(o.w, "- <none>"); err != nil {
				return err
			}
		}
		for _, similar := range workspace.Similar {
			line := similar.PaperID
			if strings.TrimSpace(similar.Title) != "" {
				line += ": " + similar.Title
			}
			if strings.TrimSpace(similar.Reason) != "" {
				line += " (" + similar.Reason + ")"
			}
			if _, err := fmt.Fprintf(o.w, "- %s\n", line); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *OutputWriter) printPlan(plan protocol.PlanResult) error {
	ready := make([]string, 0, len(plan.DAG.Nodes))
	_, err := fmt.Fprintf(o.w, "Plan ID: %s\nGoal: %s\nWill compare: %t\nNodes: %d\n\nDAG:\n", plan.PlanID, plan.Goal, plan.WillCompare, len(plan.DAG.Nodes))
	if err != nil {
		return err
	}
	for _, node := range plan.DAG.Nodes {
		if node.Status == protocol.NodeStatusReady {
			ready = append(ready, node.ID)
		}
		if _, err := fmt.Fprintf(o.w, "- %s: %s [%s, worker=%s, depends_on=%s]\n", node.ID, node.Goal, node.Kind, node.WorkerProfile, strings.Join(node.DependsOn, ",")); err != nil {
			return err
		}
	}
	if len(ready) > 0 {
		if _, err := fmt.Fprintf(o.w, "\nReady nodes: %s\n", strings.Join(ready, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(o.w, "\nRisks:"); err != nil {
		return err
	}
	for _, risk := range plan.Risks {
		if _, err := fmt.Fprintf(o.w, "- %s\n", risk); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(o.w, "\nSources:"); err != nil {
		return err
	}
	for _, source := range plan.SourceSummary {
		if _, err := fmt.Fprintf(o.w, "- %s (%s, pages=%d)\n", source.PaperID, source.SourceType, source.Inspection.PageCount); err != nil {
			return err
		}
	}
	return nil
}

func (o *OutputWriter) printApproval(approval protocol.ApprovalRequest) error {
	_, err := fmt.Fprintf(o.w, "Approval required\nPlan: %s\nCheckpoint: %s\nInterrupt: %s\nPending nodes: %s\nSummary: %s\n\n", approval.PlanID, approval.CheckpointID, approval.InterruptID, strings.Join(approval.PendingNodeIDs, ", "), approval.Summary)
	return err
}

func formatAnchor(anchor protocol.AnchorRef) string {
	switch anchor.Kind {
	case protocol.AnchorKindPage:
		return fmt.Sprintf("page %d", anchor.Page)
	case protocol.AnchorKindSnippet:
		return "snippet: " + anchor.Snippet
	case protocol.AnchorKindSection:
		return "section: " + anchor.Section
	default:
		return string(anchor.Kind)
	}
}
