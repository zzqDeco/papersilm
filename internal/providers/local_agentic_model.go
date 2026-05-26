package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type LocalAgenticModel struct{}

func NewLocalAgenticModel() model.AgenticModel {
	return &LocalAgenticModel{}
}

func (m *LocalAgenticModel) Generate(_ context.Context, input []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	return localAgenticTextMessage(localAgenticResponse(input)), nil
}

func (m *LocalAgenticModel) Stream(ctx context.Context, input []*schema.AgenticMessage, _ ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	reader, writer := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer writer.Close()
		select {
		case <-ctx.Done():
			writer.Send(nil, ctx.Err())
		default:
			writer.Send(localAgenticTextMessage(localAgenticResponse(input)), nil)
		}
	}()
	return reader, nil
}

func localAgenticTextMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func localAgenticResponse(input []*schema.AgenticMessage) string {
	last := ""
	for i := len(input) - 1; i >= 0; i-- {
		if input[i] == nil || input[i].Role != schema.AgenticRoleTypeUser {
			continue
		}
		last = agenticMessageText(input[i])
		break
	}
	last = strings.TrimSpace(last)
	if last == "" {
		return "No user input was provided."
	}
	return fmt.Sprintf("Local Eino runtime received the workspace task:\n\n%s\n\nConfigure an OpenAI-compatible provider for full tool-calling execution.", last)
}

func agenticMessageText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var parts []string
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
	return strings.Join(parts, "\n")
}
