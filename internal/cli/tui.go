package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

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
	Kind      tuiItemKind
	Title     string
	Body      string
	Markdown  bool
	CreatedAt time.Time
}

type tuiChoice struct {
	Label    string
	Value    string
	Detail   string
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
	Input     string
	Pane      bool
	PaneTitle string
	Before    protocol.SessionSnapshot
	After     protocol.SessionSnapshot
	Text      string
	Err       error
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
	background       lipgloss.Style
	header           lipgloss.Style
	headerMuted      lipgloss.Style
	headerAccent     lipgloss.Style
	userCard         lipgloss.Style
	assistantCard    lipgloss.Style
	approvalCard     lipgloss.Style
	errorCard        lipgloss.Style
	pane             lipgloss.Style
	input            lipgloss.Style
	footer           lipgloss.Style
	systemLine       lipgloss.Style
	progressLine     lipgloss.Style
	suggestionBox    lipgloss.Style
	suggestionNormal lipgloss.Style
	suggestionActive lipgloss.Style
	modal            lipgloss.Style
	modalTitle       lipgloss.Style
	modalMessage     lipgloss.Style
}

type tuiModel struct {
	ctx     context.Context
	runtime *tuiRuntimeManager

	snapshot protocol.SessionSnapshot

	timeline viewport.Model
	pane     viewport.Model
	input    textarea.Model
	modalIn  textinput.Model

	items       []tuiTimelineItem
	history     []string
	suggestions []tuiSuggestion
	sel         int

	paneVisible bool
	paneTitle   string
	paneBody    string

	modal tuiModalState

	width      int
	height     int
	ready      bool
	busy       bool
	status     string
	autoScroll bool

	styles tuiStyles
}

func RunTUI(ctx context.Context, opts TUIOptions) error {
	manager, snapshot, err := newTUIRuntimeManager(ctx, opts)
	if err != nil {
		return err
	}
	model := newTUIModel(ctx, manager, snapshot)
	events, err := manager.loadRecentEvents(snapshot.Meta.SessionID, 200)
	if err != nil {
		return err
	}
	model.hydrateTimeline(events)

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := program.Run(); err != nil && err != tea.ErrProgramKilled {
		return err
	}
	return nil
}

func newTUIModel(ctx context.Context, runtime *tuiRuntimeManager, snapshot protocol.SessionSnapshot) *tuiModel {
	input := textarea.New()
	input.Prompt = "▌ "
	input.Placeholder = "Ask about the current papers, or type /commands"
	input.ShowLineNumbers = false
	input.SetHeight(3)
	input.CharLimit = 0
	input.KeyMap.InsertNewline.SetKeys("ctrl+j")

	modalIn := textinput.New()
	modalIn.Placeholder = "Filter"

	model := &tuiModel{
		ctx:        ctx,
		runtime:    runtime,
		snapshot:   snapshot,
		timeline:   viewport.New(0, 0),
		pane:       viewport.New(0, 0),
		input:      input,
		modalIn:    modalIn,
		styles:     newTUIStyles(),
		autoScroll: true,
	}
	model.timeline.MouseWheelEnabled = true
	model.pane.MouseWheelEnabled = true
	model.setStatus("Interactive session ready")
	model.refreshSuggestions()
	if len(model.items) == 0 {
		model.appendItem(tuiTimelineItem{
			Kind:      tuiItemSystem,
			Title:     "Session",
			Body:      "Type /help for commands, Ctrl+K for the palette, or /model to switch runtime.",
			CreatedAt: time.Now(),
		})
	}
	return model
}

func newTUIStyles() tuiStyles {
	return tuiStyles{
		background:       lipgloss.NewStyle().Background(lipgloss.Color("#0B1220")).Foreground(lipgloss.Color("#E2E8F0")),
		header:           lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#12344D")).Foreground(lipgloss.Color("#F8FAFC")).Padding(0, 1),
		headerMuted:      lipgloss.NewStyle().Foreground(lipgloss.Color("#BFDBFE")),
		headerAccent:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true),
		userCard:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#38BDF8")).Padding(0, 1),
		assistantCard:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#14B8A6")).Padding(0, 1),
		approvalCard:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#F59E0B")).Padding(0, 1),
		errorCard:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#EF4444")).Padding(0, 1),
		pane:             lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#64748B")).Padding(0, 1),
		input:            lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#334155")).Padding(0, 1),
		footer:           lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Padding(0, 1),
		systemLine:       lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")),
		progressLine:     lipgloss.NewStyle().Foreground(lipgloss.Color("#7DD3FC")),
		suggestionBox:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#334155")).Padding(0, 1),
		suggestionNormal: lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")),
		suggestionActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#FBBF24")).Bold(true),
		modal:            lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#F59E0B")).Padding(1, 2),
		modalTitle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")),
		modalMessage:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")),
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), waitForTUIEvent(m.runtime.sink.ch))
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
		m.appendItem(eventToTimelineItem(msg.Event))
		m.reflow()
		return m, waitForTUIEvent(m.runtime.sink.ch)
	case tuiExecDoneMsg:
		m.busy = false
		if msg.Err != nil {
			m.appendError(msg.Err)
			m.setStatus(msg.Err.Error())
			m.reflow()
			return m, nil
		}
		m.snapshot = msg.After
		m.recordHistory(msg.Input)
		if msg.Pane {
			m.openPane(msg.PaneTitle, msg.Text)
			m.setStatus(fmt.Sprintf("%s opened", msg.PaneTitle))
		} else {
			item := executionToTimelineItem(msg.Input, msg.Before, msg.After, msg.Text)
			if strings.TrimSpace(item.Body) != "" {
				m.appendItem(item)
			}
			m.paneVisible = false
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
			m.setStatus(msg.Err.Error())
			m.reflow()
			return m, nil
		}
		m.snapshot = msg.After
		m.appendItem(tuiTimelineItem{
			Kind:      tuiItemSystem,
			Title:     "Runtime",
			Body:      fmt.Sprintf("Switched to %s / %s", msg.Profile, msg.Model),
			CreatedAt: time.Now(),
		})
		m.closeModal()
		m.setStatus("Provider and model updated")
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

	main := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.timeline.View(),
		m.renderPane(),
		m.renderInput(),
		m.renderSuggestions(),
		m.renderFooter(),
	)
	if m.modal.Kind == tuiModalNone {
		return m.styles.background.Width(m.width).Height(m.height).Render(main)
	}
	return m.renderModal()
}

func (m *tuiModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal.Kind != tuiModalNone {
		return m, nil
	}
	if m.paneVisible {
		var cmd tea.Cmd
		m.pane, cmd = m.pane.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.timeline, cmd = m.timeline.Update(msg)
	m.autoScroll = false
	return m, cmd
}

func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.modal.Kind != tuiModalNone:
		return m.handleModalKey(msg)
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.String() == "ctrl+k":
		m.openCommandPalette()
		m.reflow()
		return m, m.modalIn.Focus()
	case msg.String() == "esc":
		if m.paneVisible {
			m.paneVisible = false
			m.setStatus("Pane closed")
		} else if len(m.suggestions) > 0 {
			m.suggestions = nil
			m.sel = 0
		} else {
			m.setStatus("")
		}
		m.reflow()
		return m, nil
	case msg.String() == "pgup" || msg.String() == "pgdown":
		if m.paneVisible {
			var cmd tea.Cmd
			m.pane, cmd = m.pane.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.timeline, cmd = m.timeline.Update(msg)
		m.autoScroll = false
		return m, cmd
	case msg.String() == "up" && len(m.suggestions) > 0:
		m.sel = clamp(m.sel-1, 0, len(m.suggestions)-1)
		m.reflow()
		return m, nil
	case msg.String() == "down" && len(m.suggestions) > 0:
		m.sel = clamp(m.sel+1, 0, len(m.suggestions)-1)
		m.reflow()
		return m, nil
	case msg.String() == "tab" && len(m.suggestions) > 0:
		m.applySuggestion(m.suggestions[m.sel])
		m.reflow()
		return m, nil
	case msg.String() == "enter":
		return m.submitInput()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refreshSuggestions()
	m.reflow()
	return m, cmd
}

func (m *tuiModel) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closeModal()
		m.reflow()
		return m, nil
	case "up":
		if len(m.modal.Visible) > 0 {
			m.modal.Selection = clamp(m.modal.Selection-1, 0, len(m.modal.Visible)-1)
		}
		m.reflow()
		return m, nil
	case "down":
		if len(m.modal.Visible) > 0 {
			m.modal.Selection = clamp(m.modal.Selection+1, 0, len(m.modal.Visible)-1)
		}
		m.reflow()
		return m, nil
	case "enter":
		return m.commitModalSelection()
	}

	var cmd tea.Cmd
	m.modalIn, cmd = m.modalIn.Update(msg)
	m.refreshModalChoices()
	m.reflow()
	return m, cmd
}

func (m *tuiModel) submitInput() (tea.Model, tea.Cmd) {
	if m.busy {
		m.setStatus("A run is already in progress")
		m.reflow()
		return m, nil
	}
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return m, nil
	}
	switch line {
	case "/exit", "/quit":
		return m, tea.Quit
	case "/commands":
		m.openCommandPalette()
		m.reflow()
		return m, m.modalIn.Focus()
	case "/model":
		cmd := m.openProviderModal()
		m.reflow()
		return m, cmd
	}

	m.input.SetValue("")
	m.refreshSuggestions()

	if strings.HasPrefix(line, "/") {
		title, pane := classifyPaneCommand(line)
		if !pane {
			m.appendItem(tuiTimelineItem{
				Kind:      tuiItemUser,
				Title:     "Command",
				Body:      line,
				CreatedAt: time.Now(),
			})
		}
		m.busy = true
		m.setStatus("Running command...")
		m.reflow()
		return m, runSlashCmd(m.ctx, m.runtime, m.snapshot, line, title, pane)
	}

	m.appendItem(tuiTimelineItem{
		Kind:      tuiItemUser,
		Title:     "You",
		Body:      line,
		CreatedAt: time.Now(),
	})
	m.busy = true
	m.setStatus("Running task...")
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
		m.setStatus("Switching runtime...")
		m.reflow()
		return m, switchProviderCmd(m.runtime, m.snapshot, m.modal.Provider, modelName)
	default:
		return m, nil
	}
}

func (m *tuiModel) openCommandPalette() {
	m.input.Blur()
	m.modal = tuiModalState{
		Kind:    tuiModalCommands,
		Title:   "Command Palette",
		Message: "Filter commands, recipes, and current session context.",
	}
	m.modalIn.SetValue("")
	m.modalIn.Placeholder = "Type to filter commands or recipes"
	m.refreshModalChoices()
}

func (m *tuiModel) openProviderModal() tea.Cmd {
	m.input.Blur()
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
	m.setStatus(fmt.Sprintf("Inserted %s", suggestion.Label))
}

func (m *tuiModel) openPane(title, body string) {
	m.paneVisible = true
	m.paneTitle = title
	if strings.TrimSpace(body) == "" {
		body = "No output."
	}
	m.paneBody = body
	m.pane.SetContent(m.renderPaneBody(max(20, m.width-8)))
	m.pane.GotoTop()
}

func (m *tuiModel) appendItem(item tuiTimelineItem) {
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	m.items = append(m.items, item)
	m.timeline.SetContent(m.renderTimelineContent(max(20, m.width-6)))
	if m.autoScroll {
		m.timeline.GotoBottom()
	}
}

func (m *tuiModel) appendError(err error) {
	if err == nil {
		return
	}
	m.appendItem(tuiTimelineItem{
		Kind:      tuiItemError,
		Title:     "Error",
		Body:      err.Error(),
		CreatedAt: time.Now(),
	})
}

func (m *tuiModel) hydrateTimeline(events []protocol.StreamEvent) {
	for _, event := range events {
		m.items = append(m.items, eventToTimelineItem(event))
	}
	if len(m.items) == 0 {
		return
	}
	m.timeline.SetContent(m.renderTimelineContent(max(20, m.width-6)))
	m.timeline.GotoBottom()
}

func (m *tuiModel) setStatus(status string) {
	m.status = strings.TrimSpace(status)
}

func (m *tuiModel) reflow() {
	if !m.ready {
		return
	}
	width := max(30, m.width-2)

	m.input.SetWidth(max(20, width-6))
	lines := clamp(m.input.LineCount()+1, 3, 7)
	m.input.SetHeight(lines)

	suggestionsHeight := lipgloss.Height(m.renderSuggestions())
	footerHeight := lipgloss.Height(m.renderFooter())
	inputHeight := lipgloss.Height(m.renderInput())
	headerHeight := lipgloss.Height(m.renderHeader())

	paneHeight := 0
	if m.paneVisible {
		paneHeight = clamp(m.height/4, 8, max(8, m.height/3))
	}
	timelineHeight := max(6, m.height-headerHeight-inputHeight-suggestionsHeight-footerHeight-paneHeight)

	m.timeline.Width = max(20, width-2)
	m.timeline.Height = timelineHeight
	m.timeline.SetContent(m.renderTimelineContent(m.timeline.Width))
	if m.autoScroll {
		m.timeline.GotoBottom()
	}

	if m.paneVisible {
		m.pane.Width = max(20, width-6)
		m.pane.Height = max(4, paneHeight-2)
		m.pane.SetContent(m.renderPaneBody(m.pane.Width))
	}
}

func (m *tuiModel) renderHeader() string {
	width := max(30, m.width-2)
	profile := m.snapshot.Meta.ProviderProfile
	if profile == "" {
		profile = m.runtime.cfg.ActiveProviderName()
	}
	model := m.snapshot.Meta.Model
	if model == "" {
		model = m.runtime.cfg.ActiveProviderConfig().Model
	}
	state := string(m.snapshot.Meta.State)
	if state == "" {
		state = "idle"
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
		approvals++
	}

	left := fmt.Sprintf("papersilm  %s  %s/%s", sessionLabel(m.snapshot.Meta), m.snapshot.Meta.Language, m.snapshot.Meta.Style)
	right := fmt.Sprintf("%s · %s · %s/%s · sources=%d · tasks=%d · approvals=%d",
		state,
		m.snapshot.Meta.PermissionMode,
		profile,
		model,
		len(m.snapshot.Sources),
		taskCount,
		approvals,
	)
	if m.busy {
		right += " · busy"
	}
	line := truncateRight(left+"  "+right, max(10, width-2))
	return m.styles.header.Width(width).Render(line)
}

func (m *tuiModel) renderPane() string {
	if !m.paneVisible {
		return ""
	}
	width := max(20, m.width-2)
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render(m.paneTitle),
		m.pane.View(),
	)
	return m.styles.pane.Width(width).Render(body)
}

func (m *tuiModel) renderInput() string {
	width := max(20, m.width-2)
	return m.styles.input.Width(width).Render(m.input.View())
}

func (m *tuiModel) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	width := max(20, m.width-2)
	lines := make([]string, 0, len(m.suggestions))
	for i, suggestion := range limitSuggestions(m.suggestions, 6) {
		line := fmt.Sprintf("%s  %s", suggestion.Label, suggestion.Detail)
		if i == m.sel {
			lines = append(lines, m.styles.suggestionActive.Width(width-4).Render(truncateRight(line, width-6)))
			continue
		}
		lines = append(lines, m.styles.suggestionNormal.Render(truncateRight(line, width-6)))
	}
	return m.styles.suggestionBox.Width(width).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) renderFooter() string {
	width := max(20, m.width-2)
	status := "Enter submit • Ctrl+J newline • Tab apply • Ctrl+K palette • /model picker • Esc close • Ctrl+C quit"
	if m.status != "" {
		status = status + "   " + m.status
	}
	return m.styles.footer.Width(width).Render(truncateRight(status, width-2))
}

func (m *tuiModel) renderModal() string {
	width := clamp(m.width-8, 40, 96)
	bodyWidth := max(20, width-8)
	header := m.styles.modalTitle.Render(m.modal.Title)
	if m.modal.Kind == tuiModalModels && m.modal.Provider != "" {
		header = header + "\n" + m.styles.systemLine.Render("Profile: "+m.modal.Provider)
	}
	lines := []string{header, m.modalIn.View()}
	if m.modal.Message != "" {
		lines = append(lines, m.styles.modalMessage.Render(m.modal.Message))
	}
	if m.modal.Loading {
		lines = append(lines, m.styles.progressLine.Render("Loading models..."))
	}
	if len(m.modal.Visible) == 0 && !m.modal.Loading && strings.TrimSpace(m.modalIn.Value()) != "" {
		lines = append(lines, m.styles.systemLine.Render("Press Enter to use the typed value."))
	}
	for i, choice := range limitChoices(m.modal.Visible, 10) {
		line := fmt.Sprintf("%s  %s", choice.Label, choice.Detail)
		style := m.styles.suggestionNormal
		if choice.Disabled {
			style = style.Foreground(lipgloss.Color("#FCA5A5"))
		}
		if i == m.modal.Selection {
			style = m.styles.suggestionActive
		}
		lines = append(lines, style.Width(bodyWidth).Render(truncateRight(line, bodyWidth)))
	}
	box := m.styles.modal.Width(width).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *tuiModel) renderTimelineContent(width int) string {
	if len(m.items) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(m.items))
	for _, item := range m.items {
		rendered = append(rendered, m.renderTimelineItem(item, width))
	}
	return strings.Join(rendered, "\n\n")
}

func (m *tuiModel) renderPaneBody(width int) string {
	if width <= 0 {
		width = 20
	}
	if strings.TrimSpace(m.paneBody) == "" {
		return ""
	}
	if looksLikeMarkdown(m.paneBody) {
		return m.renderMarkdown(m.paneBody, width)
	}
	return lipgloss.NewStyle().Width(width).Render(m.paneBody)
}

func (m *tuiModel) renderTimelineItem(item tuiTimelineItem, width int) string {
	bodyWidth := max(10, width-6)
	timestamp := item.CreatedAt.Local().Format("15:04:05")
	switch item.Kind {
	case tuiItemUser:
		body := lipgloss.NewStyle().Width(bodyWidth).Render(item.Body)
		return m.styles.userCard.Width(width).Render(fmt.Sprintf("%s\n%s", timestamp+"  "+item.Title, body))
	case tuiItemAssistant:
		body := item.Body
		if item.Markdown {
			body = m.renderMarkdown(item.Body, bodyWidth)
		} else {
			body = lipgloss.NewStyle().Width(bodyWidth).Render(item.Body)
		}
		return m.styles.assistantCard.Width(width).Render(fmt.Sprintf("%s\n%s", timestamp+"  "+item.Title, body))
	case tuiItemApproval:
		body := lipgloss.NewStyle().Width(bodyWidth).Render(item.Body)
		return m.styles.approvalCard.Width(width).Render(fmt.Sprintf("%s\n%s", timestamp+"  "+item.Title, body))
	case tuiItemError:
		body := lipgloss.NewStyle().Width(bodyWidth).Render(item.Body)
		return m.styles.errorCard.Width(width).Render(fmt.Sprintf("%s\n%s", timestamp+"  "+item.Title, body))
	case tuiItemProgress:
		return m.styles.progressLine.Width(width).Render(fmt.Sprintf("%s  %s", timestamp, truncateRight(item.Body, width-2)))
	default:
		return m.styles.systemLine.Width(width).Render(fmt.Sprintf("%s  %s", timestamp, truncateRight(item.Body, width-2)))
	}
}

func (m *tuiModel) renderMarkdown(markdown string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
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
	return func() tea.Msg {
		before := snapshot
		after := snapshot
		result, err := executeSlashCommandText(ctx, runtime.svc, runtime.store, &after, line)
		if err != nil {
			return tuiExecDoneMsg{Input: line, Before: before, After: after, Err: err}
		}
		if paneTitle == "" {
			paneTitle = result.PaneTitle
		}
		return tuiExecDoneMsg{
			Input:     line,
			Before:    before,
			After:     after,
			Text:      result.Text,
			Pane:      pane || result.Pane,
			PaneTitle: paneTitle,
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
		return tuiTimelineItem{Kind: tuiItemApproval, Title: "Approval Required", Body: body, CreatedAt: event.CreatedAt}
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

func sessionLabel(meta protocol.SessionMeta) string {
	if strings.TrimSpace(meta.Name) != "" {
		return meta.Name
	}
	if len(meta.SessionID) <= 14 {
		return meta.SessionID
	}
	return meta.SessionID[:14]
}

func limitChoices(choices []tuiChoice, limit int) []tuiChoice {
	if limit <= 0 || len(choices) <= limit {
		return choices
	}
	return choices[:limit]
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
