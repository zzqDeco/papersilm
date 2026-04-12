package cli

import (
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestSplitWorkspaceCommandPreservesBody(t *testing.T) {
	t.Parallel()

	head, body := splitWorkspaceCommand("/workspace note add paper_1 :: this is a workspace body with spaces")
	if head != "/workspace note add paper_1" {
		t.Fatalf("unexpected head: %q", head)
	}
	if body != "this is a workspace body with spaces" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestParseWorkspaceAnchor(t *testing.T) {
	t.Parallel()

	pageAnchor, err := parseWorkspaceAnchor("page", []string{"3"})
	if err != nil {
		t.Fatalf("parse page anchor: %v", err)
	}
	if pageAnchor.Kind != protocol.AnchorKindPage || pageAnchor.Page != 3 {
		t.Fatalf("unexpected page anchor: %+v", pageAnchor)
	}

	snippetAnchor, err := parseWorkspaceAnchor("snippet", []string{"Attention", "improves", "accuracy"})
	if err != nil {
		t.Fatalf("parse snippet anchor: %v", err)
	}
	if snippetAnchor.Kind != protocol.AnchorKindSnippet || snippetAnchor.Snippet != "Attention improves accuracy" {
		t.Fatalf("unexpected snippet anchor: %+v", snippetAnchor)
	}

	sectionAnchor, err := parseWorkspaceAnchor("section", []string{"Experimental", "Setup"})
	if err != nil {
		t.Fatalf("parse section anchor: %v", err)
	}
	if sectionAnchor.Kind != protocol.AnchorKindSection || sectionAnchor.Section != "Experimental Setup" {
		t.Fatalf("unexpected section anchor: %+v", sectionAnchor)
	}

	if _, err := parseWorkspaceAnchor("page", []string{"0"}); err == nil {
		t.Fatalf("expected invalid page anchor error")
	}
}
