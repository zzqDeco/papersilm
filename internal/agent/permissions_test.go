package agent

import (
	"strings"
	"testing"

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

func TestCompactUnifiedDiffShowsChangedLines(t *testing.T) {
	t.Parallel()

	diff := compactUnifiedDiff("README.md", "one\ntwo\nthree\n", "one\n2\nthree\n")
	for _, want := range []string{"--- README.md", "+++ README.md", "-two", "+2"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("expected %q in diff:\n%s", want, diff)
		}
	}
}
