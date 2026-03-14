// Package providers defines the LLMProvider interface and a self-registration
// registry. Each provider implementation registers itself via its init() function,
// so adding a new provider requires only:
//
//  1. Create internal/providers/<name>/provider.go implementing LLMProvider.
//  2. Call providers.Register("<name>", ...) in its init().
//  3. Add a blank import in main.go: _ "…/providers/<name>"
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/config"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/mcp"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/queue"
)

// LLMProvider is the interface every LLM backend must implement.
type LLMProvider interface {
	// RunTask executes a task through the provider's agentic tool-use loop.
	// tools is the merged list of available tools (MCP + webhook + built-ins).
	// callTool dispatches a named tool invocation and returns the result.
	// Returns the text result, token usage accumulated across all LLM calls, and any error.
	RunTask(
		ctx context.Context,
		cfg *config.Config,
		task queue.Task,
		tools []mcp.Tool,
		callTool func(context.Context, string, json.RawMessage) (string, error),
	) (string, queue.TokenUsage, error)
}

var (
	mu       sync.RWMutex
	registry = map[string]func() LLMProvider{}
)

// Register adds a provider factory to the registry under the given name.
// Call this from an init() function in each provider package.
func Register(name string, factory func() LLMProvider) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// New returns the LLMProvider for the given name.
// An empty name defaults to "anthropic".
func New(name string) (LLMProvider, error) {
	if name == "" {
		name = "anthropic"
	}
	mu.RLock()
	factory, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q; import its package to register it", name)
	}
	return factory(), nil
}
