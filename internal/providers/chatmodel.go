package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	agenticopenaimodel "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	ollamamodel "github.com/cloudwego/eino-ext/components/model/ollama"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	qwenmodel "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"

	"github.com/zzqDeco/papersilm/internal/config"
)

func BuildChatModel(ctx context.Context, cfg config.ProviderConfig, timeout time.Duration) (model.ToolCallingChatModel, error) {
	if useLocalAgentModel(cfg) {
		return NewLocalToolCallingChatModel(), nil
	}
	switch cfg.Provider {
	case config.ProviderOpenAI:
		return openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
			Timeout: timeout,
		})
	case config.ProviderArk:
		return arkmodel.NewChatModel(ctx, &arkmodel.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
			Timeout: &timeout,
		})
	case config.ProviderQwen:
		return qwenmodel.NewChatModel(ctx, &qwenmodel.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
			Timeout: timeout,
		})
	case config.ProviderDeepSeek:
		return deepseekmodel.NewChatModel(ctx, &deepseekmodel.ChatModelConfig{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
			Timeout: timeout,
		})
	case config.ProviderOllama:
		return ollamamodel.NewChatModel(ctx, &ollamamodel.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			Timeout: timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func BuildAgenticModel(ctx context.Context, cfg config.ProviderConfig, timeout time.Duration) (model.AgenticModel, error) {
	if useLocalAgentModel(cfg) {
		return nil, fmt.Errorf("agentic runtime requires a configured provider")
	}
	switch cfg.Provider {
	case config.ProviderOpenAI:
		var timeoutPtr *time.Duration
		if timeout > 0 {
			timeoutPtr = &timeout
		}
		return agenticopenaimodel.New(ctx, &agenticopenaimodel.Config{
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			BaseURL: cfg.BaseURL,
			Timeout: timeoutPtr,
		})
	default:
		return nil, fmt.Errorf("agentic runtime does not support provider %q yet", cfg.Provider)
	}
}

func useLocalAgentModel(cfg config.ProviderConfig) bool {
	if cfg.Model == "" {
		return true
	}
	switch cfg.Provider {
	case config.ProviderOllama:
		return strings.TrimSpace(cfg.BaseURL) == ""
	default:
		return strings.TrimSpace(cfg.APIKey) == ""
	}
}
