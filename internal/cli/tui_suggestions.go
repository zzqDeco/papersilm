package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type tuiSuggestion struct {
	Label    string
	Insert   string
	Detail   string
	Category string
	Disabled bool
}

func buildInputSuggestions(input string, snapshot protocol.SessionSnapshot, history []string) []tuiSuggestion {
	if strings.HasPrefix(strings.TrimSpace(input), "/") {
		return buildSlashSuggestions(input, snapshot)
	}
	return nil
}

func buildPaletteSuggestions(query string, snapshot protocol.SessionSnapshot, history []string) []tuiSuggestion {
	suggestions := append([]tuiSuggestion{}, commandCatalogSuggestions()...)
	suggestions = append(suggestions, buildPromptSuggestions("", history)...)
	suggestions = append(suggestions, buildContextSuggestions(snapshot)...)
	return limitSuggestions(filterSuggestions(suggestions, query), 12)
}

func buildSlashSuggestions(input string, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed == "/" {
		return limitSuggestions(commandCatalogSuggestions(), 8)
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	hasTrailingSpace := strings.HasSuffix(input, " ")
	if len(fields) == 1 && !hasTrailingSpace {
		return limitSuggestions(filterSlashCommands(strings.TrimPrefix(fields[0], "/")), 8)
	}

	switch fields[0] {
	case "/source":
		return limitSuggestions(buildSourceSlashSuggestions(fields, hasTrailingSpace, snapshot), 8)
	case "/task":
		return limitSuggestions(buildTaskSlashSuggestions(fields, hasTrailingSpace, snapshot), 8)
	case "/workspace":
		return limitSuggestions(buildWorkspaceSlashSuggestions(fields, hasTrailingSpace, snapshot), 8)
	case "/paper":
		return limitSuggestions(buildPaperSlashSuggestions(fields, hasTrailingSpace, snapshot), 8)
	case "/skill":
		return limitSuggestions(buildSkillSlashSuggestions(fields, hasTrailingSpace, snapshot), 8)
	case "/lang":
		return limitSuggestions(buildLiteralSuggestions("/lang ", lastToken(fields, hasTrailingSpace), "Command", []string{"zh", "en", "both"}), 8)
	case "/style":
		return limitSuggestions(buildLiteralSuggestions("/style ", lastToken(fields, hasTrailingSpace), "Command", []string{"distill", "ultra", "reviewer"}), 8)
	case "/theme":
		return limitSuggestions(buildLiteralSuggestions("/theme ", lastToken(fields, hasTrailingSpace), "Command", []string{"auto", "dark", "light"}), 8)
	case "/hints":
		return limitSuggestions(buildLiteralSuggestions("/hints ", lastToken(fields, hasTrailingSpace), "Command", []string{"on", "off", "toggle"}), 8)
	case "/session":
		if len(fields) <= 2 {
			return limitSuggestions(filterSuggestions([]tuiSuggestion{
				{Label: "name", Insert: "/session name ", Detail: "Rename the current session", Category: "Command"},
			}, lastToken(fields, hasTrailingSpace)), 8)
		}
	}

	return limitSuggestions(filterSuggestions(commandCatalogSuggestions(), strings.TrimPrefix(trimmed, "/")), 8)
}

func buildPromptSuggestions(input string, history []string) []tuiSuggestion {
	query := strings.TrimSpace(strings.ToLower(input))
	suggestions := []tuiSuggestion{
		{
			Label:    "总结当前工作区的结构和关键文件",
			Insert:   "总结当前工作区的结构、关键文件和下一步建议。",
			Detail:   "Workspace overview recipe",
			Category: "Recipe",
		},
		{
			Label:    "搜索并解释某个配置或实现",
			Insert:   "在当前工作区里搜索相关实现，解释它的作用和依赖关系。",
			Detail:   "Workspace search recipe",
			Category: "Recipe",
		},
		{
			Label:    "总结论文核心贡献、方法和实验结果",
			Insert:   "总结这篇论文的核心贡献、方法和实验结果，并指出局限性。",
			Detail:   "Single-paper digest recipe",
			Category: "Recipe",
		},
		{
			Label:    "比较多篇论文的方法差异",
			Insert:   "比较这些论文的方法差异、实验设置和最终结论，整理成对比表。",
			Detail:   "Comparison recipe",
			Category: "Recipe",
		},
		{
			Label:    "解释关键公式或定理",
			Insert:   "解释这篇论文里最关键的公式或定理，给出直观推导和变量含义。",
			Detail:   "Equation explanation recipe",
			Category: "Recipe",
		},
		{
			Label:    "整理 reviewer 视角问题",
			Insert:   "从 reviewer 视角总结这篇论文的贡献、证据强弱、潜在漏洞和需要补实验的地方。",
			Detail:   "Review recipe",
			Category: "Recipe",
		},
	}

	seen := make(map[string]struct{}, len(suggestions))
	for _, item := range suggestions {
		seen[item.Insert] = struct{}{}
	}
	for i := len(history) - 1; i >= 0; i-- {
		value := strings.TrimSpace(history[i])
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		suggestions = append(suggestions, tuiSuggestion{
			Label:    value,
			Insert:   value,
			Detail:   "Recent history",
			Category: "History",
		})
	}
	return limitSuggestions(filterSuggestions(suggestions, query), 8)
}

func buildContextSuggestions(snapshot protocol.SessionSnapshot) []tuiSuggestion {
	suggestions := make([]tuiSuggestion, 0, len(snapshot.Sources)+len(snapshot.SkillRuns)+len(snapshot.Workspaces))
	for _, source := range snapshot.Sources {
		suggestions = append(suggestions, tuiSuggestion{
			Label:    source.PaperID,
			Insert:   fmt.Sprintf("/paper show %s", source.PaperID),
			Detail:   "Open paper workspace",
			Category: "Context",
		})
	}
	for _, task := range taskIDs(snapshot.TaskBoard) {
		suggestions = append(suggestions, tuiSuggestion{
			Label:    task,
			Insert:   fmt.Sprintf("/task show %s", task),
			Detail:   "Inspect task",
			Category: "Context",
		})
	}
	for _, run := range snapshot.SkillRuns {
		suggestions = append(suggestions, tuiSuggestion{
			Label:    run.RunID,
			Insert:   fmt.Sprintf("/skill show %s", run.RunID),
			Detail:   "Inspect skill run",
			Category: "Context",
		})
	}
	return dedupeSuggestions(suggestions)
}

func buildSourceSlashSuggestions(fields []string, hasTrailingSpace bool, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	if len(fields) == 1 || (len(fields) == 2 && !hasTrailingSpace) {
		return filterSuggestions([]tuiSuggestion{
			{Label: "add", Insert: "/source add ", Detail: "Attach a source to the current session", Category: "Command"},
			{Label: "replace", Insert: "/source replace ", Detail: "Replace existing sources", Category: "Command"},
			{Label: "list", Insert: "/source list", Detail: "List attached sources", Category: "Command"},
			{Label: "remove", Insert: "/source remove ", Detail: "Remove a source by paper_id", Category: "Command"},
		}, lastToken(fields, hasTrailingSpace))
	}
	if len(fields) >= 2 && fields[1] == "remove" {
		return buildPaperIDSuggestions("/source remove ", snapshot.Sources, lastToken(fields, hasTrailingSpace))
	}
	return nil
}

func buildTaskSlashSuggestions(fields []string, hasTrailingSpace bool, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	if len(fields) == 1 || (len(fields) == 2 && !hasTrailingSpace) {
		return filterSuggestions([]tuiSuggestion{
			{Label: "show", Insert: "/task show ", Detail: "Inspect one task", Category: "Command"},
			{Label: "run", Insert: "/task run ", Detail: "Run one task", Category: "Command"},
			{Label: "approve", Insert: "/task approve ", Detail: "Approve one task", Category: "Command"},
			{Label: "reject", Insert: "/task reject ", Detail: "Reject one task", Category: "Command"},
		}, lastToken(fields, hasTrailingSpace))
	}
	if len(fields) < 3 {
		return nil
	}
	prefix := fmt.Sprintf("/task %s ", fields[1])
	return buildLiteralSuggestions(prefix, lastToken(fields, hasTrailingSpace), "Task", taskIDs(snapshot.TaskBoard))
}

func buildWorkspaceSlashSuggestions(fields []string, hasTrailingSpace bool, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	if len(fields) == 1 || (len(fields) == 2 && !hasTrailingSpace) {
		return filterSuggestions([]tuiSuggestion{
			{Label: "show", Insert: "/workspace show", Detail: "Show the current root workspace", Category: "Command"},
			{Label: "files", Insert: "/workspace files ", Detail: "List indexed workspace files", Category: "Command"},
			{Label: "search", Insert: "/workspace search ", Detail: "Search workspace text files", Category: "Command"},
			{Label: "sessions", Insert: "/workspace sessions", Detail: "List sessions in this workspace", Category: "Command"},
		}, lastToken(fields, hasTrailingSpace))
	}
	switch {
	case len(fields) >= 2 && fields[1] == "files":
		return nil
	case len(fields) >= 2 && fields[1] == "search":
		return nil
	}
	return nil
}

func buildPaperSlashSuggestions(fields []string, hasTrailingSpace bool, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	if len(fields) == 1 || (len(fields) == 2 && !hasTrailingSpace) {
		return filterSuggestions([]tuiSuggestion{
			{Label: "list", Insert: "/paper list", Detail: "List attached paper workspaces", Category: "Command"},
			{Label: "show", Insert: "/paper show ", Detail: "Open one paper workspace", Category: "Command"},
			{Label: "note add", Insert: "/paper note add ", Detail: "Add a paper note", Category: "Command"},
			{Label: "annotation add", Insert: "/paper annotation add ", Detail: "Add an anchored annotation", Category: "Command"},
		}, lastToken(fields, hasTrailingSpace))
	}
	switch {
	case len(fields) >= 2 && fields[1] == "show":
		return buildPaperIDSuggestions("/paper show ", snapshot.Sources, lastToken(fields, hasTrailingSpace))
	case len(fields) >= 3 && fields[1] == "note" && fields[2] == "add":
		return buildPaperIDSuggestions("/paper note add ", snapshot.Sources, lastToken(fields, hasTrailingSpace), " :: ")
	case len(fields) >= 3 && fields[1] == "annotation" && fields[2] == "add":
		if len(fields) <= 3 || (len(fields) == 4 && !hasTrailingSpace) {
			return buildPaperIDSuggestions("/paper annotation add ", snapshot.Sources, lastToken(fields, hasTrailingSpace))
		}
		if len(fields) == 4 || (len(fields) == 5 && !hasTrailingSpace) {
			prefix := fmt.Sprintf("/paper annotation add %s ", fields[3])
			return filterSuggestions([]tuiSuggestion{
				{Label: "page", Insert: prefix + "page ", Detail: "Anchor to a page number", Category: "Command"},
				{Label: "snippet", Insert: prefix + "snippet ", Detail: "Anchor to a text snippet", Category: "Command"},
				{Label: "section", Insert: prefix + "section ", Detail: "Anchor to a section title", Category: "Command"},
			}, lastToken(fields, hasTrailingSpace))
		}
	}
	return nil
}

func buildSkillSlashSuggestions(fields []string, hasTrailingSpace bool, snapshot protocol.SessionSnapshot) []tuiSuggestion {
	if len(fields) == 1 || (len(fields) == 2 && !hasTrailingSpace) {
		return filterSuggestions([]tuiSuggestion{
			{Label: "list", Insert: "/skill list", Detail: "List available skills", Category: "Command"},
			{Label: "run", Insert: "/skill run ", Detail: "Run a skill", Category: "Command"},
			{Label: "show", Insert: "/skill show ", Detail: "Inspect a skill run", Category: "Command"},
		}, lastToken(fields, hasTrailingSpace))
	}
	switch {
	case len(fields) >= 2 && fields[1] == "run":
		return buildLiteralSuggestions("/skill run ", lastToken(fields, hasTrailingSpace), "Skill", []string{
			string(protocol.SkillNameReviewer),
			string(protocol.SkillNameEquationExplain),
			string(protocol.SkillNameRelatedWorkMap),
			string(protocol.SkillNameCompareRefinement),
		})
	case len(fields) >= 2 && fields[1] == "show":
		runIDs := make([]string, 0, len(snapshot.SkillRuns))
		for _, run := range snapshot.SkillRuns {
			runIDs = append(runIDs, run.RunID)
		}
		return buildLiteralSuggestions("/skill show ", lastToken(fields, hasTrailingSpace), "Skill", runIDs)
	default:
		return nil
	}
}

func buildPaperIDSuggestions(prefix string, sources []protocol.PaperRef, query string, suffix ...string) []tuiSuggestion {
	paperIDs := make([]string, 0, len(sources))
	for _, source := range sources {
		paperIDs = append(paperIDs, source.PaperID)
	}
	return buildLiteralSuggestions(prefix, query, "Source", paperIDs, suffix...)
}

func buildLiteralSuggestions(prefix, query, category string, values []string, suffix ...string) []tuiSuggestion {
	trimmedQuery := strings.TrimSpace(strings.ToLower(query))
	trailer := ""
	if len(suffix) > 0 {
		trailer = suffix[0]
	}
	suggestions := make([]tuiSuggestion, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if trimmedQuery != "" && !strings.Contains(strings.ToLower(value), trimmedQuery) {
			continue
		}
		suggestions = append(suggestions, tuiSuggestion{
			Label:    value,
			Insert:   prefix + value + trailer,
			Detail:   category,
			Category: category,
		})
	}
	return dedupeSuggestions(suggestions)
}

func commandCatalogSuggestions() []tuiSuggestion {
	return []tuiSuggestion{
		{Label: "/help", Insert: "/help", Detail: "Show slash commands", Category: "Command"},
		{Label: "/commands", Insert: "/commands", Detail: "Open the command palette", Category: "Command"},
		{Label: "/transcript", Insert: "/transcript", Detail: "Open transcript view", Category: "Command"},
		{Label: "/model", Insert: "/model", Detail: "Open provider/model picker", Category: "Command"},
		{Label: "/source add", Insert: "/source add ", Detail: "Attach a source", Category: "Command"},
		{Label: "/source replace", Insert: "/source replace ", Detail: "Replace current sources", Category: "Command"},
		{Label: "/source list", Insert: "/source list", Detail: "List attached sources", Category: "Command"},
		{Label: "/source remove", Insert: "/source remove ", Detail: "Remove one source", Category: "Command"},
		{Label: "/plan", Insert: "/plan ", Detail: "Generate a plan only", Category: "Command"},
		{Label: "/run", Insert: "/run", Detail: "Run the active plan", Category: "Command"},
		{Label: "/approve", Insert: "/approve", Detail: "Approve the current checkpoint", Category: "Command"},
		{Label: "/tasks", Insert: "/tasks", Detail: "Open task board pane", Category: "Command"},
		{Label: "/task show", Insert: "/task show ", Detail: "Inspect a task", Category: "Command"},
		{Label: "/task run", Insert: "/task run ", Detail: "Run one task", Category: "Command"},
		{Label: "/task approve", Insert: "/task approve ", Detail: "Approve one task", Category: "Command"},
		{Label: "/task reject", Insert: "/task reject ", Detail: "Reject one task", Category: "Command"},
		{Label: "/skill list", Insert: "/skill list", Detail: "Open skill list pane", Category: "Command"},
		{Label: "/skill run", Insert: "/skill run ", Detail: "Run a skill", Category: "Command"},
		{Label: "/skill show", Insert: "/skill show ", Detail: "Inspect a skill run", Category: "Command"},
		{Label: "/workspace show", Insert: "/workspace show", Detail: "Open root workspace pane", Category: "Command"},
		{Label: "/workspace files", Insert: "/workspace files ", Detail: "List indexed workspace files", Category: "Command"},
		{Label: "/workspace search", Insert: "/workspace search ", Detail: "Search workspace text files", Category: "Command"},
		{Label: "/workspace sessions", Insert: "/workspace sessions", Detail: "List workspace sessions", Category: "Command"},
		{Label: "/paper list", Insert: "/paper list", Detail: "Open paper workspace list", Category: "Command"},
		{Label: "/paper show", Insert: "/paper show ", Detail: "Open a paper workspace", Category: "Command"},
		{Label: "/paper note add", Insert: "/paper note add ", Detail: "Add a paper note", Category: "Command"},
		{Label: "/paper annotation add", Insert: "/paper annotation add ", Detail: "Add a paper annotation", Category: "Command"},
		{Label: "/open", Insert: "/open ", Detail: "Read a workspace file", Category: "Command"},
		{Label: "/write", Insert: "/write ", Detail: "Write a workspace file", Category: "Command"},
		{Label: "/shell", Insert: "/shell ", Detail: "Run a workspace shell command", Category: "Command"},
		{Label: "/lang", Insert: "/lang ", Detail: "Switch output language", Category: "Command"},
		{Label: "/style", Insert: "/style ", Detail: "Switch output style", Category: "Command"},
		{Label: "/theme", Insert: "/theme ", Detail: "Switch TUI theme", Category: "Command"},
		{Label: "/hints", Insert: "/hints", Detail: "Toggle footer shortcut hints", Category: "Command"},
		{Label: "/session name", Insert: "/session name ", Detail: "Rename this session", Category: "Command"},
		{Label: "/export", Insert: "/export", Detail: "Open artifact export pane", Category: "Command"},
		{Label: "/clear", Insert: "/clear", Detail: "Add a visual separator", Category: "Command"},
		{Label: "/exit", Insert: "/exit", Detail: "Quit the session", Category: "Command"},
	}
}

func filterSlashCommands(query string) []tuiSuggestion {
	query = strings.TrimSpace(strings.ToLower(query))
	suggestions := make([]tuiSuggestion, 0, len(commandCatalogSuggestions()))
	for _, suggestion := range commandCatalogSuggestions() {
		command := strings.TrimPrefix(suggestion.Label, "/")
		if query == "" || strings.Contains(strings.ToLower(command), query) {
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions
}

func filterSuggestions(suggestions []tuiSuggestion, query string) []tuiSuggestion {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return dedupeSuggestions(suggestions)
	}
	filtered := make([]tuiSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		haystack := strings.ToLower(strings.Join([]string{
			suggestion.Label,
			suggestion.Insert,
			suggestion.Detail,
			suggestion.Category,
		}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, suggestion)
		}
	}
	return dedupeSuggestions(filtered)
}

func dedupeSuggestions(suggestions []tuiSuggestion) []tuiSuggestion {
	if len(suggestions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(suggestions))
	out := make([]tuiSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		key := suggestion.Insert
		if key == "" {
			key = suggestion.Label
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, suggestion)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			return out[i].Label < out[j].Label
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func limitSuggestions(suggestions []tuiSuggestion, limit int) []tuiSuggestion {
	if limit <= 0 || len(suggestions) <= limit {
		return suggestions
	}
	return suggestions[:limit]
}

func taskIDs(board *protocol.TaskBoard) []string {
	if board == nil {
		return nil
	}
	values := make([]string, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		values = append(values, task.TaskID)
	}
	sort.Strings(values)
	return values
}

func lastToken(fields []string, hasTrailingSpace bool) string {
	if hasTrailingSpace || len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
