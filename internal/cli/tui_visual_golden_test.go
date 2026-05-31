package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type visualScenario struct {
	Name      string
	Width     int
	Height    int
	Theme     config.ThemeSetting
	Setup     func(*tuiModel)
	Required  []string
	Forbidden []string
	FooterMax int
}

var visualFixedTime = time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

func TestTUIVisualGolden(t *testing.T) {
	for _, scenario := range visualScenarios() {
		scenario := scenario
		t.Run(scenario.GoldenName(), func(t *testing.T) {
			model := newVisualTestTUIModel(scenario.Theme, scenario.Width, scenario.Height)
			if scenario.Setup != nil {
				scenario.Setup(model)
			}
			model.reflow()
			got := renderVisualScenario(model)
			path := filepath.Join("testdata", "tui_visual", scenario.GoldenName()+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create golden dir: %v", err)
				}
				if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v; run UPDATE_GOLDEN=1 go test ./internal/cli -run TestTUIVisualGolden", path, err)
			}
			want := strings.TrimRight(string(raw), "\n")
			if got != want {
				t.Fatalf("visual snapshot mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", scenario.GoldenName(), got, want)
			}
		})
	}
}

func TestTUIVisualInvariants(t *testing.T) {
	for _, scenario := range visualScenarios() {
		scenario := scenario
		t.Run(scenario.GoldenName(), func(t *testing.T) {
			model := newVisualTestTUIModel(scenario.Theme, scenario.Width, scenario.Height)
			if scenario.Setup != nil {
				scenario.Setup(model)
			}
			model.reflow()
			view := renderVisualScenario(model)
			assertVisualInvariants(t, scenario, model, view)
		})
	}
}

func visualScenarios() []visualScenario {
	const height = 28
	coreThemes := []config.ThemeSetting{config.ThemeDark, config.ThemeLight}
	darkOnly := []config.ThemeSetting{config.ThemeDark}
	widths := []int{80, 120}

	var scenarios []visualScenario
	add := func(name string, themes []config.ThemeSetting, setup func(*tuiModel), required, forbidden []string) {
		for _, width := range widths {
			for _, theme := range themes {
				scenarios = append(scenarios, visualScenario{
					Name:      name,
					Width:     width,
					Height:    height,
					Theme:     theme,
					Setup:     setup,
					Required:  required,
					Forbidden: forbidden,
					FooterMax: 2,
				})
			}
		}
	}

	add("idle_empty", coreThemes, nil,
		[]string{"papersilm", "workspace ready", "› Ask about workspace or papers", "? shortcuts"},
		nil)
	add("typing_prompt", darkOnly, func(m *tuiModel) {
		m.setPromptValue("draft-visible-input-row")
	},
		[]string{"draft-visible-input-row"},
		[]string{"? shortcuts"})
	add("slash_suggestions", coreThemes, func(m *tuiModel) {
		m.setPromptValue("/")
		m.refreshSuggestions()
	},
		[]string{"/help", "/commands", "› /"},
		nil)
	add("command_drawer", darkOnly, func(m *tuiModel) {
		_ = m.openCommandPalette()
	},
		[]string{"Command Palette", "/help", "Esc close"},
		nil)
	add("model_drawer", darkOnly, setupVisualModelDrawer,
		[]string{"Model Picker", "gpt-visual", "Enter switch model"},
		nil)
	add("assistant_markdown", coreThemes, setupVisualAssistantMarkdown,
		[]string{"Workspace Summary", "README.md", "fmt.Println"},
		nil)
	add("user_prompt", darkOnly, func(m *tuiModel) {
		appendVisualTranscript(m, "user", protocol.TranscriptEntryUser, "You", "Summarize this workspace and identify risks.")
	},
		[]string{"Summarize this workspace and identify risks."},
		nil)
	add("workspace_activity", darkOnly, setupVisualWorkspaceActivity,
		[]string{"· Inspecting workspace", "1 search", "1 read"},
		[]string{"⏺"})
	add("permission_edit", coreThemes, setupVisualPermissionEdit,
		[]string{"Edit README.md", "Do you want to make this edit?", "Enter select"},
		[]string{"Enter yes"})
	add("approved_decision", darkOnly, func(m *tuiModel) {
		appendVisualTranscript(m, "approved", protocol.TranscriptEntryApproval, "✓ Approved", "README.md",
			withVisualSubtype(transcriptSubtypeApprovalApproved))
	},
		[]string{"✓ Approved", "README.md"},
		[]string{"Error"})
	add("rejected_decision", darkOnly, func(m *tuiModel) {
		appendVisualTranscript(m, "rejected", protocol.TranscriptEntryApproval, "Tool use rejected", "",
			withVisualSubtype(transcriptSubtypeApprovalRejected))
	},
		[]string{"Tool use rejected"},
		[]string{"Error"})
	add("transcript_search", darkOnly, setupVisualTranscriptSearch,
		[]string{"/ search transcript", "Workspace summary", "summary"},
		nil)
	add("footer_hidden_hints", coreThemes, func(m *tuiModel) {
		m.setHintsVisible(false)
	},
		[]string{"› Ask about workspace or papers"},
		[]string{"? shortcuts"})

	return scenarios
}

func newVisualTestTUIModel(theme config.ThemeSetting, width, height int) *tuiModel {
	cfg := config.Default()
	cfg.Theme = theme
	provider := cfg.Providers[config.DefaultProviderProfile]
	provider.Model = "gpt-visual"
	cfg.Providers[config.DefaultProviderProfile] = provider
	cfg.Provider = provider

	runtime := &tuiRuntimeManager{cfg: cfg}
	model := newTUIModel(context.Background(), runtime, protocol.SessionSnapshot{
		Meta: protocol.SessionMeta{
			SessionID:       "sess_visual",
			Name:            "visual baseline",
			State:           protocol.SessionStatePlanned,
			PermissionMode:  protocol.PermissionModeConfirm,
			WorkspaceRoot:   "/workspace/papersilm",
			WorkspaceID:     protocol.DefaultWorkspaceID,
			ProviderProfile: config.DefaultProviderProfile,
			Model:           "gpt-visual",
			Language:        "zh",
			Style:           "distill",
			CreatedAt:       visualFixedTime,
			UpdatedAt:       visualFixedTime,
		},
		Workspace: &protocol.WorkspaceSummary{
			WorkspaceID:   protocol.DefaultWorkspaceID,
			Root:          "/workspace/papersilm",
			Name:          "papersilm",
			FileCount:     42,
			TextFileCount: 12,
			IndexedAt:     visualFixedTime,
		},
	})
	model.width = width
	model.height = height
	model.ready = true
	model.reflow()
	return model
}

func setupVisualModelDrawer(m *tuiModel) {
	choices := []tuiChoice{
		{Label: "gpt-visual", Value: "gpt-visual", Detail: "current model"},
		{Label: "gpt-visual-large", Value: "gpt-visual-large", Detail: "larger context"},
	}
	m.focus = tuiFocusModal
	m.modal = tuiModalState{
		Kind:      tuiModalModels,
		Title:     "Model Picker",
		Provider:  config.DefaultProviderProfile,
		Message:   "default · openai",
		All:       choices,
		Visible:   choices,
		Selection: 0,
	}
	m.modalIn.SetValue("gpt")
	m.modalIn.Placeholder = "gpt-visual"
}

func setupVisualAssistantMarkdown(m *tuiModel) {
	appendVisualTranscript(m, "assistant_md", protocol.TranscriptEntryAssistant, "Assistant", "## Workspace Summary\n\n- Read `README.md`\n- Found a small CLI\n\n```go\nfmt.Println(\"ok\")\n```",
		withVisualMarkdown())
}

func setupVisualWorkspaceActivity(m *tuiModel) {
	appendVisualTranscript(m, "progress_search", protocol.TranscriptEntryProgress, "Progress", "started · tool=workspace_search · node=search_readme",
		withVisualVisibility(protocol.TranscriptVisibilityActivity),
		withVisualPresentation(protocol.TranscriptPresentationGrouped))
	appendVisualTranscript(m, "progress_read", protocol.TranscriptEntryProgress, "Progress", "started · tool=workspace_inspect · node=read_readme",
		withVisualVisibility(protocol.TranscriptVisibilityActivity),
		withVisualPresentation(protocol.TranscriptPresentationGrouped))
}

func setupVisualPermissionEdit(m *tuiModel) {
	m.snapshot.Meta.State = protocol.SessionStateAwaitingApproval
	m.snapshot.Meta.ApprovalPending = true
	m.snapshot.Approval = &protocol.ApprovalRequest{
		ActiveRequestID: "req_edit",
		Requests: []protocol.PermissionRequest{
			{
				RequestID:  "req_edit",
				Tool:       string(protocol.NodeKindWorkspaceEdit),
				Operation:  "write",
				Title:      "Edit README.md",
				Subtitle:   "README.md",
				Question:   "Do you want to make this edit?",
				Summary:    "Prepared workspace edit preview",
				TargetPath: "README.md",
				Preview: protocol.PermissionPreview{
					Kind: "diff",
					Diff: "--- README.md\n+++ README.md\n-old line\n+new line\n+more context\n+final line",
				},
				Options: []protocol.PermissionOption{
					{Value: tuiPermissionAcceptOnce, Label: "Yes", Scope: "node", Feedback: tuiPermissionFeedbackAccept, Description: "Allow this edit once"},
					{Value: tuiPermissionAcceptSession, Label: "Yes, during this session", Scope: "path", Feedback: tuiPermissionFeedbackAccept, Description: "Allow edits to this file for this session"},
					{Value: tuiPermissionReject, Label: "No", Scope: "node", Feedback: tuiPermissionFeedbackReject, Description: "Reject this edit"},
				},
			},
		},
	}
}

func setupVisualTranscriptSearch(m *tuiModel) {
	appendVisualTranscript(m, "user", protocol.TranscriptEntryUser, "You", "summarize workspace")
	appendVisualTranscript(m, "assistant", protocol.TranscriptEntryAssistant, "Assistant", "Workspace summary is ready.")
	m.openTranscriptScreen(true)
	m.searchIn.SetValue("summary")
	m.refreshTranscriptSearch()
}

func appendVisualTranscript(m *tuiModel, id string, kind protocol.TranscriptEntryType, title, body string, opts ...func(*protocol.TranscriptEntry)) {
	entry := protocol.TranscriptEntry{
		ID:        id,
		SessionID: "sess_visual",
		Type:      kind,
		Title:     title,
		Body:      body,
		CreatedAt: visualFixedTime,
	}
	for _, opt := range opts {
		opt(&entry)
	}
	m.appendTranscript(entry, false)
}

func withVisualSubtype(subtype string) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Subtype = subtype
	}
}

func withVisualMarkdown() func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Markdown = true
	}
}

func withVisualVisibility(visibility protocol.TranscriptVisibility) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Visibility = visibility
	}
}

func withVisualPresentation(presentation protocol.TranscriptPresentation) func(*protocol.TranscriptEntry) {
	return func(entry *protocol.TranscriptEntry) {
		entry.Presentation = presentation
	}
}

func renderVisualScenario(model *tuiModel) string {
	view := model.renderMainScreen()
	if model.screen == tuiScreenTranscript {
		view = model.renderTranscriptScreen()
	}
	return normalizeVisualSnapshot(view)
}

func assertVisualInvariants(t *testing.T, scenario visualScenario, model *tuiModel, view string) {
	t.Helper()
	required := append([]string{}, scenario.Required...)
	for _, want := range required {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in %s:\n%s", want, scenario.GoldenName(), view)
		}
	}

	forbidden := append([]string{}, globalVisualForbiddenStrings()...)
	forbidden = append(forbidden, scenario.Forbidden...)
	for _, bad := range forbidden {
		if strings.Contains(view, bad) {
			t.Fatalf("did not expect %q in %s:\n%s", bad, scenario.GoldenName(), view)
		}
	}

	for lineNo, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > scenario.Width {
			t.Fatalf("line %d in %s exceeds width %d: got %d: %q", lineNo+1, scenario.GoldenName(), scenario.Width, got, line)
		}
	}

	if scenario.FooterMax > 0 {
		footer := normalizeVisualSnapshot(model.renderFooter())
		if got := len(strings.Split(footer, "\n")); got > scenario.FooterMax {
			t.Fatalf("footer in %s exceeds %d lines: got %d: %q", scenario.GoldenName(), scenario.FooterMax, got, footer)
		}
	}

	if scenario.Name == "slash_suggestions" {
		if got := countSuggestionRows(view); got > 5 {
			t.Fatalf("expected at most 5 suggestion rows, got %d:\n%s", got, view)
		}
	}
}

func globalVisualForbiddenStrings() []string {
	return []string{
		"session created",
		"session loaded",
		"node=",
		"tool=",
		"activity.grouped",
		"Progress ·",
		"Assistant ·",
		"assistant ·",
		"You ·",
		"you ·",
		"? for shortcuts",
		"Enter yes",
		"┌",
		"┐",
		"└",
		"┘",
	}
}

func countSuggestionRows(view string) int {
	count := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, " – ") && strings.Contains(line, "/") {
			count++
		}
	}
	return count
}

func (s visualScenario) GoldenName() string {
	return fmt.Sprintf("%s-%s-%d", s.Name, s.Theme, s.Width)
}

var (
	visualCSIRegexp = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	visualOSCRegexp = regexp.MustCompile(`\x1b\][^\a]*(?:\a|\x1b\\)`)
)

func normalizeVisualSnapshot(view string) string {
	view = visualOSCRegexp.ReplaceAllString(view, "")
	view = visualCSIRegexp.ReplaceAllString(view, "")
	view = strings.ReplaceAll(view, "\r\n", "\n")
	view = strings.ReplaceAll(view, "\r", "\n")
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
