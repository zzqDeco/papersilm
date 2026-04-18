package cli

import (
	"testing"

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
