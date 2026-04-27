package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Drawer renders Claude Code-style bottom overlays: one neutral divider,
// padded content, no centered card frame.
type Drawer struct {
	Width        int
	Title        string
	Message      string
	Filter       string
	Rows         []ListRow
	DividerStyle lipgloss.Style
	TitleStyle   lipgloss.Style
	MutedStyle   lipgloss.Style
	BodyStyle    lipgloss.Style
}

type ListRow struct {
	Label       string
	Detail      string
	Selected    bool
	Disabled    bool
	MarkerStyle lipgloss.Style
	LabelStyle  lipgloss.Style
	DetailStyle lipgloss.Style
}

type FullscreenLayout struct {
	Width         int
	Header        string
	StickyHeader  string
	Scrollable    string
	Bottom        string
	Pane          string
	PromptOverlay string
	ScrollPill    string
}

func RenderFullscreenLayout(layout FullscreenLayout) string {
	width := max(20, layout.Width)
	parts := []string{layout.Header}
	if strings.TrimSpace(layout.StickyHeader) != "" {
		parts = append(parts, layout.StickyHeader)
	}
	parts = append(parts, layout.Scrollable, layout.Bottom)
	base := lipgloss.JoinVertical(lipgloss.Left, parts...)
	bottomHeight := lipgloss.Height(layout.Bottom)
	promptOverlayHeight := 0
	if strings.TrimSpace(layout.PromptOverlay) != "" {
		promptOverlayHeight = lipgloss.Height(layout.PromptOverlay)
	}
	scrollPillHeight := 0
	if strings.TrimSpace(layout.ScrollPill) != "" {
		scrollPillHeight = lipgloss.Height(layout.ScrollPill)
	}

	if strings.TrimSpace(layout.Pane) != "" {
		top := overlayTop(base, bottomHeight+promptOverlayHeight+scrollPillHeight, lipgloss.Height(layout.Pane))
		base = OverlayAt(base, layout.Pane, top, lipgloss.Left, width)
	}
	if strings.TrimSpace(layout.ScrollPill) != "" {
		top := overlayTop(base, bottomHeight+promptOverlayHeight, lipgloss.Height(layout.ScrollPill))
		base = OverlayAt(base, layout.ScrollPill, top, lipgloss.Center, width)
	}
	if promptOverlayHeight > 0 {
		top := overlayTop(base, bottomHeight, promptOverlayHeight)
		base = OverlayAt(base, layout.PromptOverlay, top, lipgloss.Left, width)
	}
	return base
}

func RenderBottomDrawer(drawer Drawer) string {
	width := max(20, drawer.Width)
	bodyWidth := max(16, width-4)
	lines := []string{
		drawer.DividerStyle.Render(strings.Repeat("▔", width)),
	}
	if strings.TrimSpace(drawer.Title) != "" {
		lines = append(lines, "  "+drawer.TitleStyle.Render(truncateRight(drawer.Title, bodyWidth)))
	}
	if strings.TrimSpace(drawer.Message) != "" {
		lines = append(lines, "  "+drawer.MutedStyle.Render(truncateRight(drawer.Message, bodyWidth)))
	}
	if strings.TrimSpace(drawer.Filter) != "" {
		lines = append(lines, "  "+drawer.BodyStyle.Render(truncateRight(drawer.Filter, bodyWidth)))
	}
	if len(drawer.Rows) > 0 {
		lines = append(lines, "")
		lines = append(lines, RenderListRows(drawer.Rows, bodyWidth)...)
	}
	return strings.Join(lines, "\n")
}

func RenderListRows(rows []ListRow, width int) []string {
	if len(rows) == 0 {
		return nil
	}
	labelWidth := 16
	for _, row := range rows {
		if w := lipgloss.Width(row.Label) + 2; w > labelWidth {
			labelWidth = w
		}
	}
	if maxLabel := max(18, width/3); labelWidth > maxLabel {
		labelWidth = maxLabel
	}
	detailWidth := max(0, width-labelWidth-6)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		prefix := "  "
		if row.Selected {
			prefix = "+ "
		}
		label := padRight(truncateRight(row.Label, labelWidth), labelWidth)
		detail := truncateRight(row.Detail, detailWidth)
		line := "  " + row.MarkerStyle.Render(prefix) + row.LabelStyle.Render(label)
		if detail != "" {
			line += row.DetailStyle.Render("  " + detail)
		}
		lines = append(lines, line)
	}
	return lines
}

func OverlayBottom(base, block string, width int) string {
	return OverlayAt(base, block, max(0, len(strings.Split(base, "\n"))-lipgloss.Height(block)), lipgloss.Left, width)
}

func OverlayBottomWithPeek(base, block string, width int, peek int) string {
	baseHeight := len(strings.Split(base, "\n"))
	peek = clamp(peek, 0, max(0, baseHeight-1))
	maxOverlayHeight := max(1, baseHeight-peek)
	block = ClipBlockHeight(block, maxOverlayHeight)
	return OverlayAt(base, block, max(peek, baseHeight-lipgloss.Height(block)), lipgloss.Left, width)
}

func ClipBlockHeight(block string, maxHeight int) string {
	if maxHeight <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	if len(lines) <= maxHeight {
		return block
	}
	return strings.Join(lines[:maxHeight], "\n")
}

func OverlayAt(base, block string, top int, align lipgloss.Position, width int) string {
	baseLines := strings.Split(base, "\n")
	blockLines := strings.Split(block, "\n")
	for i, line := range blockLines {
		idx := top + i
		if idx < 0 {
			continue
		}
		placed := lipgloss.PlaceHorizontal(width, align, line)
		for idx >= len(baseLines) {
			baseLines = append(baseLines, strings.Repeat(" ", width))
		}
		baseLines[idx] = placed
	}
	return strings.Join(baseLines, "\n")
}

func overlayTop(base string, reservedBottom, overlayHeight int) int {
	baseHeight := len(strings.Split(base, "\n"))
	return clamp(baseHeight-reservedBottom-overlayHeight, 0, max(0, baseHeight-overlayHeight))
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
