package tui

type OverlayKind string

const (
	OverlayNone        OverlayKind = ""
	OverlaySuggestions OverlayKind = "suggestions"
	OverlayPalette     OverlayKind = "palette"
	OverlayModelPicker OverlayKind = "model_picker"
	OverlayThemePicker OverlayKind = "theme_picker"
	OverlayHistory     OverlayKind = "history"
	OverlayApproval    OverlayKind = "approval"
)

type PromptOverlay struct {
	Kind      OverlayKind
	Rows      []ListRow
	Selection int
	Message   string
}

type DrawerOverlay struct {
	Kind    OverlayKind
	Title   string
	Message string
	Filter  string
	Rows    []ListRow
}

type OverlayManager struct {
	prompt PromptOverlay
	drawer DrawerOverlay
}

func (m *OverlayManager) SetPrompt(overlay PromptOverlay) {
	m.prompt = overlay
}

func (m *OverlayManager) Prompt() (PromptOverlay, bool) {
	return m.prompt, m.prompt.Kind != OverlayNone
}

func (m *OverlayManager) ClearPrompt() {
	m.prompt = PromptOverlay{}
}

func (m *OverlayManager) SetDrawer(overlay DrawerOverlay) {
	m.drawer = overlay
}

func (m *OverlayManager) Drawer() (DrawerOverlay, bool) {
	return m.drawer, m.drawer.Kind != OverlayNone
}

func (m *OverlayManager) ClearDrawer() {
	m.drawer = DrawerOverlay{}
}

func (m *OverlayManager) ClearAll() {
	m.ClearPrompt()
	m.ClearDrawer()
}
