package providers

import (
	"context"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
)

func TestBuildAgenticModelOpenAICompatible(t *testing.T) {
	t.Parallel()

	model, err := BuildAgenticModel(context.Background(), config.ProviderConfig{
		Provider: config.ProviderOpenAI,
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "local-test",
	}, 2*time.Minute)
	if err != nil {
		t.Fatalf("BuildAgenticModel(openai): %v", err)
	}
	if model == nil {
		t.Fatalf("expected agentic model")
	}
}

func TestBuildAgenticModelUsesLocalFallbackWhenUnconfigured(t *testing.T) {
	t.Parallel()

	model, err := BuildAgenticModel(context.Background(), config.ProviderConfig{
		Provider: config.ProviderOpenAI,
		Model:    "gpt-5.4",
	}, 2*time.Minute)
	if err != nil {
		t.Fatalf("BuildAgenticModel(local fallback): %v", err)
	}
	if model == nil {
		t.Fatalf("expected local agentic model")
	}
}

func TestBuildAgenticModelAdaptsNonOpenAIProvider(t *testing.T) {
	t.Parallel()

	model, err := BuildAgenticModel(context.Background(), config.ProviderConfig{
		Provider: config.ProviderOllama,
		Model:    "qwen2.5:7b",
		BaseURL:  "http://127.0.0.1:11434",
	}, 2*time.Minute)
	if err != nil {
		t.Fatalf("BuildAgenticModel(ollama adapter): %v", err)
	}
	if model == nil {
		t.Fatalf("expected adapted agentic model")
	}
}
