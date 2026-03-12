package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"papersilm/pkg/protocol"
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
		if result.Plan != nil && result.Comparison == nil && len(result.Digests) == 0 {
			return o.printPlan(*result.Plan)
		}
		if result.Comparison != nil {
			if _, err := fmt.Fprintln(o.w, result.Comparison.Markdown); err != nil {
				return err
			}
		}
		if result.Comparison == nil {
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

func (o *OutputWriter) printPlan(plan protocol.PlanResult) error {
	_, err := fmt.Fprintf(o.w, "Goal: %s\nWill compare: %t\n\nTool plan:\n", plan.Goal, plan.WillCompare)
	if err != nil {
		return err
	}
	for _, step := range plan.ToolPlan {
		if _, err := fmt.Fprintf(o.w, "- %s\n", step); err != nil {
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
