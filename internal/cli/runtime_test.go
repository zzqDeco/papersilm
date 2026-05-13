package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestShouldUseTUIWithTTY(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		outputFormat protocol.OutputFormat
		printTask    string
		termValue    string
		stdinTTY     bool
		stdoutTTY    bool
		want         bool
	}{
		{
			name:         "interactive text mode uses tui",
			outputFormat: protocol.OutputFormatText,
			termValue:    "xterm-256color",
			stdinTTY:     true,
			stdoutTTY:    true,
			want:         true,
		},
		{
			name:         "empty term still uses tui",
			outputFormat: protocol.OutputFormatText,
			termValue:    "",
			stdinTTY:     true,
			stdoutTTY:    true,
			want:         true,
		},
		{
			name:         "print mode skips tui",
			outputFormat: protocol.OutputFormatText,
			printTask:    "summarize",
			termValue:    "xterm-256color",
			stdinTTY:     true,
			stdoutTTY:    true,
			want:         false,
		},
		{
			name:         "json output skips tui",
			outputFormat: protocol.OutputFormatJSON,
			termValue:    "xterm-256color",
			stdinTTY:     true,
			stdoutTTY:    true,
			want:         false,
		},
		{
			name:         "dumb term skips tui",
			outputFormat: protocol.OutputFormatText,
			termValue:    "dumb",
			stdinTTY:     true,
			stdoutTTY:    true,
			want:         false,
		},
		{
			name:         "non tty skips tui",
			outputFormat: protocol.OutputFormatText,
			termValue:    "xterm-256color",
			stdinTTY:     true,
			stdoutTTY:    false,
			want:         false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldUseTUIWithTTY(tc.outputFormat, tc.printTask, tc.termValue, tc.stdinTTY, tc.stdoutTTY)
			if got != tc.want {
				t.Fatalf("shouldUseTUIWithTTY() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProviderDiscoveryTimeoutCapsSlowRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := discoverOpenAICompatibleModels(ctx, config.ProviderConfig{
		Provider: config.ProviderOpenAI,
		BaseURL:  server.URL,
		APIKey:   "test",
	})
	if err == nil {
		t.Fatalf("expected context timeout")
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("expected discovery to stop on context timeout, took %s", elapsed)
	}
}

func TestProviderDiscoveryTimeoutUsesShortBound(t *testing.T) {
	t.Parallel()

	if got := providerDiscoveryTimeout(config.ProviderConfig{Timeout: "100ms"}); got != 100*time.Millisecond {
		t.Fatalf("expected explicit short timeout, got %s", got)
	}
	if got := providerDiscoveryTimeout(config.ProviderConfig{Timeout: "2m"}); got != 30*time.Second {
		t.Fatalf("expected timeout cap, got %s", got)
	}
	if got := providerDiscoveryTimeout(config.ProviderConfig{Timeout: "bad"}); got != 15*time.Second {
		t.Fatalf("expected default timeout, got %s", got)
	}
}
