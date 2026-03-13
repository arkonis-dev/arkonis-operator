package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// LLMProvider is the interface every LLM backend must implement.
//
// To add a new provider (e.g. OpenAI, Gemini):
//  1. Create provider_<name>.go in this package.
//  2. Implement LLMProvider on a new struct.
//  3. Register it in NewProvider below.
//
// The provider receives generic MCP tools and a callTool function so it can
// convert them to its own format without importing MCP-specific code.
type LLMProvider interface {
	// RunTask executes a task through the provider's agentic tool-use loop.
	// tools is the list of available MCP tools.
	// callTool executes a named tool on the appropriate MCP server.
	RunTask(
		ctx context.Context,
		cfg *Config,
		task Task,
		tools []MCPTool,
		callTool func(context.Context, string, json.RawMessage) (string, error),
	) (string, error)
}

// NewProvider returns the LLMProvider for the given provider name.
// provider is the value of AGENT_PROVIDER; defaults to "anthropic".
func NewProvider(provider string) (LLMProvider, error) {
	switch provider {
	case "anthropic", "":
		return &AnthropicProvider{}, nil
	// Add new providers here:
	// case "openai":
	//     return &OpenAIProvider{}, nil
	// case "gemini":
	//     return &GeminiProvider{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q; supported: anthropic", provider)
	}
}
