package tui

import (
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type PromptMode string

const (
	PromptModePrompt  PromptMode = "prompt"
	PromptModeCommand PromptMode = "command"
	PromptModeShell   PromptMode = "shell"
)

type PromptHistoryEntry struct {
	Value string
	Mode  PromptMode
}

type PromptController struct {
	value       string
	draft       string
	mode        PromptMode
	history     []PromptHistoryEntry
	historyMode PromptMode
	historyIdx  int
	navigating  bool
}

func NewPromptController() PromptController {
	return PromptController{mode: PromptModePrompt}
}

func (p *PromptController) SetValue(value string) {
	p.value = value
	if !p.navigating {
		p.draft = value
	}
	p.mode = DetectPromptMode(value)
}

func (p *PromptController) Value() string {
	return p.value
}

func (p *PromptController) Mode() PromptMode {
	return p.mode
}

func (p *PromptController) SetHistory(entries []PromptHistoryEntry) {
	p.history = append(p.history[:0], entries...)
	p.historyIdx = 0
	p.navigating = false
}

func (p *PromptController) HistoryPrev() bool {
	filtered := p.filteredHistory()
	if len(filtered) == 0 {
		return false
	}
	if !p.navigating {
		p.draft = p.value
		p.historyMode = p.mode
		p.historyIdx = 0
		p.navigating = true
	} else if p.historyIdx < len(filtered)-1 {
		p.historyIdx++
	}
	p.value = filtered[p.historyIdx].Value
	p.mode = DetectPromptMode(p.value)
	return true
}

func (p *PromptController) HistoryNext() bool {
	if !p.navigating {
		return false
	}
	filtered := p.filteredHistory()
	if p.historyIdx > 0 {
		p.historyIdx--
		p.value = filtered[p.historyIdx].Value
		p.mode = DetectPromptMode(p.value)
		return true
	}
	p.value = p.draft
	p.mode = DetectPromptMode(p.value)
	p.historyIdx = 0
	p.navigating = false
	return true
}

func (p *PromptController) CancelHistory() {
	if p.navigating {
		p.value = p.draft
		p.mode = DetectPromptMode(p.value)
	}
	p.historyIdx = 0
	p.navigating = false
}

func (p *PromptController) filteredHistory() []PromptHistoryEntry {
	mode := p.mode
	if p.navigating {
		mode = p.historyMode
	}
	out := make([]PromptHistoryEntry, 0, len(p.history))
	for _, entry := range p.history {
		if entry.Mode != "" && entry.Mode != mode {
			continue
		}
		if strings.TrimSpace(entry.Value) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func DetectPromptMode(value string) PromptMode {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(value, "/"):
		return PromptModeCommand
	case strings.HasPrefix(value, "!"), strings.HasPrefix(value, "$"):
		return PromptModeShell
	default:
		return PromptModePrompt
	}
}

func PromptModeFromTranscript(mode protocol.TranscriptInputMode) PromptMode {
	switch mode {
	case protocol.TranscriptInputCommand:
		return PromptModeCommand
	case protocol.TranscriptInputShell:
		return PromptModeShell
	default:
		return PromptModePrompt
	}
}
