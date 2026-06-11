package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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
	program *tea.Program
	master  *fileWriter
	slave   *fileWriter
	output  *lockedBuffer
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
		master: &fileWriter{file: master},
		slave:  &fileWriter{file: slave},
		output: &lockedBuffer{},
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
	waitForPTYFrame(t, run, scenario.Width, []string{"papersilm"})

	if scenario.Script != nil {
		scenario.Script(t, run)
	}

	required := append([]string{}, scenario.Required...)
	if scenario.RequirePrompt {
		required = append(required, "›")
	}
	raw, frame := waitForPTYFrame(t, run, effectivePTYWidth(scenario), required)

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

func waitForPTYFrame(t *testing.T, run *ptyRun, width int, required []string) (string, string) {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	var raw, frame string
	for time.Now().Before(deadline) {
		raw = run.output.String()
		frame = visiblePTYFrame(raw)
		if ptyFrameContainsAll(frame, required) && ptyFrameLinesFit(frame, width) {
			return raw, frame
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY frame with %v\n--- raw ---\n%q\n--- frame ---\n%s", required, raw, frame)
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
	run.program.Send(tea.WindowSizeMsg{Width: width, Height: height})
}

func assertPTYSmokeFrame(t *testing.T, scenario ptyScenario, raw string, frame string) {
	t.Helper()
	if scenario.RequireAltANSI && !strings.Contains(raw, "\x1b[?1049h") {
		t.Fatalf("expected alt-screen ANSI in raw output for %s\nraw=%q", scenario.Name, raw)
	}
	for _, want := range scenario.Required {
		if !strings.Contains(frame, want) {
			t.Fatalf("expected %q in PTY frame for %s\n%s", want, scenario.Name, frame)
		}
	}
	if scenario.RequirePrompt && !strings.Contains(frame, "›") {
		t.Fatalf("expected prompt marker in PTY frame for %s\n%s", scenario.Name, frame)
	}
	for _, bad := range scenario.Forbidden {
		if strings.Contains(frame, bad) {
			t.Fatalf("did not expect %q in PTY frame for %s\n%s", bad, scenario.Name, frame)
		}
	}
	if !ptyFrameLinesFit(frame, effectivePTYWidth(scenario)) {
		for lineNo, line := range strings.Split(frame, "\n") {
			if got := lipgloss.Width(line); got > effectivePTYWidth(scenario) {
				t.Fatalf("line %d in %s exceeds width %d: got %d: %q\n%s", lineNo+1, scenario.Name, effectivePTYWidth(scenario), got, line, frame)
			}
		}
	}
	if footer := extractPTYFooter(frame); len(footer) > 2 {
		t.Fatalf("expected footer <= 2 lines for %s, got %d: %q\n%s", scenario.Name, len(footer), footer, frame)
	}
}

func visiblePTYFrame(raw string) string {
	raw = strings.ReplaceAll(raw, "\x1b[?1049h", "\n")
	if idx := strings.LastIndex(raw, "\x1b[H"); idx >= 0 {
		raw = raw[idx:]
	}
	return normalizePTYSnapshot(raw)
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
