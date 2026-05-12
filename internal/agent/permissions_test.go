package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPermissionRuleMatchesScopedRequests(t *testing.T) {
	t.Parallel()

	edit := protocol.PermissionRequest{
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		TargetPath: "docs/readme.md",
	}
	if !permissionAllowedByRule(edit, protocol.PermissionRule{
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		Scope:      permissionScopePath,
		TargetPath: "docs/readme.md",
	}) {
		t.Fatalf("expected exact path edit rule to match")
	}
	if permissionAllowedByRule(edit, protocol.PermissionRule{
		Tool:       string(protocol.NodeKindWorkspaceEdit),
		Operation:  "write",
		Scope:      permissionScopePath,
		TargetPath: "docs/other.md",
	}) {
		t.Fatalf("did not expect different path rule to match")
	}

	command := protocol.PermissionRequest{
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Command:   "go test ./...",
	}
	if !permissionAllowedByRule(command, protocol.PermissionRule{
		Tool:          string(protocol.NodeKindWorkspaceCommand),
		Operation:     "shell",
		Scope:         permissionScopeCommandPrefix,
		CommandPrefix: "go test",
	}) {
		t.Fatalf("expected command prefix rule to match")
	}
	if permissionAllowedByRule(protocol.PermissionRequest{
		Tool:      string(protocol.NodeKindWorkspaceCommand),
		Operation: "shell",
		Command:   "go testify ./...",
	}, protocol.PermissionRule{
		Tool:          string(protocol.NodeKindWorkspaceCommand),
		Operation:     "shell",
		Scope:         permissionScopeCommandPrefix,
		CommandPrefix: "go test",
	}) {
		t.Fatalf("did not expect byte-prefix command overlap to match")
	}
}

func TestFindWorkspaceEditPreviewRequestRequiresNodeID(t *testing.T) {
	t.Parallel()

	approval := &protocol.ApprovalRequest{
		Requests: []protocol.PermissionRequest{
			{NodeID: "edit_a", Tool: string(protocol.NodeKindWorkspaceEdit), TargetPath: "README.md", Preview: protocol.PermissionPreview{Kind: "diff", NewContent: "A"}},
			{NodeID: "edit_b", Tool: string(protocol.NodeKindWorkspaceEdit), TargetPath: "README.md", Preview: protocol.PermissionPreview{Kind: "diff", NewContent: "B"}},
		},
	}
	request, ok := findWorkspaceEditPreviewRequest(approval, "edit_b", "README.md")
	if !ok {
		t.Fatalf("expected matching request")
	}
	if request.Preview.NewContent != "B" {
		t.Fatalf("expected request bound to approved node, got %q", request.Preview.NewContent)
	}
	if _, ok := findWorkspaceEditPreviewRequest(approval, "", "README.md"); ok {
		t.Fatalf("did not expect empty node id to match by path alone")
	}
}

func TestExecuteWorkspaceEditRequiresApprovedPreview(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.BaseDir = t.TempDir()
	store := storage.New(cfg.BaseDir)
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := os.WriteFile(storePath(t, store, "README.md"), []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := store.RefreshWorkspaceState(); err != nil {
		t.Fatalf("RefreshWorkspaceState: %v", err)
	}
	sessionID := "sess_preview_guard"
	now := time.Now().UTC()
	if err := store.CreateSession(protocol.SessionMeta{
		SessionID:       sessionID,
		State:           protocol.SessionStateAwaitingApproval,
		ApprovalPending: true,
		PermissionMode:  protocol.PermissionModeConfirm,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.SavePendingApproval(sessionID, protocol.ApprovalRequest{
		Requests: []protocol.PermissionRequest{
			{
				RequestID:  "approval_other",
				NodeID:     "edit_other",
				Tool:       string(protocol.NodeKindWorkspaceEdit),
				Operation:  "write",
				TargetPath: "README.md",
				Preview:    protocol.PermissionPreview{Kind: "diff", OldContentHash: contentHash("original\n"), NewContent: "approved\n"},
			},
		},
	}); err != nil {
		t.Fatalf("SavePendingApproval: %v", err)
	}
	agent := New(tools.New(pipeline.New(cfg)), cfg)
	_, err := agent.executeWorkspaceEdit(context.Background(), store, sessionID, "edit_target", "update README.md", workspaceIntent{targetPath: "README.md"})
	if err == nil || !strings.Contains(err.Error(), "approved workspace edit preview not found") {
		t.Fatalf("expected missing preview error, got %v", err)
	}
	content, readErr := store.ReadWorkspaceFile("README.md")
	if readErr != nil {
		t.Fatalf("ReadWorkspaceFile: %v", readErr)
	}
	if content != "original\n" {
		t.Fatalf("expected file to remain unchanged, got %q", content)
	}
}

func storePath(t *testing.T, store *storage.Store, path string) string {
	t.Helper()
	resolved, err := store.ResolveWorkspacePath(path)
	if err != nil {
		t.Fatalf("ResolveWorkspacePath: %v", err)
	}
	return resolved
}

func TestCompactUnifiedDiffShowsChangedLines(t *testing.T) {
	t.Parallel()

	diff := compactUnifiedDiff("README.md", "one\ntwo\nthree\n", "one\n2\nthree\n")
	for _, want := range []string{"--- README.md", "+++ README.md", "-two", "+2"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("expected %q in diff:\n%s", want, diff)
		}
	}
}

func TestInferWorkspaceIntentDoesNotDefaultEditsToReadme(t *testing.T) {
	t.Parallel()

	files := []protocol.WorkspaceFile{{Path: "README.md"}}
	intent := inferWorkspaceIntent("create `notes.md` with a todo list", files)
	if intent.kind != protocol.NodeKindWorkspaceEdit {
		t.Fatalf("expected edit intent, got %s", intent.kind)
	}
	if intent.targetPath != "notes.md" {
		t.Fatalf("expected explicit new file target, got %q", intent.targetPath)
	}

	intent = inferWorkspaceIntent("update the project notes", files)
	if intent.kind != protocol.NodeKindWorkspaceEdit {
		t.Fatalf("expected edit intent, got %s", intent.kind)
	}
	if intent.targetPath != "" {
		t.Fatalf("expected ambiguous edit target to stay empty, got %q", intent.targetPath)
	}
}
