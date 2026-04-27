package cli

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/zzqDeco/papersilm/internal/config"
)

func TestThemeFromColorFGBG(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		raw    string
		want   config.ThemeSetting
		wantOK bool
	}{
		{name: "dark background", raw: "15;0", want: config.ThemeDark, wantOK: true},
		{name: "light background", raw: "0;15", want: config.ThemeLight, wantOK: true},
		{name: "unknown background", raw: "0;9", wantOK: false},
		{name: "invalid", raw: "bogus", wantOK: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := themeFromColorFGBG(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("theme=%q want %q", got, tc.want)
			}
		})
	}
}

func TestResolveAutoTUIThemeFallsBackToDark(t *testing.T) {
	t.Parallel()

	if got := resolveAutoTUITheme("", false); got != config.ThemeDark {
		t.Fatalf("expected dark fallback, got %q", got)
	}
}

func TestTUIThemePalettesTrackClaudeCodeTokens(t *testing.T) {
	t.Parallel()

	dark := tuiDarkPalette()
	if dark.text != lipgloss.Color("#FFFFFF") {
		t.Fatalf("unexpected dark text color: %q", dark.text)
	}
	if dark.brand != lipgloss.Color("#D77757") {
		t.Fatalf("unexpected dark brand color: %q", dark.brand)
	}
	if dark.suggestion != lipgloss.Color("#B1B9F9") {
		t.Fatalf("unexpected dark suggestion color: %q", dark.suggestion)
	}
	if dark.success != lipgloss.Color("#7CCF89") {
		t.Fatalf("unexpected dark success color: %q", dark.success)
	}
	if dark.userBg != lipgloss.Color("#373737") {
		t.Fatalf("unexpected dark user background: %q", dark.userBg)
	}

	light := tuiLightPalette()
	if light.text != lipgloss.Color("#000000") {
		t.Fatalf("unexpected light text color: %q", light.text)
	}
	if light.brand != lipgloss.Color("#D77757") {
		t.Fatalf("unexpected light brand color: %q", light.brand)
	}
	if light.suggestion != lipgloss.Color("#5769F7") {
		t.Fatalf("unexpected light suggestion color: %q", light.suggestion)
	}
	if light.success != lipgloss.Color("#2E8B57") {
		t.Fatalf("unexpected light success color: %q", light.success)
	}
	if light.userBg != lipgloss.Color("#F0F0F0") {
		t.Fatalf("unexpected light user background: %q", light.userBg)
	}
}

func TestLightThemePaintsReadableSurfaceBackground(t *testing.T) {
	t.Parallel()

	styles := newTUIStyles(config.ThemeLight)
	if styles.background.GetBackground() == nil {
		t.Fatalf("expected light theme to paint a background surface")
	}
	if styles.body.GetBackground() == nil {
		t.Fatalf("expected light body style to inherit readable surface")
	}
}

func TestAutoThemeKeepsPromptTextOnTerminalDefault(t *testing.T) {
	t.Parallel()

	styles := newTUIStyles(config.ThemeAuto)
	if got := styles.body.Render("typed text"); got != "typed text" {
		t.Fatalf("expected auto body text to use terminal default foreground, got %q", got)
	}
	if got := styles.inputShell.Render("typed text"); got != "typed text" {
		t.Fatalf("expected auto input shell to use terminal default surface, got %q", got)
	}
}

func TestWindowSuggestionsCapsAtFive(t *testing.T) {
	t.Parallel()

	suggestions := []tuiSuggestion{
		{Label: "one"},
		{Label: "two"},
		{Label: "three"},
		{Label: "four"},
		{Label: "five"},
		{Label: "six"},
		{Label: "seven"},
	}

	visible, start := windowSuggestions(suggestions, 5, 5)
	if len(visible) != 5 {
		t.Fatalf("expected 5 visible suggestions, got %d", len(visible))
	}
	if start != 2 {
		t.Fatalf("expected window start 2, got %d", start)
	}
	if visible[3].Label != "six" {
		t.Fatalf("expected selected neighborhood to include six, got %+v", visible)
	}
}
