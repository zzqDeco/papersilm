package prepare

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/providers"
	"github.com/zzqDeco/papersilm/internal/runtime/agenttool"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Request struct {
	Config         config.Config
	Store          *storage.Store
	Registry       *tools.Registry
	SessionID      string
	PermissionMode protocol.PermissionMode
	Language       string
	Style          string
}

type PreparedAgent struct {
	Agent       adk.TypedAgent[*schema.AgenticMessage]
	Instruction string
}

func Agent(ctx context.Context, req Request) (PreparedAgent, error) {
	provider := req.Config.ActiveProviderConfig()
	model, err := providers.BuildAgenticModel(ctx, provider, req.Config.ProviderTimeout())
	if err != nil {
		return PreparedAgent{}, err
	}
	execTools, err := req.Registry.BuildExecutionTools(ctx, req.Store, req.SessionID, false)
	if err != nil {
		return PreparedAgent{}, err
	}
	workspaceTools, err := agenttool.BuildWorkspaceTools(agenttool.WorkspaceToolsConfig{
		Store:          req.Store,
		Registry:       req.Registry,
		SessionID:      req.SessionID,
		PermissionMode: req.PermissionMode,
	})
	if err != nil {
		return PreparedAgent{}, err
	}
	allTools := append(execTools, workspaceTools...)
	workspaceAgent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "workspace_agent",
		Description: "Sub-agent for reading, searching, and explaining the current workspace.",
		Instruction: "You are a focused workspace sub-agent. Use the provided context and answer concisely.",
		Model:       model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: workspaceTools,
			},
			EmitInternalEvents: true,
		},
		MaxIterations: 4,
	})
	if err != nil {
		return PreparedAgent{}, err
	}
	allTools = append(allTools, adk.NewTypedAgentTool(ctx, adk.TypedAgent[*schema.AgenticMessage](workspaceAgent)))
	instruction := instruction(req)
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "papersilm",
		Description: "Workspace-first research and coding assistant for papersilm sessions.",
		Instruction: instruction,
		Model:       model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: allTools,
			},
			EmitInternalEvents: true,
		},
		MaxIterations: 12,
	})
	if err != nil {
		return PreparedAgent{}, err
	}
	return PreparedAgent{Agent: agent, Instruction: instruction}, nil
}

func instruction(req Request) string {
	workspaceRoot := ""
	if req.Store != nil {
		workspaceRoot = req.Store.WorkspaceRoot()
	}
	var parts []string
	parts = append(parts,
		"You are papersilm, a workspace-first assistant.",
		"Always treat the current working directory as the default workspace.",
		"Sources are optional paper attachments; use workspace tools directly when the user asks about files or commands.",
		"For localized file edits, prefer workspace_replace_text with exact old_text/new_text; use workspace_write_file only for new files or whole-file rewrites.",
		"When workspace_run_command returns status=failed with an exit_code, treat it as a tool result and continue answering instead of treating it as a runtime failure.",
		"Do not emit low-level runtime logs to the user. Summarize tool activity briefly.",
		fmt.Sprintf("Workspace root: %s", workspaceRoot),
		fmt.Sprintf("Language: %s", fallback(req.Language, "zh")),
		fmt.Sprintf("Style: %s", fallback(req.Style, "distill")),
	)
	switch req.PermissionMode {
	case protocol.PermissionModePlan:
		parts = append(parts, "Permission mode is plan: produce a concise plan and do not perform side effects.")
	case protocol.PermissionModeConfirm:
		parts = append(parts, "Permission mode is confirm: use permission-gated tools for writes and shell commands.")
	default:
		parts = append(parts, "Permission mode is auto: execute safe workspace and paper tools as needed.")
	}
	return strings.Join(parts, "\n")
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
