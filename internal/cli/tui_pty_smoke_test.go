package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"

	"github.com/zzqDeco/papersilm/internal/config"
)

type ptyScenario struct {
	Name           string
	Width          int
	Height         int
	Theme          config.ThemeSetting
	Setup          func(*tuiModel)
	Script         func(*testing.T, *ptyRun)
	Required       []string
	Forbidden      []string
	RequirePrompt  bool
	RequireAltANSI bool
}

type ptyRun struct {
	program      *tea.Program
	master       *fileWriter
	slave        *fileWriter
	output       *lockedBuffer
	scenarioName string
	width        int
	height       int
}

type fileWriter struct {
	file *os.File
}

func (w *fileWriter) Write(data []byte) (int, error) {
	return w.file.Write(data)
}

func (w *fileWriter) Read(data []byte) (int, error) {
	return w.file.Read(data)
}

func (w *fileWriter) Close() error {
	return w.file.Close()
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestTUIPTYSmokeScenarios(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty smoke tests require a unix pseudo-terminal")
	}

	for _, scenario := range ptyScenarios() {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			raw, frame := runPTYTUI(t, scenario)
			assertPTYSmokeFrame(t, scenario, raw, frame)
		})
	}
}

func TestPTYScreenBufferHonorsEraseAndWrap(t *testing.T) {
	t.Parallel()

	lineErased := normalizePTYSnapshot(renderPTYScreen("\x1b[2J\x1b[Habcdef\x1b[1;4H\x1b[K", 8, 3))
	if strings.Contains(lineErased, "def") || !strings.Contains(lineErased, "abc") {
		t.Fatalf("expected CSI K to erase from cursor to line end, got %q", lineErased)
	}

	screenErased := normalizePTYSnapshot(renderPTYScreen("\x1b[2J\x1b[Htop\r\nmiddle\r\nbottom\x1b[2;4H\x1b[J", 8, 4))
	if strings.Contains(screenErased, "dle") || strings.Contains(screenErased, "bottom") || !strings.Contains(screenErased, "mid") {
		t.Fatalf("expected CSI J to erase from cursor to screen end, got %q", screenErased)
	}

	wrapped := normalizePTYSnapshot(renderPTYScreen(strings.Repeat("a", 20)+"bc", 20, 3))
	if wrapped != strings.Repeat("a", 20)+"\nbc" {
		t.Fatalf("expected overflow to wrap instead of overwriting the last cell, got %q", wrapped)
	}
}

func ptyScenarios() []ptyScenario {
	const (
		width  = 80
		height = 28
	)
	baseForbidden := globalVisualForbiddenStrings()
	return []ptyScenario{
		{
			Name:           "idle",
			Width:          width,
			Height:         height,
			Theme:          config.ThemeDark,
			Required:       []string{"papersilm", "workspace ready", "› Ask about workspace or papers", "? shortcuts"},
			Forbidden:      baseForbidden,
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "typing",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "draft-pty-input 中文")
			},
			Required:       []string{"draft-pty-input", "中文"},
			Forbidden:      append([]string{"? shortcuts"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "slash_suggestions",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "/")
			},
			Required:       []string{"› /", "/help", "/commands"},
			Forbidden:      baseForbidden,
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "slash_suggestions_esc",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "/")
				waitForPTYFrame(t, run, width, []string{"/help", "› /"}, baseForbidden)
				writePTYInput(t, run, "\x1b")
			},
			Required:       []string{"papersilm", "workspace ready", "› /"},
			Forbidden:      append([]string{"/help", "/commands", "/transcript"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "command_drawer",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Setup: func(m *tuiModel) {
				_ = m.openCommandPalette()
			},
			Required:       []string{"Command Palette", "/help", "Enter insert · Esc close", "› Ask about workspace or papers"},
			Forbidden:      baseForbidden,
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "command_drawer_esc",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Setup: func(m *tuiModel) {
				_ = m.openCommandPalette()
			},
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "\x1b")
			},
			Required:       []string{"papersilm", "workspace ready", "› Ask about workspace or papers"},
			Forbidden:      append([]string{"Command Palette", "Enter insert · Esc close"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:           "model_drawer",
			Width:          width,
			Height:         height,
			Theme:          config.ThemeDark,
			Setup:          setupVisualModelDrawer,
			Required:       []string{"Model Picker", "gpt-visual", "Enter switch model · Esc close", "› Ask about workspace or papers"},
			Forbidden:      baseForbidden,
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:           "permission_edit",
			Width:          width,
			Height:         height,
			Theme:          config.ThemeDark,
			Setup:          setupVisualPermissionEdit,
			Required:       []string{"Edit README.md", "Do you want to make this edit?", "Enter select", "› Ask about workspace or papers", "permission pending"},
			Forbidden:      append([]string{"Enter yes"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "permission_feedback_tab",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Setup:  setupVisualPermissionEdit,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "\tuse tests only")
			},
			Required:       []string{"Edit README.md", "Yes and tell papersilm what to do next", "use tests only", "Enter submit · Ctrl+J newline · Esc cancel", "› Ask about workspace or papers"},
			Forbidden:      append([]string{"Enter yes"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:   "permission_details_ctrl_e",
			Width:  width,
			Height: height,
			Theme:  config.ThemeDark,
			Setup:  setupVisualPermissionEdit,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				writePTYInput(t, run, "\x05")
			},
			Required:       []string{"Permission Details", "Context", "README.md", "› Ask about workspace or papers"},
			Forbidden:      append([]string{"Enter yes"}, baseForbidden...),
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
		{
			Name:           "transcript_search",
			Width:          width,
			Height:         height,
			Theme:          config.ThemeDark,
			Setup:          setupVisualTranscriptSearch,
			Required:       []string{"/ search transcript", "Workspace summary", "summary"},
			Forbidden:      baseForbidden,
			RequireAltANSI: true,
		},
		{
			Name:   "resize_80",
			Width:  120,
			Height: height,
			Theme:  config.ThemeDark,
			Script: func(t *testing.T, run *ptyRun) {
				t.Helper()
				resizePTY(t, run, 80, height)
			},
			Required:       []string{"papersilm", "workspace ready", "› Ask about workspace or papers"},
			Forbidden:      baseForbidden,
			RequirePrompt:  true,
			RequireAltANSI: true,
		},
	}
}

func runPTYTUI(t *testing.T, scenario ptyScenario) (string, string) {
	t.Helper()

	master, slave, err := pty.Open()
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skipf("pty unsupported: %v", err)
		}
		t.Fatalf("open pty: %v", err)
	}

	run := &ptyRun{
		master:       &fileWriter{file: master},
		slave:        &fileWriter{file: slave},
		output:       &lockedBuffer{},
		scenarioName: scenario.Name,
		width:        scenario.Width,
		height:       scenario.Height,
	}
	t.Cleanup(func() {
		_ = run.master.Close()
		_ = run.slave.Close()
	})
	if err := pty.Setsize(slave, &pty.Winsize{Rows: uint16(scenario.Height), Cols: uint16(scenario.Width)}); err != nil {
		t.Fatalf("set pty size: %v", err)
	}

	model := newVisualTestTUIModel(scenario.Theme, scenario.Width, scenario.Height)
	if model.runtime.sink == nil {
		model.runtime.sink = newTUIEventSink(16)
	}
	if scenario.Setup != nil {
		scenario.Setup(model)
	}
	model.reflow()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	program := tea.NewProgram(
		model,
		tea.WithInput(slave),
		tea.WithOutput(slave),
		tea.WithAltScreen(),
		tea.WithoutSignals(),
		tea.WithContext(ctx),
	)
	run.program = program

	readDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(run.output, master)
		readDone <- copyErr
	}()

	runDone := make(chan error, 1)
	go func() {
		_, runErr := program.Run()
		runDone <- runErr
	}()

	program.Send(tea.WindowSizeMsg{Width: scenario.Width, Height: scenario.Height})
	waitForPTYFrame(t, run, scenario.Width, []string{"papersilm"}, nil)

	if scenario.Script != nil {
		scenario.Script(t, run)
	}

	required := append([]string{}, scenario.Required...)
	if scenario.RequirePrompt {
		required = append(required, "›")
	}
	raw, frame := waitForPTYFrame(t, run, effectivePTYWidth(scenario), required, scenario.Forbidden)

	program.Kill()
	_ = run.slave.Close()
	_ = run.master.Close()

	select {
	case err := <-runDone:
		if err != nil && !errors.Is(err, tea.ErrProgramKilled) && !errors.Is(err, context.Canceled) {
			t.Fatalf("run program: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("tui program did not stop")
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatalf("pty reader did not stop")
	}

	return raw, frame
}

func waitForPTYFrame(t *testing.T, run *ptyRun, width int, required []string, forbidden []string) (string, string) {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	var raw, frame string
	for time.Now().Before(deadline) {
		raw = run.output.String()
		frame = visiblePTYFrame(raw, width, run.height)
		if ptyFrameContainsAll(frame, required) && !ptyFrameContainsAny(frame, forbidden) && ptyFrameLinesFit(frame, width) {
			return raw, frame
		}
		time.Sleep(20 * time.Millisecond)
	}
	artifactDir := dumpPTYFailureArtifacts(t, ptyFailureArtifact{
		ScenarioName: run.scenarioName,
		Reason:       fmt.Sprintf("timed out waiting for PTY frame with required strings: %v", required),
		Width:        width,
		Height:       run.height,
		Raw:          raw,
		Frame:        frame,
		Required:     required,
		Forbidden:    forbidden,
	})
	t.Fatalf("timed out waiting for PTY frame with %v\nPTY failure artifacts: %s\n--- raw ---\n%q\n--- frame ---\n%s", required, artifactDir, raw, frame)
	return "", ""
}

func writePTYInput(t *testing.T, run *ptyRun, input string) {
	t.Helper()
	if _, err := run.master.Write([]byte(input)); err != nil {
		t.Fatalf("write pty input %q: %v", input, err)
	}
}

func resizePTY(t *testing.T, run *ptyRun, width, height int) {
	t.Helper()
	if err := pty.Setsize(run.slave.file, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)}); err != nil {
		t.Fatalf("resize pty: %v", err)
	}
	run.width = width
	run.height = height
	run.program.Send(tea.WindowSizeMsg{Width: width, Height: height})
}

func assertPTYSmokeFrame(t *testing.T, scenario ptyScenario, raw string, frame string) {
	t.Helper()
	if scenario.RequireAltANSI && !strings.Contains(raw, "\x1b[?1049h") {
		failPTYSmokeFrame(t, scenario, raw, frame, "expected alt-screen ANSI in raw output")
	}
	for _, want := range scenario.Required {
		if !strings.Contains(frame, want) {
			failPTYSmokeFrame(t, scenario, raw, frame, fmt.Sprintf("expected %q in PTY frame", want))
		}
	}
	if scenario.RequirePrompt && !strings.Contains(frame, "›") {
		failPTYSmokeFrame(t, scenario, raw, frame, "expected prompt marker in PTY frame")
	}
	for _, bad := range scenario.Forbidden {
		if strings.Contains(frame, bad) {
			failPTYSmokeFrame(t, scenario, raw, frame, fmt.Sprintf("did not expect %q in PTY frame", bad))
		}
	}
	if !ptyFrameLinesFit(frame, effectivePTYWidth(scenario)) {
		for lineNo, line := range strings.Split(frame, "\n") {
			if got := lipgloss.Width(line); got > effectivePTYWidth(scenario) {
				failPTYSmokeFrame(t, scenario, raw, frame, fmt.Sprintf("line %d exceeds width %d: got %d: %q", lineNo+1, effectivePTYWidth(scenario), got, line))
			}
		}
	}
	if footer := extractPTYFooter(frame); len(footer) > 2 {
		failPTYSmokeFrame(t, scenario, raw, frame, fmt.Sprintf("expected footer <= 2 lines, got %d: %q", len(footer), footer))
	}
}

type ptyFailureArtifact struct {
	ScenarioName string
	Reason       string
	Width        int
	Height       int
	Raw          string
	Frame        string
	Required     []string
	Forbidden    []string
}

func failPTYSmokeFrame(t *testing.T, scenario ptyScenario, raw, frame, reason string) {
	t.Helper()
	artifactDir := dumpPTYFailureArtifacts(t, ptyFailureArtifact{
		ScenarioName: scenario.Name,
		Reason:       reason,
		Width:        effectivePTYWidth(scenario),
		Height:       scenario.Height,
		Raw:          raw,
		Frame:        frame,
		Required:     scenario.Required,
		Forbidden:    scenario.Forbidden,
	})
	t.Fatalf("%s for %s\nPTY failure artifacts: %s\n--- frame ---\n%s", reason, scenario.Name, artifactDir, frame)
}

func dumpPTYFailureArtifacts(t *testing.T, artifact ptyFailureArtifact) string {
	t.Helper()
	root := os.Getenv("PAPERSILM_TUI_PTY_ARTIFACT_DIR")
	var dir string
	var err error
	if root == "" {
		dir, err = os.MkdirTemp("", "papersilm-tui-pty-*")
	} else {
		if err = os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("create PTY artifact root: %v", err)
		}
		dir, err = os.MkdirTemp(root, sanitizePTYArtifactName(artifact.ScenarioName)+"-*")
	}
	if err != nil {
		t.Fatalf("create PTY artifact dir: %v", err)
	}

	files := map[string]string{
		"frame.normalized.txt": artifact.Frame,
		"raw.ansi":             artifact.Raw,
		"raw.escaped.txt":      fmt.Sprintf("%q\n", artifact.Raw),
		"screen.rendered.txt":  renderPTYScreen(artifact.Raw, artifact.Width, artifact.Height),
		"scenario.txt": fmt.Sprintf(
			"scenario: %s\nreason: %s\nwidth: %d\nheight: %d\nrequired: %s\nforbidden: %s\n",
			artifact.ScenarioName,
			artifact.Reason,
			artifact.Width,
			artifact.Height,
			strings.Join(artifact.Required, ", "),
			strings.Join(artifact.Forbidden, ", "),
		),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write PTY artifact %s: %v", name, err)
		}
	}
	return dir
}

func sanitizePTYArtifactName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var builder strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	cleaned := strings.Trim(builder.String(), "-")
	if cleaned == "" {
		return "scenario"
	}
	return cleaned
}

func visiblePTYFrame(raw string, width, height int) string {
	return normalizePTYSnapshot(renderPTYScreen(raw, width, height))
}

func renderPTYScreen(raw string, width, height int) string {
	width = max(20, width)
	height = max(1, height)
	screen := make([][]string, height)
	for row := range screen {
		screen[row] = make([]string, width)
		for col := range screen[row] {
			screen[row][col] = " "
		}
	}
	row, col := 0, 0
	for i := 0; i < len(raw); {
		ch := raw[i]
		switch ch {
		case '\x1b':
			next, newRow, newCol := consumePTYEscape(raw, i, row, col, screen)
			row, col = clamp(newRow, 0, height-1), clamp(newCol, 0, width-1)
			i = next
		case '\r':
			col = 0
			i++
		case '\n':
			if row < height-1 {
				row++
			}
			i++
		case '\t':
			nextTab := min(width-1, ((col/4)+1)*4)
			for col < nextTab {
				screen[row][col] = " "
				col++
			}
			i++
		default:
			r, size := nextPTYRune(raw[i:])
			if size == 0 {
				i++
				continue
			}
			if r < 0x20 || r == 0x7f {
				i += size
				continue
			}
			cellWidth := max(1, lipgloss.Width(string(r)))
			if col >= width {
				if row < height-1 {
					row++
					col = 0
				} else {
					i += size
					continue
				}
			}
			if col+cellWidth > width && cellWidth > 1 {
				if row < height-1 {
					row++
					col = 0
				} else {
					i += size
					continue
				}
			}
			screen[row][col] = string(r)
			for offset := 1; offset < cellWidth && col+offset < width; offset++ {
				screen[row][col+offset] = ""
			}
			col += cellWidth
			if col >= width {
				col = width
			}
			i += size
		}
	}
	lines := make([]string, len(screen))
	for i, cells := range screen {
		lines[i] = strings.Join(cells, "")
	}
	return strings.Join(lines, "\n")
}

func consumePTYEscape(raw string, start, row, col int, screen [][]string) (int, int, int) {
	if start+1 >= len(raw) {
		return start + 1, row, col
	}
	switch raw[start+1] {
	case '[':
		end := start + 2
		for end < len(raw) {
			final := raw[end]
			if final >= 0x40 && final <= 0x7e {
				break
			}
			end++
		}
		if end >= len(raw) {
			return len(raw), row, col
		}
		params := raw[start+2 : end]
		switch raw[end] {
		case 'H', 'f':
			nextRow, nextCol := parsePTYCursor(params)
			return end + 1, nextRow, nextCol
		case 'J':
			erasePTYScreen(screen, row, col, firstCSIParam(params, 0))
			return end + 1, row, col
		case 'K':
			erasePTYLine(screen, row, col, firstCSIParam(params, 0))
			return end + 1, row, col
		default:
			return end + 1, row, col
		}
	case ']':
		end := start + 2
		for end < len(raw) {
			if raw[end] == '\a' {
				return end + 1, row, col
			}
			if raw[end] == '\x1b' && end+1 < len(raw) && raw[end+1] == '\\' {
				return end + 2, row, col
			}
			end++
		}
		return len(raw), row, col
	default:
		return start + 2, row, col
	}
}

func firstCSIParam(params string, fallback int) int {
	params = strings.TrimPrefix(params, "?")
	if params == "" {
		return fallback
	}
	head := params
	if idx := strings.Index(head, ";"); idx >= 0 {
		head = head[:idx]
	}
	if head == "" {
		return fallback
	}
	value, err := strconv.Atoi(head)
	if err != nil {
		return fallback
	}
	return value
}

func parsePTYCursor(params string) (int, int) {
	params = strings.TrimPrefix(params, "?")
	if params == "" {
		return 0, 0
	}
	parts := strings.Split(params, ";")
	row, col := 1, 1
	if len(parts) > 0 && parts[0] != "" {
		if value, err := strconv.Atoi(parts[0]); err == nil && value > 0 {
			row = value
		}
	}
	if len(parts) > 1 && parts[1] != "" {
		if value, err := strconv.Atoi(parts[1]); err == nil && value > 0 {
			col = value
		}
	}
	return row - 1, col - 1
}

func clearPTYScreen(screen [][]string) {
	for row := range screen {
		for col := range screen[row] {
			screen[row][col] = " "
		}
	}
}

func erasePTYScreen(screen [][]string, row, col, mode int) {
	if len(screen) == 0 {
		return
	}
	row = clamp(row, 0, len(screen)-1)
	col = clamp(col, 0, len(screen[row]))
	switch mode {
	case 1:
		for r := 0; r < row; r++ {
			clearPTYLineRange(screen[r], 0, len(screen[r]))
		}
		clearPTYLineRange(screen[row], 0, min(col+1, len(screen[row])))
	default:
		if mode == 2 || mode == 3 {
			clearPTYScreen(screen)
			return
		}
		clearPTYLineRange(screen[row], col, len(screen[row]))
		for r := row + 1; r < len(screen); r++ {
			clearPTYLineRange(screen[r], 0, len(screen[r]))
		}
	}
}

func erasePTYLine(screen [][]string, row, col, mode int) {
	if len(screen) == 0 {
		return
	}
	row = clamp(row, 0, len(screen)-1)
	col = clamp(col, 0, len(screen[row]))
	switch mode {
	case 1:
		clearPTYLineRange(screen[row], 0, min(col+1, len(screen[row])))
	case 2:
		clearPTYLineRange(screen[row], 0, len(screen[row]))
	default:
		clearPTYLineRange(screen[row], col, len(screen[row]))
	}
}

func clearPTYLineRange(line []string, start, end int) {
	start = clamp(start, 0, len(line))
	end = clamp(end, start, len(line))
	for col := start; col < end; col++ {
		line[col] = " "
	}
}

func nextPTYRune(value string) (rune, int) {
	if value == "" {
		return 0, 0
	}
	r, size := utf8.DecodeRuneInString(value)
	if r == utf8.RuneError && size == 0 {
		return 0, 0
	}
	return r, size
}

func normalizePTYSnapshot(view string) string {
	view = visualOSCRegexp.ReplaceAllString(view, "")
	view = visualCSIRegexp.ReplaceAllString(view, "")
	view = strings.ReplaceAll(view, "\r\n", "\n")
	view = strings.ReplaceAll(view, "\r", "\n")
	view = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		if r == 0x7f {
			return -1
		}
		return r
	}, view)
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func ptyFrameContainsAll(frame string, required []string) bool {
	for _, want := range required {
		if !strings.Contains(frame, want) {
			return false
		}
	}
	return true
}

func ptyFrameContainsAny(frame string, forbidden []string) bool {
	for _, bad := range forbidden {
		if strings.Contains(frame, bad) {
			return true
		}
	}
	return false
}

func ptyFrameLinesFit(frame string, width int) bool {
	for _, line := range strings.Split(frame, "\n") {
		if lipgloss.Width(line) > width {
			return false
		}
	}
	return true
}

func extractPTYFooter(frame string) []string {
	lines := strings.Split(frame, "\n")
	footer := make([]string, 0, 2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "? shortcuts") ||
			strings.Contains(trimmed, "permission pending") ||
			strings.Contains(trimmed, "? transcript") {
			footer = append(footer, trimmed)
		}
	}
	return footer
}

func effectivePTYWidth(scenario ptyScenario) int {
	if scenario.Name == "resize_80" {
		return 80
	}
	return scenario.Width
}
