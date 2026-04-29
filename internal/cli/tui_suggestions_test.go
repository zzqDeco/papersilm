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
	assertHasInsert("/paper show paper_", "/paper show paper_a")
	assertHasInsert("/skill show run_", "/skill show run_1")
}

func TestBuildPaletteSuggestionsIncludesHistoryAndRecipes(t *testing.T) {
	t.Parallel()

	history := []string{
		"总结这篇论文的局限性",
		"比较这些论文的实验设置",
	}
	suggestions := buildPaletteSuggestions("比较", protocol.SessionSnapshot{}, history)
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

func TestBuildInputSuggestionsDoesNotInterruptPlainPromptTyping(t *testing.T) {
	t.Parallel()

	suggestions := buildInputSuggestions("比较", protocol.SessionSnapshot{}, []string{"比较这些论文的实验设置"})
	if len(suggestions) != 0 {
		t.Fatalf("expected plain prompt typing to avoid dropdown noise, got %+v", suggestions)
	}
}

func TestBuildInputSuggestionsIncludesThemeCommandValues(t *testing.T) {
	t.Parallel()

	suggestions := buildInputSuggestions("/theme da", protocol.SessionSnapshot{}, nil)
	for _, suggestion := range suggestions {
		if suggestion.Insert == "/theme dark" {
			return
		}
	}
	t.Fatalf("expected /theme dark suggestion, got %+v", suggestions)
}

func TestBuildInputSuggestionsIncludesTranscriptCommand(t *testing.T) {
	t.Parallel()

	suggestions := buildInputSuggestions("/trans", protocol.SessionSnapshot{}, nil)
	for _, suggestion := range suggestions {
		if suggestion.Insert == "/transcript" {
			return
		}
	}
	t.Fatalf("expected /transcript suggestion, got %+v", suggestions)
}

func TestBuildInputSuggestionsIncludesHintsCommand(t *testing.T) {
	t.Parallel()

	suggestions := buildInputSuggestions("/hint", protocol.SessionSnapshot{}, nil)
	for _, suggestion := range suggestions {
		if suggestion.Insert == "/hints" {
			return
		}
	}
	t.Fatalf("expected /hints suggestion, got %+v", suggestions)
}
