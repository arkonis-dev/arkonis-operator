package main

import "context"

// Runner executes tasks by delegating to a configured LLMProvider.
// It is provider-agnostic: swap the provider to use a different LLM backend.
type Runner struct {
	cfg        *Config
	mcpManager *MCPManager
	provider   LLMProvider
}

// NewRunner creates a Runner with the given provider and MCP tool manager.
func NewRunner(cfg *Config, mcpManager *MCPManager, provider LLMProvider) *Runner {
	return &Runner{cfg: cfg, mcpManager: mcpManager, provider: provider}
}

// RunTask executes a single task through the provider's agentic loop.
func (r *Runner) RunTask(ctx context.Context, task Task) (string, error) {
	return r.provider.RunTask(ctx, r.cfg, task, r.mcpManager.Tools(), r.mcpManager.CallTool)
}
