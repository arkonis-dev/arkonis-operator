// Package openai will implement LLMProvider for OpenAI-compatible APIs.
// Importing this package registers the "openai" provider in the global registry.
//
// TODO(v0.7.0): implement the full tool-use loop.
package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/config"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/mcp"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/providers"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/queue"
)

func init() {
	providers.Register("openai", func() providers.LLMProvider { return &Provider{} })
}

// Provider implements providers.LLMProvider for OpenAI-compatible APIs.
type Provider struct{}

// RunTask is a stub that will be implemented in v0.7.0.
func (p *Provider) RunTask(
	_ context.Context,
	_ *config.Config,
	_ queue.Task,
	_ []mcp.Tool,
	_ func(context.Context, string, json.RawMessage) (string, error),
) (string, queue.TokenUsage, error) {
	return "", queue.TokenUsage{}, fmt.Errorf("openai provider: not yet implemented (planned for v0.7.0)")
}
