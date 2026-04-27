package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigratesLegacyProviderConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := `
base_dir: /tmp/papersilm
default_lang: zh
default_style: distill
permission_mode: confirm
provider:
  provider: openai
  model: gpt-5.4
  base_url: http://127.0.0.1:8317/v1
  api_key: local-test
  timeout: 90s
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(raw)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ActiveProvider != DefaultProviderProfile {
		t.Fatalf("unexpected active provider: %q", cfg.ActiveProvider)
	}
	if cfg.Theme != ThemeAuto {
		t.Fatalf("expected missing theme to default to auto, got %q", cfg.Theme)
	}
	profile, ok := cfg.Providers[DefaultProviderProfile]
	if !ok {
		t.Fatalf("expected default profile in %+v", cfg.Providers)
	}
	if profile.Model != "gpt-5.4" || profile.BaseURL != "http://127.0.0.1:8317/v1" {
		t.Fatalf("unexpected migrated provider: %+v", profile)
	}
	if cfg.Provider.Model != profile.Model || cfg.Provider.BaseURL != profile.BaseURL {
		t.Fatalf("expected legacy provider mirror, got %+v vs %+v", cfg.Provider, profile)
	}
}

func TestSaveWritesActiveProviderAndLegacyMirror(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := Default()
	cfg.Theme = ThemeLight
	cfg.ActiveProvider = "local-openai"
	cfg.Providers = map[string]ProviderConfig{
		"local-openai": {
			Provider: ProviderOpenAI,
			Model:    "gpt-5.4",
			BaseURL:  "http://127.0.0.1:8317/v1",
			APIKey:   "local-test",
			Timeout:  "2m",
		},
		"ollama": {
			Provider: ProviderOllama,
			Model:    "qwen2.5:7b",
			BaseURL:  "http://127.0.0.1:11434",
			Timeout:  "2m",
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(saved): %v", err)
	}
	if loaded.ActiveProvider != "local-openai" {
		t.Fatalf("unexpected active provider: %q", loaded.ActiveProvider)
	}
	if loaded.Theme != ThemeLight {
		t.Fatalf("unexpected theme: %q", loaded.Theme)
	}
	if loaded.Provider.Model != "gpt-5.4" {
		t.Fatalf("expected legacy provider mirror to point at active model, got %+v", loaded.Provider)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(saved): %v", err)
	}
	text := string(raw)
	for _, want := range []string{"theme: light", "active_provider: local-openai", "providers:", "provider:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in saved config:\n%s", want, text)
		}
	}
}

func TestSetThemeValidatesValues(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if err := cfg.SetTheme(ThemeDark); err != nil {
		t.Fatalf("SetTheme(dark): %v", err)
	}
	if cfg.Theme != ThemeDark {
		t.Fatalf("expected theme dark, got %q", cfg.Theme)
	}
	if err := cfg.SetTheme("sepia"); err == nil {
		t.Fatalf("expected invalid theme error")
	}
}
