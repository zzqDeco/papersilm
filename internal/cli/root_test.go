package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	buildversion "github.com/zzqDeco/papersilm/internal/version"
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
