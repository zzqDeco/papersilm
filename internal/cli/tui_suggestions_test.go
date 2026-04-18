package cli

import (
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestBuildInputSuggestionsUsesSessionContext(t *testing.T) {
	t.Parallel()

	snapshot := protocol.SessionSnapshot{
		Sources: []protocol.PaperRef{
			{PaperID: "paper_a"},
			{PaperID: "paper_b"},
		},
		TaskBoard: &protocol.TaskBoard{
			Tasks: []protocol.TaskCard{
				{TaskID: "task_1"},
				{TaskID: "task_2"},
			},
		},
		SkillRuns: []protocol.SkillRunRecord{
			{RunID: "run_1"},
		},
	}

	assertHasInsert := func(input, want string) {
		t.Helper()
		suggestions := buildInputSuggestions(input, snapshot, nil)
		for _, suggestion := range suggestions {
			if suggestion.Insert == want {
				return
			}
		}
		t.Fatalf("expected suggestion %q for input %q, got %+v", want, input, suggestions)
	}

	assertHasInsert("/source remove pa", "/source remove paper_a")
	assertHasInsert("/task show task_", "/task show task_1")
	assertHasInsert("/workspace show paper_", "/workspace show paper_a")
	assertHasInsert("/skill show run_", "/skill show run_1")
}

func TestBuildInputSuggestionsIncludesHistoryAndRecipes(t *testing.T) {
	t.Parallel()

	history := []string{
		"总结这篇论文的局限性",
		"比较这些论文的实验设置",
	}
	suggestions := buildInputSuggestions("比较", protocol.SessionSnapshot{}, history)
	if len(suggestions) == 0 {
		t.Fatalf("expected prompt suggestions")
	}

	haveHistory := false
	haveRecipe := false
	for _, suggestion := range suggestions {
		if strings.Contains(suggestion.Insert, "实验设置") {
			haveHistory = true
		}
		if strings.Contains(suggestion.Insert, "方法差异") {
			haveRecipe = true
		}
	}
	if !haveHistory {
		t.Fatalf("expected history suggestion in %+v", suggestions)
	}
	if !haveRecipe {
		t.Fatalf("expected recipe suggestion in %+v", suggestions)
	}
}
