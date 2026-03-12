package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"papersilm/pkg/protocol"
)

type ProviderType string

const (
	ProviderOpenAI   ProviderType = "openai"
	ProviderArk      ProviderType = "ark"
	ProviderQwen     ProviderType = "qwen"
	ProviderDeepSeek ProviderType = "deepseek"
	ProviderOllama   ProviderType = "ollama"
)

type ProviderConfig struct {
	Provider ProviderType `yaml:"provider" json:"provider"`
	Model    string       `yaml:"model" json:"model"`
	BaseURL  string       `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey   string       `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Timeout  string       `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type Config struct {
	BaseDir        string               `yaml:"base_dir" json:"base_dir"`
	DefaultLang    string               `yaml:"default_lang" json:"default_lang"`
	DefaultStyle   string               `yaml:"default_style" json:"default_style"`
	PermissionMode protocol.PermissionMode `yaml:"permission_mode" json:"permission_mode"`
	Provider       ProviderConfig       `yaml:"provider" json:"provider"`
}

func Default() Config {
	base := filepath.Join(userHomeDir(), ".papersilm")
	return Config{
		BaseDir:        base,
		DefaultLang:    "zh",
		DefaultStyle:   "distill",
		PermissionMode: protocol.PermissionModeConfirm,
		Provider: ProviderConfig{
			Provider: ProviderOpenAI,
			Model:    "",
			BaseURL:  "",
			APIKey:   "",
			Timeout:  "2m",
		},
	}
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func ConfigPath(baseDir string) string {
	if baseDir == "" {
		baseDir = Default().BaseDir
	}
	return filepath.Join(baseDir, "config.yaml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (c Config) ProviderTimeout() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(c.Provider.Timeout))
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

