package providers

import (
	"context"
	"strings"
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

func TestBuildAgenticModelRequiresConfiguredProvider(t *testing.T) {
	t.Parallel()

	_, err := BuildAgenticModel(context.Background(), config.ProviderConfig{
		Provider: config.ProviderOpenAI,
		Model:    "gpt-5.4",
	}, 2*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "configured provider") {
		t.Fatalf("expected configured provider error, got %v", err)
	}
}

func TestBuildAgenticModelRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := BuildAgenticModel(context.Background(), config.ProviderConfig{
		Provider: config.ProviderOllama,
		Model:    "qwen2.5:7b",
		BaseURL:  "http://127.0.0.1:11434",
	}, 2*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "does not support provider") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}
