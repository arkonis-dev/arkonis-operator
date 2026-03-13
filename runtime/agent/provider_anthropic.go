package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements LLMProvider using the Anthropic API.
type AnthropicProvider struct{}

// RunTask executes a task through the Anthropic agentic tool-use loop.
// It keeps calling the API until the model stops requesting tool use.
func (p *AnthropicProvider) RunTask(
	ctx context.Context,
	cfg *Config,
	task Task,
	tools []MCPTool,
	callTool func(context.Context, string, json.RawMessage) (string, error),
) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is not set; required for the anthropic provider")
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	anthropicTools := toAnthropicTools(tools)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task.Prompt)),
	}

	for {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(cfg.Model),
			MaxTokens: int64(cfg.MaxTokensPerCall),
			System: []anthropic.TextBlockParam{
				{Text: cfg.SystemPrompt},
			},
			Messages: messages,
		}
		if len(anthropicTools) > 0 {
			params.Tools = anthropicTools
		}

		resp, err := client.Messages.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("anthropic API error: %w", err)
		}

		messages = append(messages, anthropicAssistantMessage(resp.Content))

		if resp.StopReason == anthropic.StopReasonEndTurn {
			return anthropicExtractText(resp.Content), nil
		}

		toolResults := anthropicExecuteTools(ctx, resp.Content, callTool)
		if len(toolResults) == 0 {
			return anthropicExtractText(resp.Content), nil
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}

// toAnthropicTools converts generic MCPTools into the Anthropic SDK format.
func toAnthropicTools(tools []MCPTool) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.InputSchema,
			},
		}
		params = append(params, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return params
}

// anthropicExecuteTools runs each tool_use block against the MCP server via callTool.
func anthropicExecuteTools(
	ctx context.Context,
	content []anthropic.ContentBlockUnion,
	callTool func(context.Context, string, json.RawMessage) (string, error),
) []anthropic.ContentBlockParamUnion {
	var results []anthropic.ContentBlockParamUnion
	for _, block := range content {
		if block.Type != "tool_use" {
			continue
		}
		output, err := callTool(ctx, block.Name, block.Input)
		if err != nil {
			results = append(results, anthropic.NewToolResultBlock(block.ID, err.Error(), true))
			continue
		}
		results = append(results, anthropic.NewToolResultBlock(block.ID, output, false))
	}
	return results
}

// anthropicAssistantMessage converts a response content slice into a MessageParam
// for appending to the conversation history.
func anthropicAssistantMessage(content []anthropic.ContentBlockUnion) anthropic.MessageParam {
	params := make([]anthropic.ContentBlockParamUnion, 0, len(content))
	for _, block := range content {
		switch block.Type {
		case "text":
			params = append(params, anthropic.NewTextBlock(block.Text))
		case "tool_use":
			params = append(params, anthropic.NewToolUseBlock(block.ID, block.Input, block.Name))
		}
	}
	return anthropic.NewAssistantMessage(params...)
}

// anthropicExtractText returns the text of the first text block in content.
func anthropicExtractText(content []anthropic.ContentBlockUnion) string {
	for _, block := range content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}
