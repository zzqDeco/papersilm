package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/storage"
	buildversion "github.com/zzqDeco/papersilm/internal/version"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestVersionCommandPrintsBuildMetadata(t *testing.T) {
	originalVersion := buildversion.Version
	originalCommit := buildversion.Commit
	originalDate := buildversion.Date
	t.Cleanup(func() {
		buildversion.Version = originalVersion
		buildversion.Commit = originalCommit
		buildversion.Date = originalDate
	})

	buildversion.Version = "v0.1.0"
	buildversion.Commit = "abc123"
	buildversion.Date = "2026-03-12T00:00:00Z"

	cmd := NewRootCommand(context.Background())
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(version): %v", err)
	}

	output := buf.String()
	for _, want := range []string{"version=v0.1.0", "commit=abc123", "date=2026-03-12T00:00:00Z"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestRootCommandFallsBackToREPLOnTUIStartupError(t *testing.T) {
	origLoadConfig := rootLoadConfig
	origShouldUseTUI := rootShouldUseTUI
	origRunTUI := rootRunTUI
	origBuildRuntime := rootBuildRuntime
	origPrepare := rootPrepareSessionSnapshot
	origRunREPL := rootRunREPL
	t.Cleanup(func() {
		rootLoadConfig = origLoadConfig
		rootShouldUseTUI = origShouldUseTUI
		rootRunTUI = origRunTUI
		rootBuildRuntime = origBuildRuntime
		rootPrepareSessionSnapshot = origPrepare
		rootRunREPL = origRunREPL
	})

	rootLoadConfig = func() (config.Config, error) {
		return config.Default(), nil
	}
	rootShouldUseTUI = func(protocol.OutputFormat, string) bool {
		return true
	}
	rootRunTUI = func(context.Context, TUIOptions) error {
		return &tuiStartupError{attempts: []string{"mouse mode failed: boom"}}
	}

	replCalled := false
	rootBuildRuntime = func(context.Context, string) (config.Config, *storage.Store, *core.Service, *OutputWriter, error) {
		return config.Default(), nil, nil, NewOutputWriter(&bytes.Buffer{}, protocol.OutputFormatText), nil
	}
	rootPrepareSessionSnapshot = func(context.Context, *core.Service, *storage.Store, protocol.PermissionMode, string, string, bool, string) (protocol.SessionSnapshot, error) {
		return protocol.SessionSnapshot{}, nil
	}
	rootRunREPL = func(context.Context, *core.Service, *storage.Store, protocol.SessionSnapshot, *OutputWriter) error {
		replCalled = true
		return nil
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewRootCommand(context.Background())
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if !replCalled {
		t.Fatalf("expected fallback REPL path to run")
	}
	if got := stderr.String(); !strings.Contains(got, "tui unavailable, falling back to plain repl") {
		t.Fatalf("expected fallback warning, got %q", got)
	}
}

func TestRootCommandReturnsNonStartupTUIError(t *testing.T) {
	origLoadConfig := rootLoadConfig
	origShouldUseTUI := rootShouldUseTUI
	origRunTUI := rootRunTUI
	origBuildRuntime := rootBuildRuntime
	t.Cleanup(func() {
		rootLoadConfig = origLoadConfig
		rootShouldUseTUI = origShouldUseTUI
		rootRunTUI = origRunTUI
		rootBuildRuntime = origBuildRuntime
	})

	rootLoadConfig = func() (config.Config, error) {
		return config.Default(), nil
	}
	rootShouldUseTUI = func(protocol.OutputFormat, string) bool {
		return true
	}

	want := errors.New("runtime exploded")
	rootRunTUI = func(context.Context, TUIOptions) error {
		return want
	}
	rootBuildRuntime = func(context.Context, string) (config.Config, *storage.Store, *core.Service, *OutputWriter, error) {
		t.Fatalf("buildRuntime should not be called when TUI returns a non-startup error")
		return config.Config{}, nil, nil, nil, nil
	}

	cmd := NewRootCommand(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}
