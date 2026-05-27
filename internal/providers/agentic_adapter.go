package providers

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type chatAgenticAdapter struct {
	chat model.ToolCallingChatModel
}

func newChatAgenticAdapter(chat model.ToolCallingChatModel) model.AgenticModel {
	return &chatAgenticAdapter{chat: chat}
}

func (a *chatAgenticAdapter) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	chat, opts, err := a.chatWithTools(opts...)
	if err != nil {
		return nil, err
	}
	msgs := make([]*schema.Message, 0, len(input))
	for _, msg := range input {
		msgs = append(msgs, agenticToChatMessage(msg))
	}
	out, err := chat.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return chatToAgenticMessage(out), nil
}

func (a *chatAgenticAdapter) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	chat, opts, err := a.chatWithTools(opts...)
	if err != nil {
		return nil, err
	}
	msgs := make([]*schema.Message, 0, len(input))
	for _, msg := range input {
		msgs = append(msgs, agenticToChatMessage(msg))
	}
	reader, err := chat.Stream(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderWithConvert(reader, func(msg *schema.Message) (*schema.AgenticMessage, error) {
		return chatToAgenticMessage(msg), nil
	}), nil
}

func (a *chatAgenticAdapter) chatWithTools(opts ...model.Option) (model.ToolCallingChatModel, []model.Option, error) {
	common := model.GetCommonOptions(nil, opts...)
	if len(common.Tools) == 0 {
		return a.chat, opts, nil
	}
	chat, err := a.chat.WithTools(common.Tools)
	if err != nil {
		return nil, nil, err
	}
	return chat, opts, nil
}

func agenticToChatMessage(msg *schema.AgenticMessage) *schema.Message {
	if msg == nil {
		return schema.UserMessage("")
	}
	text := agenticPlainText(msg)
	switch msg.Role {
	case schema.AgenticRoleTypeSystem:
		return schema.SystemMessage(text)
	case schema.AgenticRoleTypeAssistant:
		toolCalls := agenticToolCalls(msg)
		return schema.AssistantMessage(text, toolCalls)
	default:
		if result := agenticToolResult(msg); result != nil {
			return result
		}
		return schema.UserMessage(text)
	}
}

func chatToAgenticMessage(msg *schema.Message) *schema.AgenticMessage {
	if msg == nil {
		return schema.UserAgenticMessage("")
	}
	role := schema.AgenticRoleTypeUser
	switch msg.Role {
	case schema.System:
		role = schema.AgenticRoleTypeSystem
	case schema.Assistant:
		role = schema.AgenticRoleTypeAssistant
	}
	blocks := make([]*schema.ContentBlock, 0, 1+len(msg.ToolCalls))
	if strings.TrimSpace(msg.Content) != "" {
		if role == schema.AgenticRoleTypeAssistant {
			blocks = append(blocks, schema.NewContentBlock(&schema.AssistantGenText{Text: msg.Content}))
		} else {
			blocks = append(blocks, schema.NewContentBlock(&schema.UserInputText{Text: msg.Content}))
		}
	}
	for _, call := range msg.ToolCalls {
		blocks = append(blocks, schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		}))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, schema.NewContentBlock(&schema.AssistantGenText{Text: ""}))
	}
	return &schema.AgenticMessage{Role: role, ContentBlocks: blocks}
}

func agenticPlainText(msg *schema.AgenticMessage) string {
	parts := make([]string, 0, len(msg.ContentBlocks))
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.UserInputText != nil:
			parts = append(parts, block.UserInputText.Text)
		case block.AssistantGenText != nil:
			parts = append(parts, block.AssistantGenText.Text)
		case block.FunctionToolResult != nil:
			for _, content := range block.FunctionToolResult.Content {
				if content != nil && content.Text != nil {
					parts = append(parts, content.Text.Text)
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func agenticToolCalls(msg *schema.AgenticMessage) []schema.ToolCall {
	out := make([]schema.ToolCall, 0)
	for _, block := range msg.ContentBlocks {
		if block == nil || block.FunctionToolCall == nil {
			continue
		}
		call := block.FunctionToolCall
		out = append(out, schema.ToolCall{
			ID:   call.CallID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      call.Name,
				Arguments: call.Arguments,
			},
		})
	}
	return out
}

func agenticToolResult(msg *schema.AgenticMessage) *schema.Message {
	for _, block := range msg.ContentBlocks {
		if block == nil || block.FunctionToolResult == nil {
			continue
		}
		result := block.FunctionToolResult
		parts := make([]string, 0, len(result.Content))
		for _, content := range result.Content {
			if content != nil && content.Text != nil {
				parts = append(parts, content.Text.Text)
			}
		}
		return schema.ToolMessage(strings.Join(parts, "\n"), result.CallID, schema.WithToolName(result.Name))
	}
	return nil
}
