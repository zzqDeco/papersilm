package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zzqDeco/papersilm/pkg/protocol"
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
	BaseDir        string                    `yaml:"base_dir" json:"base_dir"`
	DefaultLang    string                    `yaml:"default_lang" json:"default_lang"`
	DefaultStyle   string                    `yaml:"default_style" json:"default_style"`
	PermissionMode protocol.PermissionMode   `yaml:"permission_mode" json:"permission_mode"`
	ActiveProvider string                    `yaml:"active_provider,omitempty" json:"active_provider,omitempty"`
	Providers      map[string]ProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
	Provider       ProviderConfig            `yaml:"provider" json:"provider"`
}

const DefaultProviderProfile = "default"

func Default() Config {
	base := filepath.Join(userHomeDir(), ".papersilm")
	provider := ProviderConfig{
		Provider: ProviderOpenAI,
		Model:    "",
		BaseURL:  "",
		APIKey:   "",
		Timeout:  "2m",
	}
	return Config{
		BaseDir:        base,
		DefaultLang:    "zh",
		DefaultStyle:   "distill",
		PermissionMode: protocol.PermissionModeConfirm,
		ActiveProvider: DefaultProviderProfile,
		Providers: map[string]ProviderConfig{
			DefaultProviderProfile: provider,
		},
		Provider: provider,
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
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = Default()
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.Normalize()
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
	d, err := time.ParseDuration(strings.TrimSpace(c.ActiveProviderConfig().Timeout))
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

func (c *Config) Normalize() {
	defaults := Default()
	if strings.TrimSpace(c.BaseDir) == "" {
		c.BaseDir = defaults.BaseDir
	}
	if strings.TrimSpace(c.DefaultLang) == "" {
		c.DefaultLang = defaults.DefaultLang
	}
	if strings.TrimSpace(c.DefaultStyle) == "" {
		c.DefaultStyle = defaults.DefaultStyle
	}
	if c.PermissionMode == "" {
		c.PermissionMode = defaults.PermissionMode
	}

	if len(c.Providers) == 0 {
		legacy := normalizeProviderConfig(c.Provider, defaults.Provider)
		c.Providers = map[string]ProviderConfig{
			DefaultProviderProfile: legacy,
		}
		if strings.TrimSpace(c.ActiveProvider) == "" {
			c.ActiveProvider = DefaultProviderProfile
		}
	} else {
		normalized := make(map[string]ProviderConfig, len(c.Providers))
		for name, provider := range c.Providers {
			key := strings.TrimSpace(name)
			if key == "" {
				continue
			}
			normalized[key] = normalizeProviderConfig(provider, defaults.Provider)
		}
		if len(normalized) == 0 {
			normalized[DefaultProviderProfile] = defaults.Provider
		}
		c.Providers = normalized
		if strings.TrimSpace(c.ActiveProvider) == "" {
			if _, ok := c.Providers[DefaultProviderProfile]; ok {
				c.ActiveProvider = DefaultProviderProfile
			} else {
				names := make([]string, 0, len(c.Providers))
				for name := range c.Providers {
					names = append(names, name)
				}
				sort.Strings(names)
				c.ActiveProvider = names[0]
			}
		}
	}

	if _, ok := c.Providers[c.ActiveProvider]; !ok {
		if _, ok := c.Providers[DefaultProviderProfile]; ok {
			c.ActiveProvider = DefaultProviderProfile
		} else {
			names := c.ProviderProfileNames()
			if len(names) == 0 {
				c.Providers = map[string]ProviderConfig{
					DefaultProviderProfile: defaults.Provider,
				}
				c.ActiveProvider = DefaultProviderProfile
			} else {
				c.ActiveProvider = names[0]
			}
		}
	}

	if provider, ok := c.Providers[c.ActiveProvider]; ok {
		c.Provider = provider
	} else {
		c.Provider = defaults.Provider
	}
}

func (c Config) ProviderProfileNames() []string {
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		if strings.TrimSpace(name) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c Config) ActiveProviderName() string {
	cfg := c
	cfg.Normalize()
	return cfg.ActiveProvider
}

func (c Config) ActiveProviderConfig() ProviderConfig {
	cfg := c
	cfg.Normalize()
	provider, ok := cfg.Providers[cfg.ActiveProvider]
	if !ok {
		return cfg.Provider
	}
	return provider
}

func (c *Config) SetActiveProvider(name string) error {
	c.Normalize()
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("provider profile is required")
	}
	provider, ok := c.Providers[name]
	if !ok {
		return fmt.Errorf("provider profile not found: %s", name)
	}
	c.ActiveProvider = name
	c.Provider = provider
	return nil
}

func (c *Config) SetActiveModel(model string) error {
	c.Normalize()
	profile := c.ActiveProvider
	provider, ok := c.Providers[profile]
	if !ok {
		return fmt.Errorf("provider profile not found: %s", profile)
	}
	provider.Model = strings.TrimSpace(model)
	c.Providers[profile] = provider
	c.Provider = provider
	return nil
}

func normalizeProviderConfig(provider ProviderConfig, defaults ProviderConfig) ProviderConfig {
	if provider.Provider == "" {
		provider.Provider = defaults.Provider
	}
	if strings.TrimSpace(provider.Timeout) == "" {
		provider.Timeout = defaults.Timeout
	}
	return provider
}
