package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestSplitWorkspaceCommandPreservesBody(t *testing.T) {
	t.Parallel()

	head, body := splitWorkspaceCommand("/workspace note add paper_1 :: this is a workspace body with spaces")
	if head != "/workspace note add paper_1" {
		t.Fatalf("unexpected head: %q", head)
	}
	if body != "this is a workspace body with spaces" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestParseWorkspaceAnchor(t *testing.T) {
	t.Parallel()

	pageAnchor, err := parseWorkspaceAnchor("page", []string{"3"})
	if err != nil {
		t.Fatalf("parse page anchor: %v", err)
	}
	if pageAnchor.Kind != protocol.AnchorKindPage || pageAnchor.Page != 3 {
		t.Fatalf("unexpected page anchor: %+v", pageAnchor)
	}

	snippetAnchor, err := parseWorkspaceAnchor("snippet", []string{"Attention", "improves", "accuracy"})
	if err != nil {
		t.Fatalf("parse snippet anchor: %v", err)
	}
	if snippetAnchor.Kind != protocol.AnchorKindSnippet || snippetAnchor.Snippet != "Attention improves accuracy" {
		t.Fatalf("unexpected snippet anchor: %+v", snippetAnchor)
	}

	sectionAnchor, err := parseWorkspaceAnchor("section", []string{"Experimental", "Setup"})
	if err != nil {
		t.Fatalf("parse section anchor: %v", err)
	}
	if sectionAnchor.Kind != protocol.AnchorKindSection || sectionAnchor.Section != "Experimental Setup" {
		t.Fatalf("unexpected section anchor: %+v", sectionAnchor)
	}

	if _, err := parseWorkspaceAnchor("page", []string{"0"}); err == nil {
		t.Fatalf("expected invalid page anchor error")
	}
}

func TestHandleSlashThemePersistsConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.Default()
	cfg.BaseDir = filepath.Join(home, ".papersilm")
	if err := config.Save(config.ConfigPath(cfg.BaseDir), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var buf bytes.Buffer
	out := NewOutputWriter(&buf, protocol.OutputFormatText)
	session := protocol.SessionSnapshot{}
	if err := handleSlash(context.Background(), nil, nil, &session, out, "/theme light"); err != nil {
		t.Fatalf("handle /theme: %v", err)
	}

	loaded, err := config.Load(config.ConfigPath(cfg.BaseDir))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Theme != config.ThemeLight {
		t.Fatalf("expected theme light, got %q", loaded.Theme)
	}
	if got := buf.String(); got != "theme set to light\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestHandleSlashThemeRejectsInvalidValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var buf bytes.Buffer
	out := NewOutputWriter(&buf, protocol.OutputFormatText)
	session := protocol.SessionSnapshot{}
	err := handleSlash(context.Background(), nil, nil, &session, out, "/theme neon")
	if err == nil {
		t.Fatalf("expected invalid theme error")
	}
}

func TestParseHintsCommand(t *testing.T) {
	t.Parallel()

	visible, ok, err := parseHintsCommand("/hints", true)
	if err != nil || !ok || visible {
		t.Fatalf("expected /hints to toggle off, got visible=%v ok=%v err=%v", visible, ok, err)
	}

	visible, ok, err = parseHintsCommand("/hints on", false)
	if err != nil || !ok || !visible {
		t.Fatalf("expected /hints on to enable hints, got visible=%v ok=%v err=%v", visible, ok, err)
	}

	_, ok, err = parseHintsCommand("/hints neon", true)
	if !ok || err == nil {
		t.Fatalf("expected invalid /hints subcommand error, got ok=%v err=%v", ok, err)
	}
}
