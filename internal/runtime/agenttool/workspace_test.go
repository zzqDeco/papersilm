package agenttool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPersistAcceptSessionRulePropagatesStoreErrors(t *testing.T) {
	t.Parallel()

	baseFile := filepath.Join(t.TempDir(), "store-file")
	if err := os.WriteFile(baseFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := WorkspaceToolsConfig{Store: storage.New(baseFile), SessionID: "sess_test"}
	request := protocol.PermissionRequest{
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		TargetPath: "README.md",
	}
	decision := protocol.PermissionDecision{Value: PermissionAcceptSession, Scope: PermissionScopePath}

	err := persistAcceptSessionRule(cfg, request, decision)
	if err == nil {
		t.Fatalf("expected permission rule persistence failure")
	}
	if !strings.Contains(err.Error(), "save session permission rule") {
		t.Fatalf("expected wrapped persistence error, got %v", err)
	}
}

func TestPersistAcceptSessionRuleIgnoresNonSessionDecision(t *testing.T) {
	t.Parallel()

	cfg := WorkspaceToolsConfig{Store: storage.New(filepath.Join(t.TempDir(), "store-file")), SessionID: "sess_test"}
	decision := protocol.PermissionDecision{Value: PermissionAcceptOnce}
	if err := persistAcceptSessionRule(cfg, protocol.PermissionRequest{}, decision); err != nil {
		t.Fatalf("accept-once decision should not persist a session rule: %v", err)
	}
}
