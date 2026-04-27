package cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/zzqDeco/papersilm/internal/config"
)

type tuiPalette struct {
	text         lipgloss.Color
	inactive     lipgloss.Color
	subtle       lipgloss.Color
	brand        lipgloss.Color
	suggestion   lipgloss.Color
	success      lipgloss.Color
	permission   lipgloss.Color
	warning      lipgloss.Color
	error        lipgloss.Color
	divider      lipgloss.Color
	promptBorder lipgloss.Color
	surface      lipgloss.Color
	userBg       lipgloss.Color
	selectionBg  lipgloss.Color
}

func newTUIStyles(setting config.ThemeSetting) tuiStyles {
	resolved := resolveTUITheme(setting)
	lipgloss.SetHasDarkBackground(resolved == config.ThemeDark)

	palette := tuiDarkPalette()
	if resolved == config.ThemeLight {
		palette = tuiLightPalette()
	}

	background := lipgloss.NewStyle().Foreground(palette.text)
	body := lipgloss.NewStyle().Foreground(palette.text)
	if setting == config.ThemeAuto {
		// In auto mode, terminal background detection is best-effort. Keep body
		// and prompt input on the terminal default foreground so typed text
		// remains visible even when OSC/COLORFGBG detection is unavailable.
		background = lipgloss.NewStyle()
		body = lipgloss.NewStyle()
	}
	inputShell := lipgloss.NewStyle()
	footer := lipgloss.NewStyle()
	paneBody := lipgloss.NewStyle().Foreground(palette.text).Padding(0, 2)
	if setting == config.ThemeAuto {
		paneBody = lipgloss.NewStyle().Padding(0, 2)
	}
	modalShell := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(palette.divider).Padding(0, 1)
	if resolved == config.ThemeLight && setting != config.ThemeAuto {
		background = background.Background(palette.surface)
		body = body.Background(palette.surface)
		inputShell = inputShell.Background(palette.surface)
		footer = footer.Background(palette.surface)
		paneBody = paneBody.Background(palette.surface)
		modalShell = modalShell.Background(palette.surface)
	}

	return tuiStyles{
		theme:                  resolved,
		markdownStyle:          string(resolved),
		background:             background,
		body:                   body,
		header:                 lipgloss.NewStyle().Foreground(palette.inactive),
		headerMuted:            lipgloss.NewStyle().Foreground(palette.inactive),
		headerAccent:           lipgloss.NewStyle().Foreground(palette.brand),
		headerStatus:           lipgloss.NewStyle().Foreground(palette.inactive),
		userShell:              lipgloss.NewStyle().Foreground(palette.text).Background(palette.userBg).Padding(0, 1),
		userLabel:              lipgloss.NewStyle().Foreground(palette.inactive),
		assistantLabel:         lipgloss.NewStyle().Foreground(palette.text),
		approvalShell:          lipgloss.NewStyle().Foreground(palette.text).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(palette.permission).PaddingLeft(1),
		approvalLabel:          lipgloss.NewStyle().Foreground(palette.permission),
		successShell:           lipgloss.NewStyle().Foreground(palette.text).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(palette.success).PaddingLeft(1),
		successLabel:           lipgloss.NewStyle().Foreground(palette.success),
		rejectionShell:         lipgloss.NewStyle().Foreground(palette.text).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(palette.error).PaddingLeft(1),
		rejectionLabel:         lipgloss.NewStyle().Foreground(palette.error),
		errorShell:             lipgloss.NewStyle().Foreground(palette.text).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(palette.error).PaddingLeft(1),
		errorLabel:             lipgloss.NewStyle().Foreground(palette.error),
		paneDivider:            lipgloss.NewStyle().Foreground(palette.divider),
		paneTitle:              lipgloss.NewStyle().Foreground(palette.text),
		paneBody:               paneBody,
		inputShell:             inputShell,
		footer:                 footer,
		footerMuted:            lipgloss.NewStyle().Foreground(palette.inactive),
		footerAccent:           lipgloss.NewStyle().Foreground(palette.suggestion),
		keycap:                 lipgloss.NewStyle().Foreground(palette.text),
		systemLine:             lipgloss.NewStyle().Foreground(palette.inactive),
		progressLine:           lipgloss.NewStyle().Foreground(palette.inactive),
		suggestionMarker:       lipgloss.NewStyle().Foreground(palette.suggestion),
		suggestionLabel:        lipgloss.NewStyle().Foreground(palette.inactive),
		suggestionDetail:       lipgloss.NewStyle().Foreground(palette.subtle),
		suggestionActiveLabel:  lipgloss.NewStyle().Foreground(palette.suggestion),
		suggestionActiveDetail: lipgloss.NewStyle().Foreground(palette.inactive),
		modalShell:             modalShell,
		modalTitle:             lipgloss.NewStyle().Foreground(palette.text),
		modalMessage:           lipgloss.NewStyle().Foreground(palette.inactive),
		modalHint:              lipgloss.NewStyle().Foreground(palette.inactive),
		modalDisabled:          lipgloss.NewStyle().Foreground(palette.error),
	}
}

func tuiDarkPalette() tuiPalette {
	return tuiPalette{
		text:         lipgloss.Color("#FFFFFF"),
		inactive:     lipgloss.Color("#999999"),
		subtle:       lipgloss.Color("#505050"),
		brand:        lipgloss.Color("#D77757"),
		suggestion:   lipgloss.Color("#B1B9F9"),
		success:      lipgloss.Color("#7CCF89"),
		permission:   lipgloss.Color("#5769F7"),
		warning:      lipgloss.Color("#FFC107"),
		error:        lipgloss.Color("#FF6B80"),
		divider:      lipgloss.Color("#505050"),
		promptBorder: lipgloss.Color("#888888"),
		surface:      lipgloss.Color("#FFFFFF"),
		userBg:       lipgloss.Color("#373737"),
		selectionBg:  lipgloss.Color("#264F78"),
	}
}

func tuiLightPalette() tuiPalette {
	return tuiPalette{
		text:         lipgloss.Color("#000000"),
		inactive:     lipgloss.Color("#666666"),
		subtle:       lipgloss.Color("#AFAFAF"),
		brand:        lipgloss.Color("#D77757"),
		suggestion:   lipgloss.Color("#5769F7"),
		success:      lipgloss.Color("#2E8B57"),
		permission:   lipgloss.Color("#5769F7"),
		warning:      lipgloss.Color("#966C1E"),
		error:        lipgloss.Color("#AB2B3F"),
		divider:      lipgloss.Color("#AFAFAF"),
		promptBorder: lipgloss.Color("#999999"),
		surface:      lipgloss.Color("#FFFFFF"),
		userBg:       lipgloss.Color("#F0F0F0"),
		selectionBg:  lipgloss.Color("#B4D5FF"),
	}
}

func resolveTUITheme(setting config.ThemeSetting) config.ThemeSetting {
	switch setting {
	case config.ThemeDark, config.ThemeLight:
		return setting
	default:
		return resolveAutoTUITheme(os.Getenv("COLORFGBG"), lipgloss.HasDarkBackground() || termenv.HasDarkBackground())
	}
}

func resolveAutoTUITheme(colorFGBG string, hasDarkBackground bool) config.ThemeSetting {
	if theme, ok := themeFromColorFGBG(colorFGBG); ok {
		return theme
	}
	if hasDarkBackground {
		return config.ThemeDark
	}
	return config.ThemeDark
}

func themeFromColorFGBG(raw string) (config.ThemeSetting, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parts := strings.Split(raw, ";")
	last := strings.TrimSpace(parts[len(parts)-1])
	value, err := strconv.Atoi(last)
	if err != nil {
		return "", false
	}
	switch value {
	case 7, 15:
		return config.ThemeLight, true
	case 0, 1, 2, 3, 4, 5, 6, 8:
		return config.ThemeDark, true
	default:
		return "", false
	}
}

func applyTextareaTheme(input *textarea.Model, styles tuiStyles) {
	input.Prompt = "› "
	input.FocusedStyle.Base = styles.body
	input.FocusedStyle.Text = styles.body
	input.FocusedStyle.Placeholder = styles.footerMuted
	input.FocusedStyle.Prompt = styles.body
	input.FocusedStyle.CursorLine = styles.body
	input.FocusedStyle.CursorLineNumber = styles.footerMuted
	input.FocusedStyle.LineNumber = styles.footerMuted
	input.FocusedStyle.EndOfBuffer = styles.footerMuted

	input.BlurredStyle = input.FocusedStyle
	input.BlurredStyle.Prompt = styles.footerMuted
	input.Cursor.Style = styles.keycap.Reverse(true)
}

func applyTextInputTheme(input *textinput.Model, styles tuiStyles) {
	input.Prompt = "› "
	input.PromptStyle = styles.body
	input.TextStyle = styles.body
	input.PlaceholderStyle = styles.footerMuted
	input.CompletionStyle = styles.footerAccent
	input.Cursor.Style = styles.keycap.Reverse(true)
}
