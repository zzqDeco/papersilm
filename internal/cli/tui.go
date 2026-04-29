package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	tuiui "github.com/zzqDeco/papersilm/internal/cli/tui"
	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type tuiItemKind string

const (
	tuiItemUser      tuiItemKind = "user"
	tuiItemAssistant tuiItemKind = "assistant"
	tuiItemSystem    tuiItemKind = "system"
	tuiItemProgress  tuiItemKind = "progress"
	tuiItemApproval  tuiItemKind = "approval"
	tuiItemError     tuiItemKind = "error"
)

type tuiModalKind string

const (
	tuiModalNone      tuiModalKind = ""
	tuiModalCommands  tuiModalKind = "commands"
	tuiModalProviders tuiModalKind = "providers"
	tuiModalModels    tuiModalKind = "models"
)

type tuiTimelineItem struct {
	ID        string
	Kind      tuiItemKind
	Subtype   string
	Title     string
	Body      string
	Markdown  bool
	Compact   bool
	CreatedAt time.Time
}

type tuiChoice struct {
	Label    string
	Value    string
	Detail   string
	Disabled bool
}

type tuiApprovalAction string

const (
	tuiApprovalApprove tuiApprovalAction = "approve"
	tuiApprovalInspect tuiApprovalAction = "inspect"
	tuiApprovalReject  tuiApprovalAction = "reject"
)

const tuiPromptPlaceholder = "Ask about current workspace or /commands"

type tuiApprovalOption struct {
	Label    string
	Detail   string
	Action   tuiApprovalAction
	Command  string
	Disabled bool
}

type tuiModalState struct {
	Kind      tuiModalKind
	Title     string
	Provider  string
	Message   string
	Loading   bool
	All       []tuiChoice
	Visible   []tuiChoice
	Selection int
}

type tuiEventMsg struct {
	Event protocol.StreamEvent
}

type tuiExecDoneMsg struct {
	Input       string
	Pane        bool
	PaneTitle   string
	SkipHistory bool
	Before      protocol.SessionSnapshot
	After       protocol.SessionSnapshot
	Text        string
	Err         error
}

type tuiDiscoverModelsMsg struct {
	Profile string
	Models  []string
	Err     error
}

type tuiSwitchProviderMsg struct {
	Profile string
	Model   string
	After   protocol.SessionSnapshot
	Err     error
}

type tuiStyles struct {
	theme                  config.ThemeSetting
	markdownStyle          string
	background             lipgloss.Style
	body                   lipgloss.Style
	header                 lipgloss.Style
	headerMuted            lipgloss.Style
	headerAccent           lipgloss.Style
	headerStatus           lipgloss.Style
	userShell              lipgloss.Style
	userLabel              lipgloss.Style
	assistantLabel         lipgloss.Style
	approvalShell          lipgloss.Style
	approvalLabel          lipgloss.Style
	successShell           lipgloss.Style
	successLabel           lipgloss.Style
	rejectionShell         lipgloss.Style
	rejectionLabel         lipgloss.Style
	errorShell             lipgloss.Style
	errorLabel             lipgloss.Style
	paneDivider            lipgloss.Style
	paneTitle              lipgloss.Style
	paneBody               lipgloss.Style
	inputShell             lipgloss.Style
	footer                 lipgloss.Style
	footerMuted            lipgloss.Style
	footerAccent           lipgloss.Style
	keycap                 lipgloss.Style
	systemLine             lipgloss.Style
	progressLine           lipgloss.Style
	suggestionMarker       lipgloss.Style
	suggestionLabel        lipgloss.Style
	suggestionDetail       lipgloss.Style
	suggestionActiveLabel  lipgloss.Style
	suggestionActiveDetail lipgloss.Style
	modalShell             lipgloss.Style
	modalTitle             lipgloss.Style
	modalMessage           lipgloss.Style
	modalHint              lipgloss.Style
	modalDisabled          lipgloss.Style
}

type tuiModel struct {
	ctx     context.Context
	runtime *tuiRuntimeManager

	snapshot protocol.SessionSnapshot

	timeline   viewport.Model
	transcript viewport.Model
	pane       viewport.Model
	input      textarea.Model
	modalIn    textinput.Model
	searchIn   textinput.Model
	historyIn  textinput.Model

	items                []tuiTimelineItem
	messageViewport      tuiui.MessageViewport
	messageStore         tuiui.MessageStore
	messagePipeline      tuiui.MessagePipeline
	promptController     tuiui.PromptController
	transcriptScreen     tuiui.TranscriptScreen
	history              []string
	suggestions          []tuiSuggestion
	sel                  int
	screen               tuiScreen
	focus                tuiFocus
	historyState         tuiHistoryState
	historyMatches       []protocol.TranscriptEntry
	historySelection     int
	historyStatus        string
	historyDraft         string
	workspaceName        string
	workspaceDisplayPath string
	activityCount        int
	activityStarted      time.Time
	activityStats        map[string]int
	approvalSelection    int

	paneVisible bool
	paneTitle   string
	paneBody    string

	modal tuiModalState

	width        int
	height       int
	ready        bool
	busy         bool
	mainStatus   string
	autoScroll   bool
	unread       int
	hintsVisible bool

	styles tuiStyles
}

type tuiStartupError struct {
	attempts []string
}

func (e *tuiStartupError) Error() string {
	if e == nil || len(e.attempts) == 0 {
		return ErrTUIStartup.Error()
	}
	return fmt.Sprintf("%s: %s", ErrTUIStartup, strings.Join(e.attempts, "; "))
}

func (e *tuiStartupError) Is(target error) bool {
	return target == ErrTUIStartup
}

func RunTUI(ctx context.Context, opts TUIOptions) error {
	manager, snapshot, err := newTUIRuntimeManager(ctx, opts)
	if err != nil {
		return err
	}
	firstErr := runTUIProgram(ctx, manager, snapshot, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if firstErr == nil {
		return nil
	}
	secondErr := runTUIProgram(ctx, manager, snapshot, tea.WithAltScreen())
	if secondErr == nil {
		return nil
	}
	return &tuiStartupError{
		attempts: []string{
			fmt.Sprintf("mouse mode failed: %v", firstErr),
			fmt.Sprintf("plain mode failed: %v", secondErr),
		},
	}
}

func runTUIProgram(
	ctx context.Context,
	manager *tuiRuntimeManager,
	snapshot protocol.SessionSnapshot,
	opts ...tea.ProgramOption,
) error {
	model, err := loadTUIModel(ctx, manager, snapshot)
	if err != nil {
		return err
	}
	manager.drainPendingStartupEvents()
	program := tea.NewProgram(model, opts...)
	if _, err := program.Run(); err != nil && err != tea.ErrProgramKilled {
		return err
	}
	return nil
}

func loadTUIModel(ctx context.Context, manager *tuiRuntimeManager, snapshot protocol.SessionSnapshot) (*tuiModel, error) {
	model := newTUIModel(ctx, manager, snapshot)
	transcript, err := manager.loadTranscript(snapshot.Meta.SessionID)
	if err != nil {
		return nil, err
	}
	if len(transcript) > 0 {
		model.hydrateTranscript(transcript)
		return model, nil
	}
	events, err := manager.loadRecentEvents(snapshot.Meta.SessionID, 200)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		model.hydrateTimeline(events)
	}
	return model, nil
}

func newTUIModel(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot) *tuiModel {
	input := textarea.New()
	input.Placeholder = ""
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.CharLimit = 0
	input.KeyMap.InsertNewline.SetKeys("ctrl+j")

	modalIn := textinput.New()
	modalIn.Placeholder = "Filter"
	searchIn := textinput.New()
	searchIn.Placeholder = "Search transcript"
	historyIn := textinput.New()
	historyIn.Placeholder = "Search prompt history"

	workspaceRoot, _ := os.Getwd()
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	workspaceName := filepath.Base(workspaceRoot)
	if workspaceName == "." || workspaceName == string(filepath.Separator) {
		workspaceName = "workspace"
	}
	if snapshot.Workspace != nil {
		if strings.TrimSpace(snapshot.Workspace.Root) != "" {
			workspaceRoot = snapshot.Workspace.Root
		}
		if strings.TrimSpace(snapshot.Workspace.Name) != "" {
			workspaceName = snapshot.Workspace.Name
		}
	}

	model := &tuiModel{
		ctx:                  ctx,
		runtime:              runtime,
		snapshot:             snapshot,
		timeline:             viewport.New(0, 0),
		transcript:           viewport.New(0, 0),
		pane:                 viewport.New(0, 0),
		input:                input,
		modalIn:              modalIn,
		searchIn:             searchIn,
		historyIn:            historyIn,
		messagePipeline:      tuiui.NewMessagePipeline(),
		promptController:     tuiui.NewPromptController(),
		screen:               tuiScreenMain,
		focus:                tuiFocusInput,
		autoScroll:           true,
		hintsVisible:         true,
		workspaceName:        workspaceName,
		workspaceDisplayPath: workspaceRoot,
		activityStats:        make(map[string]int),
	}
	model.applyTheme(runtime.cfg.Theme)
	model.timeline.MouseWheelEnabled = true
	model.transcript.MouseWheelEnabled = true
	model.pane.MouseWheelEnabled = true
	model.refreshSuggestions()
	if len(model.items) == 0 {
		model.ensureWelcomeItem()
	}
	return model
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), waitForTUIEvent(m.runtime.sink.ch))
}

func (m *tuiModel) applyTheme(theme config.ThemeSetting) {
	m.styles = newTUIStyles(theme)
	m.runtime.cfg.Theme = theme
	applyTextareaTheme(&m.input, m.styles)
	applyTextInputTheme(&m.modalIn, m.styles)
	applyTextInputTheme(&m.searchIn, m.styles)
	applyTextInputTheme(&m.historyIn, m.styles)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ready = true
		m.width = msg.Width
		m.height = msg.Height
		m.reflow()
		return m, nil
	case tuiEventMsg:
		m.appendEvent(msg.Event)
		m.reflow()
		return m, waitForTUIEvent(m.runtime.sink.ch)
	case tuiExecDoneMsg:
		m.busy = false
		if msg.Err != nil {
			m.appendError(msg.Err)
			m.setMainStatus(msg.Err.Error())
			m.reflow()
			return m, nil
		}
		m.snapshot = msg.After
		m.syncApprovalSelection()
		if theme, ok := themeCommandValue(msg.Input); ok {
			m.applyTheme(theme)
		}
		if !msg.SkipHistory {
			m.recordHistory(msg.Input)
		}
		if msg.Pane {
			m.openPane(msg.PaneTitle, msg.Text)
			m.setMainStatus(fmt.Sprintf("%s opened", msg.PaneTitle))
		} else {
			for _, entry := range executionToTranscriptEntries(msg.Input, msg.Before, msg.After, msg.Text) {
				if strings.TrimSpace(entry.Body) == "" && strings.TrimSpace(entry.Title) == "" {
					continue
				}
				m.appendTranscript(entry, true)
			}
			m.paneVisible = false
			m.setMainStatus(statusForSnapshot(m.snapshot))
		}
		m.refreshSuggestions()
		m.reflow()
		return m, nil
	case tuiDiscoverModelsMsg:
		if m.modal.Kind != tuiModalModels || msg.Profile != m.modal.Provider {
			return m, nil
		}
		m.modal.Loading = false
		if msg.Err != nil {
			m.modal.Message = fmt.Sprintf("Model discovery failed: %v. Enter a model name manually.", msg.Err)
		} else {
			provider, _ := m.runtime.providerProfile(msg.Profile)
			models := uniqueStrings(append([]string{provider.Model}, msg.Models...))
			choices := make([]tuiChoice, 0, len(models))
			for _, model := range models {
				if strings.TrimSpace(model) == "" {
					continue
				}
				choices = append(choices, tuiChoice{
					Label:  model,
					Value:  model,
					Detail: "Discovered model",
				})
			}
			m.modal.All = choices
		}
		m.refreshModalChoices()
		m.reflow()
		return m, nil
	case tuiSwitchProviderMsg:
		m.busy = false
		if msg.Err != nil {
			m.appendError(msg.Err)
			m.setMainStatus(msg.Err.Error())
			m.reflow()
			return m, nil
		}
		m.snapshot = msg.After
		m.syncApprovalSelection()
		m.appendTranscript(newTranscriptEntry(
			m.snapshot.Meta.SessionID,
			protocol.TranscriptEntrySystem,
			"Runtime",
			fmt.Sprintf("Switched to %s / %s", msg.Profile, msg.Model),
			withTranscriptSubtype("runtime"),
		), true)
		m.closeModal()
		m.setMainStatus("Provider and model updated")
		m.refreshSuggestions()
		m.reflow()
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *tuiModel) View() string {
	if !m.ready {
		return "Loading papersilm workspace..."
	}
	if m.screen == tuiScreenTranscript {
		view := m.renderTranscriptScreen()
		if m.modal.Kind != tuiModalNone {
			return m.renderModalOver(view)
		}
		return m.styles.background.Width(m.width).Height(m.height).Render(view)
	}

	main := m.renderMainScreen()
	if m.modal.Kind != tuiModalNone {
		return m.renderModalOver(main)
	}
	return m.styles.background.Width(m.width).Height(m.height).Render(main)
}

func (m *tuiModel) renderMainScreen() string {
	bottomParts := []string{}
	if approval := m.renderApprovalStickyPanel(); strings.TrimSpace(approval) != "" {
		bottomParts = append(bottomParts, approval)
	}
	bottomParts = append(bottomParts, m.renderInput(), m.renderFooter())
	bottom := lipgloss.JoinVertical(lipgloss.Left, bottomParts...)
	suggestions := m.renderSuggestions()
	pill := ""
	if !m.autoScroll {
		pill = m.renderScrollPill()
	}
	return tuiui.RenderFullscreenLayout(tuiui.FullscreenLayout{
		Width:         m.width,
		Header:        m.renderHeader(),
		StickyHeader:  m.renderStickyPromptHeader(),
		Scrollable:    m.timeline.View(),
		Bottom:        bottom,
		Pane:          m.renderPane(),
		PromptOverlay: suggestions,
		ScrollPill:    pill,
	})
}

func (m *tuiModel) renderScrollPill() string {
	text := "Jump to bottom ↓"
	if m.unread > 0 {
		label := "new message"
		if m.unread != 1 {
			label = "new messages"
		}
		text = fmt.Sprintf("%d %s ↓", m.unread, label)
	}
	return m.styles.footerAccent.Render(" " + text + " ")
}

func (m *tuiModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal.Kind != tuiModalNone {
		return m, nil
	}
	if m.screen == tuiScreenTranscript {
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	}
	if m.paneVisible {
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.timeline, cmd = m.timeline.Update(msg)
	m.updateScrollState()
	return m, cmd
}

func (m *tuiModel) submitInput() (tea.Model, tea.Cmd) {
	if m.busy {
		m.setMainStatus("A run is already in progress")
		m.reflow()
		return m, nil
	}
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return m, nil
	}
	if visible, ok, err := parseHintsCommand(line, m.hintsVisible); ok {
		m.consumeSubmittedInput()
		if err != nil {
			m.setMainStatus(err.Error())
			m.reflow()
			return m, nil
		}
		m.setHintsVisible(visible)
		m.reflow()
		return m, nil
	}
	if theme, ok, err := parseThemeCommand(line); ok {
		m.consumeSubmittedInput()
		if err != nil {
			m.setMainStatus(err.Error())
			m.reflow()
			return m, nil
		}
		if err := m.runtime.cfg.SetTheme(theme); err != nil {
			m.setMainStatus(err.Error())
			m.reflow()
			return m, nil
		}
		if err := config.Save(config.ConfigPath(m.runtime.cfg.BaseDir), m.runtime.cfg); err != nil {
			m.setMainStatus(err.Error())
			m.reflow()
			return m, nil
		}
		m.applyTheme(theme)
		m.setMainStatus(fmt.Sprintf("Theme set to %s", theme))
		m.reflow()
		return m, nil
	}
	switch line {
	case "/exit", "/quit":
		return m, tea.Quit
	case "/transcript":
		m.consumeSubmittedInput()
		m.openTranscriptScreen(false)
		m.reflow()
		return m, nil
	case "/commands":
		m.consumeSubmittedInput()
		cmd := m.openCommandPalette()
		m.reflow()
		return m, cmd
	case "/model":
		m.consumeSubmittedInput()
		cmd := m.openProviderModal()
		m.reflow()
		return m, cmd
	}

	line = m.consumeSubmittedInput()

	if strings.HasPrefix(line, "/") {
		title, pane := classifyPaneCommand(line)
		m.appendTranscript(newTranscriptEntry(
			m.snapshot.Meta.SessionID,
			protocol.TranscriptEntryCommand,
			"Command",
			line,
			withTranscriptInputMode(protocol.TranscriptInputCommand),
		), true)
		m.busy = true
		m.setMainStatus("Running command...")
		m.reflow()
		return m, runSlashCmd(m.ctx, m.runtime, m.snapshot, line, title, pane)
	}

	m.appendTranscript(newTranscriptEntry(
		m.snapshot.Meta.SessionID,
		protocol.TranscriptEntryUser,
		"You",
		line,
		withTranscriptInputMode(protocol.TranscriptInputPrompt),
	), true)
	m.busy = true
	m.setMainStatus("Running task...")
	m.reflow()
	return m, runPromptCmd(m.ctx, m.runtime, m.snapshot, line)
}

func (m *tuiModel) commitModalSelection() (tea.Model, tea.Cmd) {
	switch m.modal.Kind {
	case tuiModalCommands:
		if len(m.modal.Visible) == 0 {
			return m, nil
		}
		choice := m.modal.Visible[m.modal.Selection]
		m.closeModal()
		m.input.SetValue(choice.Value)
		m.refreshSuggestions()
		m.reflow()
		return m, m.input.Focus()
	case tuiModalProviders:
		if len(m.modal.Visible) == 0 {
			return m, nil
		}
		choice := m.modal.Visible[m.modal.Selection]
		if choice.Disabled {
			m.modal.Message = "This provider profile is missing required credentials."
			m.reflow()
			return m, nil
		}
		cmd := m.openModelModal(choice.Value)
		m.reflow()
		return m, cmd
	case tuiModalModels:
		modelName := strings.TrimSpace(m.modalIn.Value())
		if len(m.modal.Visible) > 0 {
			modelName = strings.TrimSpace(m.modal.Visible[m.modal.Selection].Value)
		}
		if modelName == "" {
			modelName = strings.TrimSpace(m.modalIn.Value())
		}
		if modelName == "" {
			m.modal.Message = "Enter a model name or pick one from the list."
			m.reflow()
			return m, nil
		}
		m.busy = true
		m.setMainStatus("Switching runtime...")
		m.reflow()
		return m, switchProviderCmd(m.runtime, m.snapshot, m.modal.Provider, modelName)
	default:
		return m, nil
	}
}

func (m *tuiModel) openCommandPalette() tea.Cmd {
	m.input.Blur()
	m.focus = tuiFocusModal
	m.modal = tuiModalState{
		Kind:    tuiModalCommands,
		Title:   "Command Palette",
		Message: "Filter commands, recipes, and current session context.",
	}
	m.modalIn.SetValue("")
	m.modalIn.Placeholder = "Type to filter commands or recipes"
	m.refreshModalChoices()
	return m.modalIn.Focus()
}

func (m *tuiModel) openProviderModal() tea.Cmd {
	m.input.Blur()
	m.focus = tuiFocusModal
	choices := make([]tuiChoice, 0, len(m.runtime.profileNames()))
	for _, name := range m.runtime.profileNames() {
		provider, _ := m.runtime.providerProfile(name)
		blocked, reason := providerProfileBlocked(provider)
		detail := fmt.Sprintf("%s · model=%s", provider.Provider, provider.Model)
		if blocked {
			detail = detail + " · " + reason
		}
		choices = append(choices, tuiChoice{
			Label:    name,
			Value:    name,
			Detail:   detail,
			Disabled: blocked,
		})
	}
	m.modal = tuiModalState{
		Kind:    tuiModalProviders,
		Title:   "Provider Profiles",
		Message: "Pick a configured provider profile.",
		All:     choices,
	}
	m.modalIn.SetValue("")
	m.modalIn.Placeholder = "Filter provider profiles"
	m.refreshModalChoices()
	return m.modalIn.Focus()
}

func (m *tuiModel) openModelModal(profile string) tea.Cmd {
	provider, _ := m.runtime.providerProfile(profile)
	m.focus = tuiFocusModal
	m.modal = tuiModalState{
		Kind:     tuiModalModels,
		Title:    "Model Picker",
		Provider: profile,
		Message:  fmt.Sprintf("Profile: %s (%s). Enter a model name or pick a discovered one.", profile, provider.Provider),
		Loading:  true,
		All:      nil,
	}
	m.modalIn.SetValue("")
	m.modalIn.Placeholder = provider.Model
	m.refreshModalChoices()
	return tea.Batch(m.modalIn.Focus(), discoverModelsCmd(m.runtime, profile))
}

func (m *tuiModel) closeModal() {
	m.modal = tuiModalState{}
	m.modalIn.SetValue("")
	m.modalIn.Blur()
	m.focus = tuiFocusInput
	m.input.Focus()
}

func (m *tuiModel) refreshModalChoices() {
	query := strings.TrimSpace(m.modalIn.Value())
	switch m.modal.Kind {
	case tuiModalCommands:
		base := buildPaletteSuggestions(query, m.snapshot, m.history)
		m.modal.All = suggestionsToChoices(base)
	case tuiModalProviders:
		if len(m.modal.All) == 0 {
			return
		}
	case tuiModalModels:
		if len(m.modal.All) == 0 && !m.modal.Loading {
			m.modal.Visible = nil
			m.modal.Selection = 0
			return
		}
	default:
		return
	}
	if query == "" {
		m.modal.Visible = append([]tuiChoice(nil), m.modal.All...)
	} else {
		visible := make([]tuiChoice, 0, len(m.modal.All))
		for _, choice := range m.modal.All {
			haystack := strings.ToLower(strings.Join([]string{choice.Label, choice.Value, choice.Detail}, " "))
			if strings.Contains(haystack, strings.ToLower(query)) {
				visible = append(visible, choice)
			}
		}
		m.modal.Visible = visible
	}
	if len(m.modal.Visible) == 0 {
		m.modal.Selection = 0
		return
	}
	m.modal.Selection = clamp(m.modal.Selection, 0, len(m.modal.Visible)-1)
}

func (m *tuiModel) recordHistory(input string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return
	}
	if len(m.history) == 0 || m.history[len(m.history)-1] != value {
		m.history = append(m.history, value)
	}
}

func (m *tuiModel) refreshSuggestions() {
	if strings.TrimSpace(m.input.Value()) == "" {
		m.suggestions = nil
		m.sel = 0
		return
	}
	m.suggestions = buildInputSuggestions(m.input.Value(), m.snapshot, m.history)
	if len(m.suggestions) == 0 {
		m.sel = 0
		return
	}
	m.sel = clamp(m.sel, 0, len(m.suggestions)-1)
}

func (m *tuiModel) applySuggestion(suggestion tuiSuggestion) {
	m.input.SetValue(suggestion.Insert)
	m.refreshSuggestions()
	m.focus = tuiFocusInput
	m.setMainStatus(fmt.Sprintf("Inserted %s", suggestion.Label))
}

func (m *tuiModel) openPane(title, body string) {
	m.paneVisible = true
	m.focus = tuiFocusPane
	m.paneTitle = title
	if strings.TrimSpace(body) == "" {
		body = "No output."
	}
	m.paneBody = body
	m.pane.SetContent(m.renderPaneBody(max(20, m.width-8)))
	m.pane.GotoTop()
}

func (m *tuiModel) openTranscriptScreen(withSearch bool) tea.Cmd {
	m.transcriptScreen.Open(m.messageStore.Len())
	m.screen = tuiScreenTranscript
	m.input.Blur()
	if withSearch {
		m.focus = tuiFocusTranscriptSearch
		m.refreshTranscriptSearch()
		return m.searchIn.Focus()
	}
	m.searchIn.Blur()
	m.focus = tuiFocusTranscript
	return nil
}

func (m *tuiModel) closeTranscriptScreen() {
	m.screen = tuiScreenMain
	m.focus = tuiFocusInput
	m.transcriptScreen.Close()
	m.searchIn.SetValue("")
	m.searchIn.Blur()
	m.input.Focus()
}

func (m *tuiModel) openHistorySearch() tea.Cmd {
	m.historyDraft = m.input.Value()
	m.historyIn.SetValue("")
	m.historySelection = 0
	m.historyStatus = ""
	m.focus = tuiFocusHistorySearch
	m.suggestions = nil
	m.sel = 0
	m.input.Blur()
	m.refreshHistorySearch()
	return m.historyIn.Focus()
}

func (m *tuiModel) closeHistorySearch(restoreDraft bool) tea.Cmd {
	if restoreDraft {
		m.input.SetValue(m.historyDraft)
		m.input.CursorEnd()
	}
	m.historyIn.SetValue("")
	m.historyIn.Blur()
	m.historyMatches = nil
	m.historySelection = 0
	m.historyStatus = ""
	m.historyDraft = ""
	m.focus = tuiFocusInput
	m.refreshSuggestions()
	return m.input.Focus()
}

func (m *tuiModel) appendItem(item tuiTimelineItem) {
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = fmt.Sprintf("%s_%d_%d", item.Kind, item.CreatedAt.UnixNano(), len(m.items))
	}
	m.items = append(m.items, item)
	width := max(20, m.width-6)
	m.timeline.SetContent(m.renderTimelineContent(width))
	if m.autoScroll {
		m.timeline.GotoBottom()
		m.unread = 0
	}
}

func (m *tuiModel) appendTranscript(entry protocol.TranscriptEntry, persist bool) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(entry.SessionID) == "" {
		entry.SessionID = m.snapshot.Meta.SessionID
	}
	m.messageStore.Append(entry)
	if persist && m.runtime != nil && m.runtime.store != nil {
		_ = m.runtime.store.AppendTranscriptEntry(entry.SessionID, entry)
	}
	if entry.Type == protocol.TranscriptEntryUser || entry.Type == protocol.TranscriptEntryCommand {
		m.recordHistory(entry.Body)
	}
	visible, countUnread := m.appendTranscriptProjection(entry)
	m.transcript.SetContent(m.renderTranscriptContent(max(20, m.width-2)))
	if m.screen == tuiScreenTranscript && m.transcript.AtBottom() {
		m.transcript.GotoBottom()
	} else if visible && countUnread && !m.autoScroll {
		m.unread++
	}
	if m.focus == tuiFocusTranscriptSearch {
		m.refreshTranscriptSearch()
	}
}

func (m *tuiModel) appendEvent(event protocol.StreamEvent) {
	entry, ok := transcriptEntryFromEvent(event)
	if !ok {
		return
	}
	m.appendTranscript(entry, true)
}

func (m *tuiModel) appendTranscriptProjection(entry protocol.TranscriptEntry) (bool, bool) {
	message, ok := m.messagePipeline.Project(entry)
	if !ok {
		return false, false
	}
	visibility := message.Visibility
	presentation := message.Presentation
	switch {
	case presentation == protocol.TranscriptPresentationHidden:
		return false, false
	case visibility == protocol.TranscriptVisibilityAmbient || visibility == protocol.TranscriptVisibilityDebug:
		return false, false
	case message.Type == tuiui.UIMessageActivity || visibility == protocol.TranscriptVisibilityActivity || presentation == protocol.TranscriptPresentationGrouped:
		m.upsertActivityItem(entry)
		return true, false
	default:
		m.activityCount = 0
		m.activityStarted = time.Time{}
		m.activityStats = make(map[string]int)
		m.appendItem(timelineItemFromUIMessage(message))
		return true, true
	}
}

func (m *tuiModel) upsertActivityItem(entry protocol.TranscriptEntry) {
	if m.activityCount == 0 {
		m.activityStarted = entry.CreatedAt
		if m.activityStarted.IsZero() {
			m.activityStarted = time.Now()
		}
	}
	m.activityCount++
	if m.activityStats == nil {
		m.activityStats = make(map[string]int)
	}
	if key := activityKey(entry); key != "" && shouldCountActivity(entry) {
		m.activityStats[key]++
	}
	item := timelineItemFromTranscriptEntry(entry)
	item.ID = "activity.grouped"
	item.Kind = tuiItemProgress
	item.Subtype = "activity.grouped"
	item.Title = "Activity"
	item.Body = activitySummary(entry, m.activityStats, m.activityCount, m.activityStarted)
	item.CreatedAt = m.activityStarted
	if len(m.items) > 0 && m.items[len(m.items)-1].Kind == tuiItemProgress && m.items[len(m.items)-1].Subtype == "activity.grouped" {
		m.replaceLastItem(item)
		return
	}
	m.appendItem(item)
}

func (m *tuiModel) replaceLastItem(item tuiTimelineItem) {
	if len(m.items) == 0 {
		m.appendItem(item)
		return
	}
	m.items[len(m.items)-1] = item
	width := max(20, m.width-6)
	if content, ok := m.messageViewport.ReplaceLastByKey(width, item.ID, m.renderTimelineItem(item, width)); ok {
		m.timeline.SetContent(content)
	} else {
		m.timeline.SetContent(m.renderTimelineContent(width))
	}
	if m.autoScroll {
		m.timeline.GotoBottom()
		m.unread = 0
	}
}

func (m *tuiModel) appendError(err error) {
	if err == nil {
		return
	}
	m.appendTranscript(newTranscriptEntry(
		m.snapshot.Meta.SessionID,
		protocol.TranscriptEntryError,
		"Error",
		err.Error(),
	), true)
}

func (m *tuiModel) hydrateTimeline(events []protocol.StreamEvent) {
	m.hydrateTranscript(transcriptEntriesFromLegacyEvents(events))
}

func (m *tuiModel) hydrateTranscript(entries []protocol.TranscriptEntry) {
	m.messageStore.Reset(entries)
	m.items = m.items[:0]
	m.messageViewport.Reset()
	m.messagePipeline = tuiui.NewMessagePipeline()
	m.activityCount = 0
	m.activityStarted = time.Time{}
	m.history = nil
	for _, entry := range entries {
		if entry.Type == protocol.TranscriptEntryUser || entry.Type == protocol.TranscriptEntryCommand {
			m.recordHistory(entry.Body)
		}
		m.appendTranscriptProjection(entry)
	}
	if len(m.items) == 0 {
		m.ensureWelcomeItem()
	}
	m.timeline.SetContent(m.renderTimelineContent(max(20, m.width-6)))
	m.timeline.GotoBottom()
	m.transcript.SetContent(m.renderTranscriptContent(max(20, m.width-2)))
	m.transcript.GotoBottom()
}

func (m *tuiModel) ensureWelcomeItem() {
	m.appendItem(tuiTimelineItem{
		ID:        "welcome",
		Kind:      tuiItemSystem,
		Subtype:   "welcome",
		Title:     "Welcome",
		Body:      "Ask about this workspace, edit files, or attach papers when needed.",
		CreatedAt: time.Now(),
	})
}

func (m *tuiModel) consumeSubmittedInput() string {
	line := strings.TrimSpace(m.input.Value())
	m.input.SetValue("")
	m.historyState = tuiHistoryState{}
	m.promptController.SetValue("")
	m.promptController.CancelHistory()
	m.focus = tuiFocusInput
	m.suggestions = nil
	m.sel = 0
	return line
}

func (m *tuiModel) toggleHints() {
	m.setHintsVisible(!m.hintsVisible)
}

func (m *tuiModel) setHintsVisible(visible bool) {
	m.hintsVisible = visible
	if visible {
		m.setMainStatus("Hints shown")
		return
	}
	m.setMainStatus("Hints hidden")
}

func (m *tuiModel) clearTranscriptSearch() {
	m.transcriptScreen.ClearSearch()
	m.searchIn.SetValue("")
	m.searchIn.Blur()
}

func (m *tuiModel) openTranscriptSearch() {
	m.focus = tuiFocusTranscriptSearch
	m.searchIn.SetValue("")
	m.transcriptScreen.OpenSearch()
	m.searchIn.Focus()
}

func (m *tuiModel) closeTranscriptSearch(clear bool) {
	m.focus = tuiFocusTranscript
	m.searchIn.Blur()
	m.transcriptScreen.CloseSearch(clear)
	if clear {
		m.searchIn.SetValue("")
	}
}

func (m *tuiModel) moveTranscriptSearchSelection(delta int) {
	if m.transcriptScreen.MoveSearch(delta) {
		m.jumpToSearchSelection()
	}
}

func (m *tuiModel) setMainStatus(status string) {
	m.mainStatus = strings.TrimSpace(status)
}

func statusForSnapshot(snapshot protocol.SessionSnapshot) string {
	if snapshot.Meta.ApprovalPending || snapshot.Meta.State == protocol.SessionStateAwaitingApproval {
		return "Awaiting approval"
	}
	return ""
}

func (m *tuiModel) reflow() {
	if !m.ready {
		return
	}
	width := max(30, m.width-2)
	wasPinned := m.timeline.AtBottom()
	bottomGap := max(0, m.timeline.TotalLineCount()-(m.timeline.YOffset+m.timeline.Height))

	m.input.SetWidth(max(20, width-4))
	lines := clamp(m.input.LineCount(), 1, 7)
	m.input.SetHeight(lines)
	m.modalIn.Width = clamp(width-16, 18, 72)
	m.searchIn.Width = clamp(width-16, 18, 72)
	m.historyIn.Width = clamp(width-24, 18, 72)

	footerHeight := lipgloss.Height(m.renderFooter())
	inputHeight := lipgloss.Height(m.renderInput())
	approvalHeight := lipgloss.Height(m.renderApprovalStickyPanel())
	headerHeight := lipgloss.Height(m.renderHeader())
	if m.screen == tuiScreenMain {
		headerHeight += lipgloss.Height(m.renderStickyPromptHeader())
	}

	paneHeight := 0
	if m.paneVisible {
		paneHeight = clamp(m.height/4, 8, max(8, m.height/3))
	}
	timelineHeight := max(6, m.height-headerHeight-inputHeight-footerHeight-approvalHeight)

	m.timeline.Width = max(20, width-2)
	m.timeline.Height = timelineHeight
	anchor, hasAnchor := m.messageViewport.AnchorAt(m.timeline.YOffset)
	m.timeline.SetContent(m.renderTimelineContent(m.timeline.Width))
	if m.autoScroll || wasPinned {
		m.timeline.GotoBottom()
		m.unread = 0
	} else if hasAnchor {
		if offset, ok := m.messageViewport.OffsetForAnchor(anchor); ok {
			m.timeline.SetYOffset(offset)
		} else {
			m.timeline.SetYOffset(max(0, m.timeline.TotalLineCount()-m.timeline.Height-bottomGap))
		}
	} else {
		m.timeline.SetYOffset(max(0, m.timeline.TotalLineCount()-m.timeline.Height-bottomGap))
	}
	transcriptSearchHeight := 0
	if m.screen == tuiScreenTranscript && m.focus == tuiFocusTranscriptSearch {
		transcriptSearchHeight = lipgloss.Height(m.renderTranscriptSearchBar(width))
	}
	m.transcript.Width = max(20, width-2)
	m.transcript.Height = max(6, m.height-headerHeight-footerHeight-transcriptSearchHeight-3)
	m.transcript.SetContent(m.renderTranscriptContent(m.transcript.Width))

	if m.paneVisible {
		m.pane.Width = max(20, width-4)
		m.pane.Height = max(4, paneHeight-2)
		m.pane.SetContent(m.renderPaneBody(m.pane.Width))
	}
}

func (m *tuiModel) renderHeader() string {
	width := max(30, m.width-2)
	workspaceName := m.workspaceName
	if m.snapshot.Workspace != nil && strings.TrimSpace(m.snapshot.Workspace.Name) != "" {
		workspaceName = m.snapshot.Workspace.Name
	}
	state := string(m.snapshot.Meta.State)
	if state == "" {
		state = "idle"
	}
	profile := firstNonEmpty(m.snapshot.Meta.ProviderProfile, m.runtime.cfg.ActiveProviderName())
	model := firstNonEmpty(m.snapshot.Meta.Model, m.runtime.cfg.ActiveProviderConfig().Model)
	rightParts := []string{
		sessionLabel(m.snapshot.Meta),
		state,
	}
	if profile != "" || model != "" {
		rightParts = append(rightParts, strings.Trim(strings.Join([]string{profile, model}, "/"), "/"))
	}
	if m.busy {
		rightParts = append(rightParts, "running")
	}
	if m.screen == tuiScreenTranscript {
		rightParts = append(rightParts, "transcript")
	}
	right := strings.Join(rightParts, " · ")

	rightWidth := 0
	if width >= 48 {
		rightWidth = clamp(width/2, 18, 48)
	} else if width >= 36 {
		rightWidth = clamp(width/3, 10, 18)
	}
	leftWidth := width
	if rightWidth > 0 {
		leftWidth = max(12, width-rightWidth-1)
	}
	if width < 64 {
		workspaceName = ""
	}
	leftRaw := "papersilm"
	if workspaceName != "" {
		workspaceBudget := max(0, leftWidth-lipgloss.Width("papersilm · "))
		workspaceName = truncateRight(workspaceName, workspaceBudget)
		leftRaw = "papersilm · " + workspaceName
	}
	right = truncateRight(right, rightWidth)

	line := m.styles.headerAccent.Render("papersilm")
	if workspaceName != "" {
		line += m.styles.headerMuted.Render(" · " + workspaceName)
	}
	if right != "" {
		gap := max(1, width-lipgloss.Width(leftRaw)-lipgloss.Width(right))
		line += strings.Repeat(" ", gap) + m.styles.headerStatus.Render(right)
	}
	return line
}

func (m *tuiModel) renderStickyPromptHeader() string {
	if m.screen != tuiScreenMain || m.autoScroll {
		return ""
	}
	prompt := m.lastUserPrompt()
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	width := max(20, m.width-2)
	return m.styles.footerMuted.Width(width).Render(truncateRight("› "+prompt, width))
}

func (m *tuiModel) lastUserPrompt() string {
	if body := strings.TrimSpace(m.messageStore.LatestInput()); body != "" {
		return strings.ReplaceAll(body, "\n", " ")
	}
	for i := len(m.items) - 1; i >= 0; i-- {
		item := m.items[i]
		if item.Kind != tuiItemUser {
			continue
		}
		if body := strings.TrimSpace(item.Body); body != "" {
			return strings.ReplaceAll(body, "\n", " ")
		}
	}
	return ""
}

func (m *tuiModel) renderPane() string {
	if !m.paneVisible {
		return ""
	}
	width := max(20, m.width-2)
	divider := m.styles.paneDivider.Width(width).Render(strings.Repeat("─", max(1, width)))
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.paneTitle.Render(m.paneTitle),
		m.pane.View(),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		divider,
		m.styles.paneBody.Width(width).Render(body),
	)
}

func (m *tuiModel) renderInput() string {
	width := max(20, m.width-2)
	label := "prompt"
	if m.approvalPanelActive() {
		label = "prompt · approval pending"
	}
	body := m.input.View()
	if strings.TrimSpace(m.input.Value()) == "" {
		placeholder := truncateRight(tuiPromptPlaceholder, max(8, width-4))
		body = m.styles.footerMuted.Render("› " + placeholder)
	}
	return tuiui.RenderPromptChrome(tuiui.PromptChrome{
		Width:        width,
		Label:        label,
		Body:         body,
		LabelStyle:   m.styles.footerMuted,
		DividerStyle: m.styles.paneDivider,
		BodyStyle:    m.styles.inputShell,
	})
}

func (m *tuiModel) renderApprovalStickyPanel() string {
	if !m.approvalPanelActive() {
		return ""
	}
	options := m.approvalOptions()
	if len(options) == 0 {
		return ""
	}
	width := max(20, m.width-2)
	bodyWidth := max(12, width-4)
	m.approvalSelection = clamp(m.approvalSelection, 0, len(options)-1)

	summary := strings.TrimSpace(summarizePendingApproval(m.snapshot))
	if summary == "" {
		summary = "Plan is waiting for approval."
	}
	rows := make([]tuiui.ListRow, 0, len(options))
	for i, option := range options {
		selected := i == m.approvalSelection
		labelStyle := m.styles.suggestionLabel
		detailStyle := m.styles.suggestionDetail
		if selected {
			labelStyle = m.styles.suggestionActiveLabel
			detailStyle = m.styles.suggestionActiveDetail
		}
		if option.Disabled {
			labelStyle = m.styles.modalDisabled
			detailStyle = m.styles.modalDisabled
		}
		rows = append(rows, tuiui.ListRow{
			Label:       option.Label,
			Detail:      option.Detail,
			Selected:    selected,
			Disabled:    option.Disabled,
			MarkerStyle: m.styles.suggestionMarker,
			LabelStyle:  labelStyle,
			DetailStyle: detailStyle,
		})
	}

	hint := "Y/Enter approve · N/Esc keep planning · I inspect · ↑↓ move"
	if !m.approvalKeyboardActive() {
		hint = "Draft active · Enter sends prompt · clear prompt for approval shortcuts"
	}
	return tuiui.RenderDecisionPanel(tuiui.DecisionPanel{
		Width:        width,
		Title:        "Approval required",
		Summary:      truncateRight(summary, bodyWidth),
		Hint:         hint,
		Rows:         rows,
		DividerStyle: m.styles.paneDivider,
		TitleStyle:   m.styles.approvalLabel,
		MutedStyle:   m.styles.footerMuted,
	})
}

func (m *tuiModel) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	width := clamp(m.width-4, 24, 96)
	visible, start := windowSuggestions(m.suggestions, m.sel, 5)
	rows := make([]tuiui.ListRow, 0, len(visible))
	for i, suggestion := range visible {
		selected := start+i == m.sel
		labelStyle := m.styles.suggestionLabel
		detailStyle := m.styles.suggestionDetail
		if selected {
			labelStyle = m.styles.suggestionActiveLabel
			detailStyle = m.styles.suggestionActiveDetail
		}
		rows = append(rows, tuiui.ListRow{
			Label:       suggestion.Label,
			Detail:      suggestion.Detail,
			Selected:    selected,
			MarkerStyle: m.styles.suggestionMarker,
			LabelStyle:  labelStyle,
			DetailStyle: detailStyle,
		})
	}
	return tuiui.RenderPromptOverlay(tuiui.PromptOverlay{
		Kind:      tuiui.OverlaySuggestions,
		Rows:      rows,
		Selection: m.sel,
	}, width)
}

func (m *tuiModel) renderFooter() string {
	width := max(20, m.width-2)
	profile := m.snapshot.Meta.ProviderProfile
	if profile == "" {
		profile = m.runtime.cfg.ActiveProviderName()
	}
	model := m.snapshot.Meta.Model
	if model == "" {
		model = m.runtime.cfg.ActiveProviderConfig().Model
	}
	taskCount := 0
	approvals := 0
	if m.snapshot.TaskBoard != nil {
		taskCount = len(m.snapshot.TaskBoard.Tasks)
		for _, task := range m.snapshot.TaskBoard.Tasks {
			if task.Status == protocol.TaskStatusAwaitingApproval {
				approvals++
			}
		}
	}
	if m.snapshot.Meta.ApprovalPending {
		if approvals == 0 {
			approvals = 1
		}
	}
	leftParts := []string{string(m.snapshot.Meta.PermissionMode)}
	if m.screen == tuiScreenMain {
		if m.busy {
			leftParts = append(leftParts, "running")
		}
		if strings.TrimSpace(m.mainStatus) != "" {
			leftParts = append(leftParts, strings.TrimSpace(m.mainStatus))
		}
	} else if m.screen == tuiScreenTranscript {
		leftParts = append(leftParts, "transcript")
	}
	if taskCount > 0 {
		leftParts = append(leftParts, fmt.Sprintf("%d tasks", taskCount))
	}
	if approvals > 0 {
		leftParts = append(leftParts, fmt.Sprintf("%d approvals", approvals))
	}
	if len(m.snapshot.Sources) > 0 && width >= 90 {
		leftParts = append(leftParts, fmt.Sprintf("%d sources", len(m.snapshot.Sources)))
	}
	left := strings.Join(leftParts, " · ")
	rightParts := []string{}
	if (profile != "" || model != "") && width >= 44 {
		rightParts = append(rightParts, strings.Trim(strings.Join([]string{profile, model}, "/"), "/"))
	}
	if workspace := compactWorkspaceName(m.workspaceDisplayPath, m.workspaceName); workspace != "" && width >= 72 {
		rightParts = append(rightParts, workspace)
	}
	if width >= 96 {
		rightParts = append(rightParts, string(m.styles.theme))
	}
	right := strings.Join(rightParts, " · ")
	shortcuts := "Enter send · Tab complete · Ctrl+K commands · Ctrl+O transcript · Ctrl+R history · Ctrl+/ hints · Esc close"
	if m.screen == tuiScreenTranscript {
		shortcuts = "/ search · n/N next · q/Esc back · Ctrl+/ hints"
	}
	searchLine := ""
	if m.screen == tuiScreenMain && m.focus == tuiFocusHistorySearch {
		searchLine = m.renderHistorySearchFooterLine(width)
	}
	showHints := m.hintsVisible && m.height >= 18 && strings.TrimSpace(m.input.Value()) == "" && m.focus != tuiFocusHistorySearch
	return tuiui.RenderFooterChrome(tuiui.FooterChrome{
		Width:       width,
		MetaLeft:    left,
		MetaRight:   right,
		SearchLine:  searchLine,
		Hints:       shortcuts,
		ShowHints:   showHints,
		FooterStyle: m.styles.footer,
		LeftStyle:   m.styles.footerMuted,
		RightStyle:  m.styles.footerMuted,
		HintStyle:   m.styles.footerMuted,
	})
}

func (m *tuiModel) renderHistorySearchFooterLine(width int) string {
	label := "search prompts:"
	if len(m.historyMatches) == 0 && strings.TrimSpace(m.historyIn.Value()) != "" {
		label = "no matching prompt:"
	}
	status := strings.TrimSpace(m.historyStatus)
	left := m.styles.footerMuted.Render(label + " ")
	inputWidth := max(8, width-lipgloss.Width(label)-lipgloss.Width(status)-6)
	line := left + m.styles.body.Width(inputWidth).Render(m.historyIn.View())
	if status != "" {
		line += m.styles.footerMuted.Render(" · ") + m.styles.footerAccent.Render(status)
	}
	if lipgloss.Width(line) > width {
		return truncateRight(line, width)
	}
	return line
}

func (m *tuiModel) renderModalOver(base string) string {
	box := m.renderModalBox()
	if strings.TrimSpace(box) == "" {
		return base
	}
	return tuiui.OverlayBottomWithPeek(base, box, m.width, 2)
}

func (m *tuiModel) renderModal() string {
	box := m.renderModalBox()
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Bottom, box)
}

func (m *tuiModel) renderModalBox() string {
	width := max(40, m.width)
	bodyWidth := max(20, width-4)
	filter := m.modalIn.View()
	if m.modal.Kind == tuiModalModels && m.modal.Provider != "" {
		filter = m.styles.modalHint.Render("profile "+m.modal.Provider) + "\n  " + filter
	}
	message := strings.TrimSpace(m.modal.Message)
	if m.modal.Loading {
		if message != "" {
			message += " · "
		}
		message += "Loading models..."
	}
	if len(m.modal.Visible) == 0 && !m.modal.Loading && strings.TrimSpace(m.modalIn.Value()) != "" {
		if message != "" {
			message += " · "
		}
		message += "Press Enter to use the typed value."
	}
	choiceLimit := clamp(m.height/3, 5, 10)
	visible, start := windowChoices(m.modal.Visible, m.modal.Selection, choiceLimit)
	rows := make([]tuiui.ListRow, 0, len(visible))
	for i, choice := range visible {
		selected := start+i == m.modal.Selection
		labelStyle := m.styles.suggestionLabel
		detailStyle := m.styles.suggestionDetail
		if choice.Disabled {
			labelStyle = m.styles.modalDisabled
			detailStyle = m.styles.modalHint
		}
		if selected {
			if !choice.Disabled {
				labelStyle = m.styles.suggestionActiveLabel
				detailStyle = m.styles.suggestionActiveDetail
			}
		}
		rows = append(rows, tuiui.ListRow{
			Label:       choice.Label,
			Detail:      choice.Detail,
			Selected:    selected,
			Disabled:    choice.Disabled,
			MarkerStyle: m.styles.suggestionMarker,
			LabelStyle:  labelStyle,
			DetailStyle: detailStyle,
		})
	}
	return tuiui.RenderDrawerOverlay(tuiui.DrawerOverlay{
		Kind:    modalOverlayKind(m.modal.Kind),
		Title:   m.modal.Title,
		Message: truncateRight(message, bodyWidth),
		Filter:  filter,
		Rows:    rows,
	}, tuiui.Drawer{
		Width:        width,
		DividerStyle: m.styles.approvalLabel,
		TitleStyle:   m.styles.modalTitle,
		MutedStyle:   m.styles.modalMessage,
		BodyStyle:    m.styles.body,
	})
}

func modalOverlayKind(kind tuiModalKind) tuiui.OverlayKind {
	switch kind {
	case tuiModalCommands:
		return tuiui.OverlayPalette
	case tuiModalProviders, tuiModalModels:
		return tuiui.OverlayModelPicker
	default:
		return tuiui.OverlayNone
	}
}

func (m *tuiModel) renderTranscriptScreen() string {
	width := max(20, m.width-2)
	lines := []string{m.renderHeader()}
	lines = append(lines, m.transcript.View())
	if m.focus == tuiFocusTranscriptSearch {
		lines = append(lines, m.renderTranscriptSearchBar(width))
	}
	lines = append(lines, m.renderFooter())
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *tuiModel) renderTranscriptSearchBar(width int) string {
	header := m.styles.footerMuted.Render("/ search transcript")
	if current, count := m.transcriptScreen.MatchPosition(); count > 0 {
		header = header + m.styles.footerMuted.Render(" · ") + m.styles.footerAccent.Render(
			fmt.Sprintf("%d/%d", current, count),
		)
	} else if status := strings.TrimSpace(m.transcriptScreen.Status()); status != "" {
		header = header + m.styles.footerMuted.Render(" · ") + m.styles.footerAccent.Render(status)
	}
	search := lipgloss.JoinVertical(lipgloss.Left, header, m.searchIn.View())
	return m.styles.inputShell.Width(width).Render(search)
}

func (m *tuiModel) renderTimelineContent(width int) string {
	keys := make([]string, 0, len(m.items))
	for i, item := range m.items {
		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = fmt.Sprintf("%s_%d_%d", item.Kind, item.CreatedAt.UnixNano(), i)
		}
		keys = append(keys, key)
	}
	return m.messageViewport.ContentByKey(width, keys, func(index int, width int) string {
		return m.renderTimelineItem(m.items[index], width)
	})
}

func (m *tuiModel) renderTranscriptContent(width int) string {
	entries := m.transcriptEntries()
	if len(entries) == 0 {
		return m.styles.footerMuted.Width(width).Render("No transcript entries yet.")
	}
	rendered := make([]string, 0, len(entries))
	for idx, entry := range entries {
		selected := m.transcriptScreen.IsSelected(idx)
		rendered = append(rendered, m.renderTranscriptEntry(entry, width, selected))
	}
	return strings.Join(rendered, "\n\n")
}

func (m *tuiModel) transcriptEntries() []protocol.TranscriptEntry {
	return m.transcriptScreen.Entries(m.messageStore.Entries())
}

func (m *tuiModel) renderPaneBody(width int) string {
	if width <= 0 {
		width = 20
	}
	if strings.TrimSpace(m.paneBody) == "" {
		return ""
	}
	if paneLooksLikeMarkdown(m.paneBody) {
		return m.renderMarkdown(m.paneBody, width)
	}
	return m.styles.body.Width(width).Render(m.paneBody)
}

func (m *tuiModel) renderTimelineItem(item tuiTimelineItem, width int) string {
	return tuiui.RenderTimelineItem(m.timelineRenderItem(item, max(10, width-4)), width, tuiui.TimelineRenderer{
		Styles: tuiui.TimelineStyles{
			Body:           m.styles.body,
			UserShell:      m.styles.userShell,
			UserLabel:      m.styles.userLabel,
			AssistantLabel: m.styles.assistantLabel,
			ApprovalShell:  m.styles.approvalShell,
			ApprovalLabel:  m.styles.approvalLabel,
			SuccessShell:   m.styles.successShell,
			SuccessLabel:   m.styles.successLabel,
			RejectionShell: m.styles.rejectionShell,
			RejectionLabel: m.styles.rejectionLabel,
			ErrorShell:     m.styles.errorShell,
			ErrorLabel:     m.styles.errorLabel,
			FooterMuted:    m.styles.footerMuted,
			ProgressLine:   m.styles.progressLine,
			SystemLine:     m.styles.systemLine,
		},
		Markdown: m.renderMarkdown,
	})
}

func (m *tuiModel) timelineRenderItem(item tuiTimelineItem, bodyWidth int) tuiui.TimelineItem {
	return tuiui.TimelineItem{
		ID:              item.ID,
		Kind:            tuiui.TimelineKind(item.Kind),
		Subtype:         item.Subtype,
		Title:           item.Title,
		Body:            item.Body,
		Markdown:        item.Markdown,
		Compact:         item.Compact,
		CreatedAt:       item.CreatedAt,
		Workspace:       compactWorkspaceName(m.workspaceDisplayPath, m.workspaceName),
		DecisionOptions: m.renderApprovalOptions(item, bodyWidth),
	}
}

func (m *tuiModel) renderApprovalOptions(item tuiTimelineItem, width int) string {
	if m.approvalPanelActive() {
		return ""
	}
	if !m.shouldRenderApprovalOptions(item) {
		return ""
	}
	options := m.approvalOptions()
	if len(options) == 0 {
		return ""
	}
	m.approvalSelection = clamp(m.approvalSelection, 0, len(options)-1)
	rows := make([]tuiui.ListRow, 0, len(options))
	for i, option := range options {
		selected := i == m.approvalSelection
		labelStyle := m.styles.suggestionLabel
		detailStyle := m.styles.suggestionDetail
		if selected {
			labelStyle = m.styles.suggestionActiveLabel
			detailStyle = m.styles.suggestionActiveDetail
		}
		if option.Disabled {
			labelStyle = m.styles.modalDisabled
			detailStyle = m.styles.modalDisabled
		}
		rows = append(rows, tuiui.ListRow{
			Label:       option.Label,
			Detail:      option.Detail,
			Selected:    selected,
			Disabled:    option.Disabled,
			MarkerStyle: m.styles.suggestionMarker,
			LabelStyle:  labelStyle,
			DetailStyle: detailStyle,
		})
	}
	lines := tuiui.RenderListRows(rows, width)
	hint := m.styles.footerMuted.Render("  Enter select · Tab/↑↓ move · A approve · R keep planning · I inspect")
	lines = append(lines, hint)
	return strings.Join(lines, "\n")
}

func (m *tuiModel) shouldRenderApprovalOptions(item tuiTimelineItem) bool {
	if !m.approvalPanelActive() || item.Subtype != transcriptSubtypeApprovalRequired {
		return false
	}
	for i := len(m.items) - 1; i >= 0; i-- {
		candidate := m.items[i]
		if candidate.Kind != tuiItemApproval || candidate.Subtype != transcriptSubtypeApprovalRequired {
			continue
		}
		return candidate.CreatedAt.Equal(item.CreatedAt) && candidate.Body == item.Body && candidate.Title == item.Title
	}
	return true
}

func (m *tuiModel) renderTranscriptEntry(entry protocol.TranscriptEntry, width int, selected bool) string {
	block := m.renderTimelineItem(timelineItemFromTranscriptEntry(entry), width)
	if !selected {
		return block
	}
	return m.styles.footerAccent.Render("› ") + block
}

func (m *tuiModel) refreshTranscriptSearch() {
	m.transcriptScreen.RefreshSearch(m.transcriptEntries(), m.searchIn.Value())
	if m.transcriptScreen.MatchCount() > 0 {
		m.jumpToSearchSelection()
	}
}

func (m *tuiModel) refreshHistorySearch() {
	mode := transcriptInputModeForValue(m.historyDraft)
	if strings.TrimSpace(m.historyDraft) == "" {
		mode = protocol.TranscriptInputPrompt
	}
	query := strings.TrimSpace(strings.ToLower(m.historyIn.Value()))
	candidates := m.messageStore.History(mode)
	if len(candidates) == 0 && len(m.history) > 0 {
		for i := len(m.history) - 1; i >= 0; i-- {
			value := strings.TrimSpace(m.history[i])
			if value == "" || transcriptInputModeForValue(value) != mode {
				continue
			}
			candidates = append(candidates, protocol.TranscriptEntry{
				ID:        fmt.Sprintf("history_%d", i),
				Type:      protocol.TranscriptEntryUser,
				Title:     "History",
				Body:      value,
				InputMode: mode,
			})
		}
	}
	m.historyMatches = m.historyMatches[:0]
	for _, entry := range candidates {
		if query == "" || strings.Contains(strings.ToLower(entry.Body), query) || strings.Contains(strings.ToLower(entry.Title), query) {
			m.historyMatches = append(m.historyMatches, entry)
		}
	}
	if len(m.historyMatches) == 0 {
		m.historySelection = 0
		if query == "" {
			m.historyStatus = "No prompt history"
		} else {
			m.historyStatus = "No matching prompt"
		}
		return
	}
	m.historySelection = clamp(m.historySelection, 0, len(m.historyMatches)-1)
	m.historyStatus = fmt.Sprintf("%d/%d", m.historySelection+1, len(m.historyMatches))
}

func (m *tuiModel) moveHistorySearchSelection(delta int) {
	if len(m.historyMatches) == 0 {
		return
	}
	next := m.historySelection + delta
	if next < 0 {
		next = len(m.historyMatches) - 1
	}
	if next >= len(m.historyMatches) {
		next = 0
	}
	m.historySelection = next
	m.historyStatus = fmt.Sprintf("%d/%d", m.historySelection+1, len(m.historyMatches))
}

func (m *tuiModel) acceptHistorySearch(execute bool) tea.Cmd {
	if len(m.historyMatches) == 0 {
		return m.closeHistorySearch(false)
	}
	value := m.historyMatches[m.historySelection].Body
	cmd := m.closeHistorySearch(false)
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.refreshSuggestions()
	if !execute {
		return cmd
	}
	_, submitCmd := m.submitInput()
	if cmd == nil {
		return submitCmd
	}
	return tea.Batch(cmd, submitCmd)
}

func (m *tuiModel) jumpToSearchSelection() {
	index, ok := m.transcriptScreen.SelectedEntryIndex()
	if !ok {
		return
	}
	offset := transcriptOffsetForIndex(m.transcriptEntries(), index, m.transcript.Width)
	target := max(0, offset-max(0, m.transcript.Height/3))
	m.transcript.SetYOffset(target)
}

func (m *tuiModel) updateScrollState() {
	m.autoScroll = m.timeline.AtBottom()
	if m.autoScroll {
		m.unread = 0
	}
}

func (m *tuiModel) jumpMainToBottom() {
	m.autoScroll = true
	m.unread = 0
	m.timeline.GotoBottom()
}

func (m *tuiModel) shouldUseHistoryUp() bool {
	if m.paneVisible || m.screen != tuiScreenMain {
		return false
	}
	if strings.TrimSpace(m.input.Value()) == "" {
		return true
	}
	return m.input.Line() == 0 && m.input.LineInfo().RowOffset == 0
}

func (m *tuiModel) shouldUseHistoryDown() bool {
	if m.screen != tuiScreenMain {
		return false
	}
	if m.historyState.active {
		return true
	}
	return m.input.Line() >= max(0, m.input.LineCount()-1)
}

func (m *tuiModel) historyUp() {
	mode := transcriptInputModeForValue(m.input.Value())
	entries := m.messageStore.History(mode)
	if len(entries) == 0 {
		return
	}
	if !m.historyState.active || m.historyState.mode != mode {
		m.promptController.SetValue(m.input.Value())
		m.promptController.SetHistory(promptHistoryEntries(entries))
		m.historyState = tuiHistoryState{
			active: true,
			draft:  m.input.Value(),
			mode:   mode,
		}
	}
	if !m.promptController.HistoryPrev() {
		return
	}
	if m.historyState.index < len(entries) {
		m.historyState.index++
	}
	m.input.SetValue(m.promptController.Value())
	m.input.CursorEnd()
}

func (m *tuiModel) historyDown() {
	if !m.historyState.active {
		return
	}
	mode := m.historyState.mode
	entries := m.messageStore.History(mode)
	if len(entries) == 0 {
		m.historyState = tuiHistoryState{}
		m.promptController.CancelHistory()
		return
	}
	if !m.promptController.HistoryNext() {
		return
	}
	if m.historyState.index > 1 {
		m.historyState.index--
		m.input.SetValue(m.promptController.Value())
		m.input.CursorEnd()
		return
	}
	m.input.SetValue(m.promptController.Value())
	m.input.CursorEnd()
	m.historyState = tuiHistoryState{}
}

func promptHistoryEntries(entries []protocol.TranscriptEntry) []tuiui.PromptHistoryEntry {
	out := make([]tuiui.PromptHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, tuiui.PromptHistoryEntry{
			Value: entry.Body,
			Mode:  tuiui.PromptModeFromTranscript(entry.InputMode),
		})
	}
	return out
}

func timelineItemFromTranscriptEntry(entry protocol.TranscriptEntry) tuiTimelineItem {
	item := tuiTimelineItem{
		ID:        entry.ID,
		Subtype:   entry.Subtype,
		Title:     firstNonEmpty(entry.Title, string(entry.Type)),
		Body:      entry.Body,
		Markdown:  entry.Markdown,
		CreatedAt: entry.CreatedAt,
	}
	switch entry.Type {
	case protocol.TranscriptEntryUser, protocol.TranscriptEntryCommand:
		item.Kind = tuiItemUser
	case protocol.TranscriptEntryAssistant:
		item.Kind = tuiItemAssistant
	case protocol.TranscriptEntryApproval:
		item.Kind = tuiItemApproval
		item.Compact = entry.Subtype == transcriptSubtypeApprovalRejected && !strings.Contains(strings.TrimSpace(entry.Body), "\n")
	case protocol.TranscriptEntryError:
		item.Kind = tuiItemError
	case protocol.TranscriptEntryProgress:
		item.Kind = tuiItemProgress
	default:
		item.Kind = tuiItemSystem
	}
	if entry.Type == protocol.TranscriptEntryDivider {
		item.Kind = tuiItemSystem
		item.Body = strings.Repeat("─", 48)
	}
	return item
}

func timelineItemFromUIMessage(message tuiui.UIMessage) tuiTimelineItem {
	item := tuiTimelineItem{
		ID:        message.ID,
		Subtype:   message.Subtype,
		Title:     firstNonEmpty(message.Title, string(message.Type)),
		Body:      message.Body,
		Markdown:  message.Markdown,
		CreatedAt: message.CreatedAt,
	}
	switch message.Type {
	case tuiui.UIMessageUser:
		item.Kind = tuiItemUser
	case tuiui.UIMessageAssistant:
		item.Kind = tuiItemAssistant
	case tuiui.UIMessageDecision:
		item.Kind = tuiItemApproval
		item.Compact = message.Subtype == transcriptSubtypeApprovalRejected && !strings.Contains(strings.TrimSpace(message.Body), "\n")
	case tuiui.UIMessageError:
		item.Kind = tuiItemError
	case tuiui.UIMessageActivity:
		item.Kind = tuiItemProgress
	default:
		item.Kind = tuiItemSystem
	}
	return item
}

func transcriptDisplayForEntry(entry protocol.TranscriptEntry) (protocol.TranscriptVisibility, protocol.TranscriptPresentation) {
	visibility := entry.Visibility
	presentation := entry.Presentation
	if visibility == "" {
		switch entry.Type {
		case protocol.TranscriptEntryUser, protocol.TranscriptEntryCommand, protocol.TranscriptEntryAssistant, protocol.TranscriptEntryDivider:
			visibility = protocol.TranscriptVisibilityPrimary
		case protocol.TranscriptEntryApproval:
			visibility = protocol.TranscriptVisibilityDecision
		case protocol.TranscriptEntryError:
			visibility = protocol.TranscriptVisibilityDecision
		case protocol.TranscriptEntryProgress:
			visibility = protocol.TranscriptVisibilityActivity
		default:
			visibility = protocol.TranscriptVisibilityAmbient
		}
	}
	if presentation == "" {
		switch visibility {
		case protocol.TranscriptVisibilityActivity:
			presentation = protocol.TranscriptPresentationGrouped
		case protocol.TranscriptVisibilityAmbient, protocol.TranscriptVisibilityDebug:
			presentation = protocol.TranscriptPresentationHidden
		default:
			presentation = protocol.TranscriptPresentationBlock
		}
	}
	return visibility, presentation
}

func activitySummary(entry protocol.TranscriptEntry, stats map[string]int, count int, started time.Time) string {
	verb := activityVerb(entry, stats)
	last := strings.TrimSpace(entry.Body)
	if last == "" {
		last = strings.TrimSpace(entry.Title)
	}
	parts := []string{verb}
	statText := activityStatsText(stats)
	if statText != "" {
		parts = append(parts, statText)
	} else if count > 1 {
		parts = append(parts, fmt.Sprintf("%d updates", count))
	}
	if !started.IsZero() {
		if elapsed := time.Since(started).Round(time.Second); elapsed > 0 {
			parts = append(parts, elapsed.String())
		}
	}
	if last != "" && statText == "" {
		parts = append(parts, truncateRight(last, 72))
	}
	return strings.Join(parts, " · ")
}

func activityVerb(entry protocol.TranscriptEntry, stats map[string]int) string {
	if stats["write"] > 0 {
		return "Editing workspace"
	}
	if stats["command"] > 0 {
		return "Running command"
	}
	if stats["download"] > 0 || stats["paper"] > 0 {
		return "Preparing paper context"
	}
	if stats["search"] > 0 || stats["read"] > 0 {
		return "Inspecting workspace"
	}
	text := strings.ToLower(strings.Join([]string{entry.Subtype, entry.Title, entry.Body}, " "))
	switch {
	case strings.Contains(text, "workspace"):
		return "Inspecting workspace"
	case strings.Contains(text, "source") || strings.Contains(text, "paper"):
		return "Preparing paper context"
	case strings.Contains(text, "artifact") || strings.Contains(text, "write"):
		return "Writing artifact"
	case strings.Contains(text, "skill"):
		return "Running skill"
	case strings.Contains(text, "plan"):
		return "Planning"
	case strings.Contains(text, "node"):
		return "Running plan"
	default:
		return "Working"
	}
}

func activityKey(entry protocol.TranscriptEntry) string {
	text := strings.ToLower(strings.Join([]string{entry.Subtype, entry.Title, entry.Body}, " "))
	switch {
	case strings.Contains(text, "workspace_search") || strings.Contains(text, "search"):
		return "search"
	case strings.Contains(text, "workspace_inspect") || strings.Contains(text, "read") || strings.Contains(text, "open"):
		return "read"
	case strings.Contains(text, "workspace_edit") || strings.Contains(text, "write") || strings.Contains(text, "artifact"):
		return "write"
	case strings.Contains(text, "workspace_command") || strings.Contains(text, "shell") || strings.Contains(text, "command"):
		return "command"
	case strings.Contains(text, "download"):
		return "download"
	case strings.Contains(text, "paper") || strings.Contains(text, "source"):
		return "paper"
	default:
		return ""
	}
}

func shouldCountActivity(entry protocol.TranscriptEntry) bool {
	text := strings.ToLower(entry.Body)
	if strings.Contains(text, "completed") || strings.Contains(text, "failed") {
		return false
	}
	return true
}

func activityStatsText(stats map[string]int) string {
	if len(stats) == 0 {
		return ""
	}
	ordered := []struct {
		key   string
		label string
	}{
		{key: "read", label: "read"},
		{key: "search", label: "search"},
		{key: "write", label: "write"},
		{key: "command", label: "command"},
		{key: "download", label: "download"},
		{key: "paper", label: "paper"},
	}
	parts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		n := stats[item.key]
		if n <= 0 {
			continue
		}
		label := item.label
		if n > 1 {
			label += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, label))
	}
	return strings.Join(parts, " · ")
}

func executionToTranscriptEntries(input string, before, after protocol.SessionSnapshot, fallback string) []protocol.TranscriptEntry {
	entries := make([]protocol.TranscriptEntry, 0, 2)
	if decision, ok := approvalDecisionTranscriptEntry(input, before, after); ok {
		entries = append(entries, decision)
	}
	if decision, ok := pendingApprovalTranscriptEntry(after); ok {
		entries = append(entries, decision)
		return entries
	}

	title := resultTitle(input)
	if markdown := newestComparisonMarkdown(before, after); markdown != "" {
		entries = append(entries, newTranscriptEntry(after.Meta.SessionID, protocol.TranscriptEntryAssistant, title, markdown, withTranscriptMarkdown(true)))
		return entries
	}
	if markdown := newestDigestMarkdown(before, after); markdown != "" {
		entries = append(entries, newTranscriptEntry(after.Meta.SessionID, protocol.TranscriptEntryAssistant, title, markdown, withTranscriptMarkdown(true)))
		return entries
	}
	if markdown := newestSkillArtifactMarkdown(before, after); markdown != "" {
		entries = append(entries, newTranscriptEntry(after.Meta.SessionID, protocol.TranscriptEntryAssistant, title, markdown, withTranscriptMarkdown(true)))
		return entries
	}

	if len(entries) > 0 {
		return entries
	}

	entryType := protocol.TranscriptEntryAssistant
	if strings.TrimSpace(fallback) == "" {
		entryType = protocol.TranscriptEntrySystem
	}
	if strings.TrimSpace(input) == "/clear" {
		entryType = protocol.TranscriptEntryDivider
	}
	entries = append(entries, newTranscriptEntry(after.Meta.SessionID, entryType, title, fallback, withTranscriptMarkdown(looksLikeMarkdown(fallback))))
	return entries
}

func approvalDecisionTranscriptEntry(input string, before, after protocol.SessionSnapshot) (protocol.TranscriptEntry, bool) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return protocol.TranscriptEntry{}, false
	}
	switch fields[0] {
	case "/approve":
		title, body := summarizeSessionApproval(before, after)
		if title == "" {
			return protocol.TranscriptEntry{}, false
		}
		return newTranscriptEntry(
			after.Meta.SessionID,
			protocol.TranscriptEntryApproval,
			title,
			body,
			withTranscriptSubtype(transcriptSubtypeApprovalApproved),
		), true
	case "/reject":
		title, body, compact := summarizeSessionRejection(before, after)
		if title == "" {
			return protocol.TranscriptEntry{}, false
		}
		entry := newTranscriptEntry(
			after.Meta.SessionID,
			protocol.TranscriptEntryApproval,
			title,
			body,
			withTranscriptSubtype(transcriptSubtypeApprovalRejected),
		)
		if compact {
			entry.RenderState = protocol.TranscriptRenderCollapsed
		}
		return entry, true
	case "/task":
		if len(fields) < 3 {
			return protocol.TranscriptEntry{}, false
		}
		switch fields[1] {
		case "approve":
			title, body := summarizeTaskApproval(fields[2], before, after)
			if title == "" {
				return protocol.TranscriptEntry{}, false
			}
			return newTranscriptEntry(
				after.Meta.SessionID,
				protocol.TranscriptEntryApproval,
				title,
				body,
				withTranscriptSubtype(transcriptSubtypeApprovalApproved),
			), true
		case "reject":
			title, body, compact := summarizeTaskRejection(fields[2], before, after)
			if title == "" {
				return protocol.TranscriptEntry{}, false
			}
			entry := newTranscriptEntry(
				after.Meta.SessionID,
				protocol.TranscriptEntryApproval,
				title,
				body,
				withTranscriptSubtype(transcriptSubtypeApprovalRejected),
			)
			if compact {
				entry.RenderState = protocol.TranscriptRenderCollapsed
			}
			return entry, true
		}
	}
	return protocol.TranscriptEntry{}, false
}

func pendingApprovalTranscriptEntry(after protocol.SessionSnapshot) (protocol.TranscriptEntry, bool) {
	if !after.Meta.ApprovalPending && after.Meta.State != protocol.SessionStateAwaitingApproval {
		return protocol.TranscriptEntry{}, false
	}
	body := summarizePendingApproval(after)
	if strings.TrimSpace(body) == "" {
		body = "Plan is waiting for approval."
	}
	return newTranscriptEntry(
		after.Meta.SessionID,
		protocol.TranscriptEntryApproval,
		"Approval Required",
		body,
		withTranscriptSubtype(transcriptSubtypeApprovalRequired),
	), true
}

func summarizePendingApproval(after protocol.SessionSnapshot) string {
	planID := firstNonEmpty(after.Meta.ActivePlanID, taskBoardPlanID(after.TaskBoard), planResultID(after.Plan))
	tasks := awaitingApprovalTasks(after.TaskBoard)
	parts := make([]string, 0, 4)
	if planID != "" {
		parts = append(parts, "Plan "+planID)
	}
	if len(tasks) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending %s", len(tasks), pluralWord(len(tasks), "task")))
		parts = append(parts, strings.Join(tasks, ", "))
	}
	if after.Meta.ActiveCheckpointID != "" {
		parts = append(parts, "checkpoint "+after.Meta.ActiveCheckpointID)
	}
	return strings.Join(parts, " · ")
}

func taskBoardPlanID(board *protocol.TaskBoard) string {
	if board == nil {
		return ""
	}
	return strings.TrimSpace(board.PlanID)
}

func planResultID(plan *protocol.PlanResult) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.PlanID)
}

func awaitingApprovalTasks(board *protocol.TaskBoard) []string {
	if board == nil {
		return nil
	}
	tasks := make([]string, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		if task.Status != protocol.TaskStatusAwaitingApproval {
			continue
		}
		label := firstNonEmpty(task.Title, task.NodeID, task.TaskID)
		if label == "" {
			continue
		}
		tasks = append(tasks, truncateRight(label, 36))
		if len(tasks) >= 3 {
			break
		}
	}
	return tasks
}

func pluralWord(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

func summarizeSessionApproval(before, after protocol.SessionSnapshot) (string, string) {
	pendingBefore := pendingApprovalCount(before)
	pendingAfter := pendingApprovalCount(after)
	bodyParts := []string{"Approved the current checkpoint."}
	if pendingBefore > pendingAfter {
		bodyParts = append(bodyParts, fmt.Sprintf("%d approval item(s) were released.", pendingBefore-pendingAfter))
	}
	switch after.Meta.State {
	case protocol.SessionStateCompleted:
		bodyParts = append(bodyParts, "Execution completed.")
	case protocol.SessionStateAwaitingApproval:
		bodyParts = append(bodyParts, fmt.Sprintf("%d task(s) still need approval.", pendingAfter))
	default:
		bodyParts = append(bodyParts, "Execution resumed.")
	}
	return "Approved", strings.Join(bodyParts, " ")
}

func summarizeSessionRejection(before, after protocol.SessionSnapshot) (string, string, bool) {
	pendingBefore := pendingApprovalCount(before)
	pendingAfter := pendingApprovalCount(after)
	bodyParts := []string{"Kept the plan open without running the checkpoint."}
	if pendingBefore > 0 && pendingAfter < pendingBefore {
		bodyParts = append(bodyParts, fmt.Sprintf("%d approval item(s) were cleared.", pendingBefore-pendingAfter))
	}
	if after.Meta.State == protocol.SessionStatePlanned {
		bodyParts = append(bodyParts, "You can keep editing the request or inspect tasks.")
	}
	return "Rejected", strings.Join(bodyParts, " "), true
}

func summarizeTaskApproval(taskID string, before, after protocol.SessionSnapshot) (string, string) {
	beforeTask, _ := findTaskByID(before.TaskBoard, taskID)
	afterTask, ok := findTaskByID(after.TaskBoard, taskID)
	name := taskDisplayName(afterTask, beforeTask, taskID)
	if !ok && beforeTask.TaskID == "" {
		return "", ""
	}
	bodyParts := []string{fmt.Sprintf("Approved %s.", name)}
	if status := friendlyTaskStatus(afterTask.Status); status != "" {
		bodyParts = append(bodyParts, fmt.Sprintf("Status is now %s.", status))
	}
	if after.Meta.ApprovalPending {
		bodyParts = append(bodyParts, fmt.Sprintf("%d task(s) still need approval.", pendingApprovalCount(after)))
	} else if after.Meta.State == protocol.SessionStateCompleted {
		bodyParts = append(bodyParts, "The active run is complete.")
	} else {
		bodyParts = append(bodyParts, "Execution resumed.")
	}
	return "Approved", strings.Join(bodyParts, " ")
}

func summarizeTaskRejection(taskID string, before, after protocol.SessionSnapshot) (string, string, bool) {
	beforeTask, _ := findTaskByID(before.TaskBoard, taskID)
	afterTask, ok := findTaskByID(after.TaskBoard, taskID)
	name := taskDisplayName(afterTask, beforeTask, taskID)
	if !ok && beforeTask.TaskID == "" {
		return "", "", false
	}
	summary := fmt.Sprintf("%s was rejected.", name)
	if status := friendlyTaskStatus(afterTask.Status); status != "" {
		summary = fmt.Sprintf("%s Status is now %s.", summary, status)
	}
	reason := strings.TrimSpace(afterTask.Error)
	if reason == "" {
		return "Rejected", summary, true
	}
	body := summary + "\n\nReason: " + reason
	return "Rejected", body, false
}

func pendingApprovalCount(snapshot protocol.SessionSnapshot) int {
	total := 0
	if snapshot.TaskBoard != nil {
		for _, task := range snapshot.TaskBoard.Tasks {
			if task.Status == protocol.TaskStatusAwaitingApproval {
				total++
			}
		}
	}
	if snapshot.Meta.ApprovalPending {
		if total == 0 {
			total = 1
		}
	}
	return total
}

func taskDisplayName(afterTask, beforeTask protocol.TaskCard, fallback string) string {
	for _, task := range []protocol.TaskCard{afterTask, beforeTask} {
		if strings.TrimSpace(task.Title) != "" {
			return task.Title
		}
		if strings.TrimSpace(task.TaskID) != "" {
			return task.TaskID
		}
	}
	return fallback
}

func friendlyTaskStatus(status protocol.TaskStatus) string {
	text := strings.TrimSpace(string(status))
	if text == "" {
		return ""
	}
	return strings.ReplaceAll(text, "_", " ")
}

func transcriptOffsetForIndex(entries []protocol.TranscriptEntry, index, width int) int {
	if index <= 0 || len(entries) == 0 {
		return 0
	}
	offset := 0
	for i := 0; i < len(entries) && i < index; i++ {
		block := timelineItemFromTranscriptEntry(entries[i])
		rendered := renderTimelinePreview(block, max(20, width))
		offset += lipgloss.Height(rendered) + 2
	}
	return offset
}

func renderTimelinePreview(item tuiTimelineItem, width int) string {
	bodyWidth := max(10, width-4)
	timestamp := item.CreatedAt.Local().Format("15:04")
	switch item.Kind {
	case tuiItemUser:
		label := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.Title)), "you")
		return label + " · " + timestamp + "\n" + truncateRight(item.Body, bodyWidth)
	case tuiItemAssistant:
		return strings.ToLower(item.Title) + " · " + timestamp + "\n" + truncateRight(item.Body, bodyWidth)
	case tuiItemApproval, tuiItemError:
		return item.Title + " · " + timestamp + "\n" + truncateRight(item.Body, bodyWidth)
	default:
		return timestamp + "  " + truncateRight(item.Body, width)
	}
}

func (m *tuiModel) renderMarkdown(markdown string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.styles.markdownStyle),
		glamour.WithWordWrap(max(20, width)),
	)
	if err != nil {
		return markdown
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return strings.TrimSpace(rendered)
}

func waitForTUIEvent(ch <-chan protocol.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		return tuiEventMsg{Event: <-ch}
	}
}

func runPromptCmd(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot, prompt string) tea.Cmd {
	return func() tea.Msg {
		before := snapshot
		after := snapshot
		text, err := executePromptText(ctx, runtime.svc, &after, prompt)
		return tuiExecDoneMsg{
			Input:  prompt,
			Before: before,
			After:  after,
			Text:   text,
			Err:    err,
		}
	}
}

func runSlashCmd(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot, line, paneTitle string, pane bool) tea.Cmd {
	return runSlashCmdWithHistory(ctx, runtime, snapshot, line, paneTitle, pane, false)
}

func runSlashCmdWithHistory(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot, line, paneTitle string, pane, skipHistory bool) tea.Cmd {
	return func() tea.Msg {
		before := snapshot
		after := snapshot
		result, err := executeSlashCommandText(ctx, runtime.svc, runtime.store, &after, line)
		if err != nil {
			return tuiExecDoneMsg{Input: line, SkipHistory: skipHistory, Before: before, After: after, Err: err}
		}
		if paneTitle == "" {
			paneTitle = result.PaneTitle
		}
		return tuiExecDoneMsg{
			Input:       line,
			SkipHistory: skipHistory,
			Before:      before,
			After:       after,
			Text:        result.Text,
			Pane:        pane || result.Pane,
			PaneTitle:   paneTitle,
		}
	}
}

func runApprovalRejectCmd(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot) tea.Cmd {
	return func() tea.Msg {
		before := snapshot
		result, err := runtime.svc.Approve(ctx, snapshot.Meta.SessionID, false, "")
		after := snapshot
		if err == nil {
			after = result.Session
		}
		return tuiExecDoneMsg{
			Input:       "/reject",
			SkipHistory: true,
			Before:      before,
			After:       after,
			Text:        "Kept the plan open without running the checkpoint.",
			Err:         err,
		}
	}
}

func discoverModelsCmd(runtime *tuiRuntimeManager, profile string) tea.Cmd {
	return func() tea.Msg {
		models, err := runtime.discoverModels(profile)
		return tuiDiscoverModelsMsg{Profile: profile, Models: models, Err: err}
	}
}

func switchProviderCmd(runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot, profile, model string) tea.Cmd {
	return func() tea.Msg {
		after := snapshot
		err := runtime.switchProviderModel(&after, profile, model)
		return tuiSwitchProviderMsg{
			Profile: profile,
			Model:   model,
			After:   after,
			Err:     err,
		}
	}
}

func suggestionsToChoices(suggestions []tuiSuggestion) []tuiChoice {
	choices := make([]tuiChoice, 0, len(suggestions))
	for _, suggestion := range suggestions {
		choices = append(choices, tuiChoice{
			Label:    suggestion.Label,
			Value:    suggestion.Insert,
			Detail:   suggestion.Detail,
			Disabled: suggestion.Disabled,
		})
	}
	return choices
}

func eventToTimelineItem(event protocol.StreamEvent) tuiTimelineItem {
	title := string(event.Type)
	body := strings.TrimSpace(event.Message)
	if body == "" {
		body = title
	}

	switch event.Type {
	case protocol.EventApprovalRequired:
		if summary := approvalSummary(event.Payload); summary != "" {
			body = summary
		}
		return tuiTimelineItem{Kind: tuiItemApproval, Subtype: transcriptSubtypeApprovalRequired, Title: "Approval Required", Body: body, CreatedAt: event.CreatedAt}
	case protocol.EventError:
		return tuiTimelineItem{Kind: tuiItemError, Title: "Error", Body: body, CreatedAt: event.CreatedAt}
	case protocol.EventProgress:
		if progress := progressSummary(event.Payload); progress != "" {
			body = progress
		}
		return tuiTimelineItem{Kind: tuiItemProgress, Title: "Progress", Body: body, CreatedAt: event.CreatedAt}
	case protocol.EventAssistant:
		return tuiTimelineItem{Kind: tuiItemAssistant, Title: "Assistant", Body: body, Markdown: looksLikeMarkdown(body), CreatedAt: event.CreatedAt}
	default:
		if detail := payloadSummary(event.Payload); detail != "" && detail != body {
			body = body + " · " + detail
		}
		return tuiTimelineItem{Kind: tuiItemSystem, Title: title, Body: body, CreatedAt: event.CreatedAt}
	}
}

func executionToTimelineItem(input string, before, after protocol.SessionSnapshot, fallback string) tuiTimelineItem {
	title := resultTitle(input)
	if markdown := newestComparisonMarkdown(before, after); markdown != "" {
		return tuiTimelineItem{
			Kind:      tuiItemAssistant,
			Title:     title,
			Body:      markdown,
			Markdown:  true,
			CreatedAt: time.Now(),
		}
	}
	if markdown := newestDigestMarkdown(before, after); markdown != "" {
		return tuiTimelineItem{
			Kind:      tuiItemAssistant,
			Title:     title,
			Body:      markdown,
			Markdown:  true,
			CreatedAt: time.Now(),
		}
	}
	if markdown := newestSkillArtifactMarkdown(before, after); markdown != "" {
		return tuiTimelineItem{
			Kind:      tuiItemAssistant,
			Title:     title,
			Body:      markdown,
			Markdown:  true,
			CreatedAt: time.Now(),
		}
	}
	return tuiTimelineItem{
		Kind:      tuiItemAssistant,
		Title:     title,
		Body:      fallback,
		Markdown:  looksLikeMarkdown(fallback),
		CreatedAt: time.Now(),
	}
}

func newestComparisonMarkdown(before, after protocol.SessionSnapshot) string {
	if after.Compare == nil || strings.TrimSpace(after.Compare.Markdown) == "" {
		return ""
	}
	if before.Compare == nil {
		return after.Compare.Markdown
	}
	if after.Compare.ArtifactID != before.Compare.ArtifactID || after.Compare.GeneratedAt.After(before.Compare.GeneratedAt) {
		return after.Compare.Markdown
	}
	return ""
}

func newestDigestMarkdown(before, after protocol.SessionSnapshot) string {
	beforeMap := make(map[string]protocol.PaperDigest, len(before.Digests))
	for _, digest := range before.Digests {
		key := digest.PaperID + "|" + digest.ArtifactID
		beforeMap[key] = digest
	}
	parts := make([]string, 0)
	for _, digest := range after.Digests {
		if strings.TrimSpace(digest.Markdown) == "" {
			continue
		}
		key := digest.PaperID + "|" + digest.ArtifactID
		prev, ok := beforeMap[key]
		if !ok || digest.GeneratedAt.After(prev.GeneratedAt) {
			parts = append(parts, digest.Markdown)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n---\n\n"))
}

func newestSkillArtifactMarkdown(before, after protocol.SessionSnapshot) string {
	beforeIDs := make(map[string]struct{}, len(before.SkillArtifacts))
	for _, artifact := range before.SkillArtifacts {
		beforeIDs[artifact.ArtifactID] = struct{}{}
	}
	for i := len(after.SkillArtifacts) - 1; i >= 0; i-- {
		artifact := after.SkillArtifacts[i]
		if _, ok := beforeIDs[artifact.ArtifactID]; ok {
			continue
		}
		if markdown := readArtifactMarkdown(&artifact); strings.TrimSpace(markdown) != "" {
			return markdown
		}
	}
	return ""
}

func approvalSummary(payload interface{}) string {
	switch v := payload.(type) {
	case protocol.ApprovalRequest:
		return fmt.Sprintf("Plan %s · pending nodes %s · %s", v.PlanID, strings.Join(v.PendingNodeIDs, ", "), v.Summary)
	case map[string]interface{}:
		planID := anyString(v["plan_id"])
		summary := anyString(v["summary"])
		nodes := anyStrings(v["pending_node_ids"])
		if summary != "" {
			return fmt.Sprintf("Plan %s · pending nodes %s · %s", planID, strings.Join(nodes, ", "), summary)
		}
	}
	return ""
}

func progressSummary(payload interface{}) string {
	switch v := payload.(type) {
	case protocol.PlanProgress:
		parts := []string{string(v.Status)}
		if v.Tool != "" {
			parts = append(parts, "tool="+v.Tool)
		}
		if v.NodeID != "" {
			parts = append(parts, "node="+v.NodeID)
		}
		if v.StepID != "" {
			parts = append(parts, "step="+v.StepID)
		}
		if v.Message != "" {
			parts = append(parts, v.Message)
		}
		if v.Error != "" {
			parts = append(parts, "error="+v.Error)
		}
		return strings.Join(parts, " · ")
	case map[string]interface{}:
		parts := []string{anyString(v["status"])}
		if tool := anyString(v["tool"]); tool != "" {
			parts = append(parts, "tool="+tool)
		}
		if nodeID := anyString(v["node_id"]); nodeID != "" {
			parts = append(parts, "node="+nodeID)
		}
		if stepID := anyString(v["step_id"]); stepID != "" {
			parts = append(parts, "step="+stepID)
		}
		if message := anyString(v["message"]); message != "" {
			parts = append(parts, message)
		}
		if failure := anyString(v["error"]); failure != "" {
			parts = append(parts, "error="+failure)
		}
		return strings.Trim(strings.Join(parts, " · "), " ·")
	default:
		return ""
	}
}

func payloadSummary(payload interface{}) string {
	switch v := payload.(type) {
	case protocol.ArtifactManifest:
		return fmt.Sprintf("artifact=%s kind=%s", v.ArtifactID, v.Kind)
	case map[string]interface{}:
		artifactID := anyString(v["artifact_id"])
		kind := anyString(v["kind"])
		if artifactID != "" || kind != "" {
			return strings.TrimSpace(fmt.Sprintf("artifact=%s kind=%s", artifactID, kind))
		}
	}
	return ""
}

func resultTitle(input string) string {
	if !strings.HasPrefix(strings.TrimSpace(input), "/") {
		return "Assistant"
	}
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "Result"
	}
	switch fields[0] {
	case "/plan":
		return "Plan"
	case "/run":
		return "Run Result"
	case "/approve":
		return "Approval Result"
	case "/reject":
		return "Approval Result"
	case "/task":
		if len(fields) > 1 {
			return "Task " + strings.Title(fields[1])
		}
	case "/skill":
		if len(fields) > 1 {
			return "Skill " + strings.Title(fields[1])
		}
	case "/source":
		return "Sources"
	}
	return "Command Result"
}

func isHintsToggleKey(value string) bool {
	switch strings.TrimSpace(value) {
	case "ctrl+/", "ctrl+_":
		return true
	default:
		return false
	}
}

func looksLikeMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	markers := []string{"# ", "## ", "### ", "```", "\n- ", "\n1. ", "| ---", "\n> "}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func paneLooksLikeMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	markers := []string{"# ", "## ", "### ", "```", "| ---", "\n> "}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func sessionLabel(meta protocol.SessionMeta) string {
	if strings.TrimSpace(meta.Name) != "" {
		return meta.Name
	}
	if len(meta.SessionID) <= 14 {
		return meta.SessionID
	}
	return meta.SessionID[:14]
}

func themeCommandValue(line string) (config.ThemeSetting, bool) {
	theme, ok, err := parseThemeCommand(line)
	return theme, ok && err == nil
}

func parseThemeCommand(line string) (config.ThemeSetting, bool, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 || fields[0] != "/theme" {
		return "", false, nil
	}
	if len(fields) != 2 {
		return "", true, fmt.Errorf("usage: /theme <auto|dark|light>")
	}
	theme := config.ThemeSetting(fields[1])
	if !theme.Valid() {
		return "", true, fmt.Errorf("usage: /theme <auto|dark|light>")
	}
	return theme, true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func limitChoices(choices []tuiChoice, limit int) []tuiChoice {
	if limit <= 0 || len(choices) <= limit {
		return choices
	}
	return choices[:limit]
}

func windowSuggestions(suggestions []tuiSuggestion, selected, limit int) ([]tuiSuggestion, int) {
	start, end := windowRange(len(suggestions), selected, limit)
	if start == end {
		return nil, 0
	}
	return suggestions[start:end], start
}

func windowChoices(choices []tuiChoice, selected, limit int) ([]tuiChoice, int) {
	start, end := windowRange(len(choices), selected, limit)
	if start == end {
		return nil, 0
	}
	return choices[start:end], start
}

func windowRange(total, selected, limit int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if limit <= 0 || total <= limit {
		return 0, total
	}
	selected = clamp(selected, 0, total-1)
	start := max(0, min(selected-limit/2, total-limit))
	end := min(total, start+limit)
	return start, end
}

func anyString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func anyStrings(value interface{}) []string {
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func truncateRight(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	runes := []rune(value)
	if len(runes) <= width-1 {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func compactWorkspaceName(pathValue, fallback string) string {
	if name := strings.TrimSpace(fallback); name != "" {
		return name
	}
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return ""
	}
	name := filepath.Base(pathValue)
	if name == "." || name == string(filepath.Separator) {
		return pathValue
	}
	return name
}

func padRight(value string, width int) string {
	if width <= 0 {
		return value
	}
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
