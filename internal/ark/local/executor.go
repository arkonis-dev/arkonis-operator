// Package local provides a flow executor that runs ArkFlow steps directly
// in the current process — no Kubernetes cluster or Redis required.
// It reuses runtime/agent's Runner (MCP, tool dispatch, provider loop)
// and internal/flow's DAG logic unchanged.
package local

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
	"github.com/arkonis-dev/ark-operator/internal/agent/config"
	"github.com/arkonis-dev/ark-operator/internal/agent/mcp"
	"github.com/arkonis-dev/ark-operator/internal/agent/providers"
	"github.com/arkonis-dev/ark-operator/internal/agent/queue"
	"github.com/arkonis-dev/ark-operator/internal/agent/runner"
	"github.com/arkonis-dev/ark-operator/internal/flow"
)

// StepEvent is emitted by the executor on each step state transition.
// Callers use it to render --watch output or collect --output json results.
type StepEvent struct {
	Step    string
	Phase   arkonisv1alpha1.ArkFlowStepPhase
	Output  string
	Tokens  queue.TokenUsage
	Elapsed time.Duration
	Err     error
}

// Executor drives an ArkFlow locally without a Kubernetes cluster or Redis.
type Executor struct {
	// Provider returns the LLMProvider to use for a given model name.
	// Each step calls Provider(agent.Spec.Model), so a flow can mix models
	// from different backends (e.g. claude-* and gpt-*) in the same run.
	Provider func(model string) (providers.LLMProvider, error)

	// NoMCP disables all MCP tool connections. When false (the default),
	// unreachable MCP servers are silently skipped.
	NoMCP bool
}

// Run executes the flow to completion, calling onEvent on each step transition.
// The flow's Status is updated in place — callers can inspect it after Run returns.
// Returns a non-nil error only for unexpected execution failures; a flow that
// ends in Failed phase returns nil (the caller checks f.Status.Phase).
func (e *Executor) Run(
	ctx context.Context,
	f *arkonisv1alpha1.ArkFlow,
	agents map[string]*arkonisv1alpha1.ArkAgent,
	onEvent func(StepEvent),
) error {
	flow.InitializeSteps(f)

	for !flow.IsTerminalPhase(f.Status.Phase) {
		statusByName := flow.BuildStatusByName(f)
		templateData := flow.BuildTemplateData(f, statusByName)
		flow.EvaluateLoops(f, statusByName, templateData)

		ready := e.collectReadySteps(f, statusByName, templateData)

		// No steps are ready and nothing is running — flow is stuck (shouldn't
		// happen after DAG validation, but handle it gracefully).
		if len(ready) == 0 {
			flow.UpdateFlowPhase(f, templateData)
			break
		}

		// Execute all ready steps in parallel, collect results via channel.
		type stepResult struct {
			name    string
			output  string
			tokens  queue.TokenUsage
			elapsed time.Duration
			err     error
		}
		results := make(chan stepResult, len(ready))

		for _, step := range ready {
			st := statusByName[step.Name]
			now := metav1.Now()
			st.Phase = arkonisv1alpha1.ArkFlowStepPhaseRunning
			st.StartTime = &now
			onEvent(StepEvent{Step: step.Name, Phase: arkonisv1alpha1.ArkFlowStepPhaseRunning})

			go func(s arkonisv1alpha1.ArkFlowStep) {
				start := time.Now()
				prompt, _ := flow.ResolvePrompt(s, templateData)
				output, tokens, execErr := e.runStep(ctx, s, agents, prompt)
				results <- stepResult{
					name:    s.Name,
					output:  output,
					tokens:  tokens,
					elapsed: time.Since(start),
					err:     execErr,
				}
			}(step)
		}

		// Collect all results before advancing.
		for range ready {
			res := <-results
			st := statusByName[res.name]
			now := metav1.Now()
			st.CompletionTime = &now

			evt := StepEvent{
				Step:    res.name,
				Output:  res.output,
				Tokens:  res.tokens,
				Elapsed: res.elapsed,
				Err:     res.err,
			}
			if res.err != nil {
				st.Phase = arkonisv1alpha1.ArkFlowStepPhaseFailed
				st.Message = res.err.Error()
				evt.Phase = arkonisv1alpha1.ArkFlowStepPhaseFailed
			} else {
				st.Phase = arkonisv1alpha1.ArkFlowStepPhaseSucceeded
				st.Output = res.output
				st.TokenUsage = &arkonisv1alpha1.TokenUsage{
					InputTokens:  res.tokens.InputTokens,
					OutputTokens: res.tokens.OutputTokens,
					TotalTokens:  res.tokens.InputTokens + res.tokens.OutputTokens,
				}
				evt.Phase = arkonisv1alpha1.ArkFlowStepPhaseSucceeded
			}
			onEvent(evt)
		}

		flow.ParseOutputJSON(f, statusByName)
		templateData = flow.BuildTemplateData(f, statusByName)
		flow.UpdateFlowPhase(f, templateData)
	}

	return nil
}

// collectReadySteps returns all Pending steps whose deps are satisfied.
// It handles if: conditionals inline, marking false-condition steps as Skipped.
func (e *Executor) collectReadySteps(
	f *arkonisv1alpha1.ArkFlow,
	statusByName map[string]*arkonisv1alpha1.ArkFlowStepStatus,
	templateData map[string]any,
) []arkonisv1alpha1.ArkFlowStep {
	var ready []arkonisv1alpha1.ArkFlowStep
	for _, step := range f.Spec.Steps {
		st := statusByName[step.Name]
		if st == nil || st.Phase != arkonisv1alpha1.ArkFlowStepPhasePending {
			continue
		}
		if !flow.DepsSucceeded(step.DependsOn, statusByName) {
			continue
		}
		if step.If != "" {
			result, err := flow.ResolveTemplate(step.If, templateData)
			if err != nil || !flow.IsTruthy(result) {
				now := metav1.Now()
				st.Phase = arkonisv1alpha1.ArkFlowStepPhaseSkipped
				st.CompletionTime = &now
				st.Message = "skipped: if condition evaluated to false"
				continue
			}
		}
		ready = append(ready, step)
	}
	return ready
}

// runStep executes a single step by looking up its ArkAgent, building a runner,
// and calling runner.RunTask directly (no Redis).
func (e *Executor) runStep(
	ctx context.Context,
	step arkonisv1alpha1.ArkFlowStep,
	agents map[string]*arkonisv1alpha1.ArkAgent,
	prompt string,
) (string, queue.TokenUsage, error) {
	agent, ok := agents[step.ArkAgent]
	if !ok {
		return "", queue.TokenUsage{}, fmt.Errorf("step %q: ArkAgent %q not found in loaded YAML", step.Name, step.ArkAgent)
	}

	cfg := agentConfig(agent)
	provider, err := e.Provider(agent.Spec.Model)
	if err != nil {
		return "", queue.TokenUsage{}, fmt.Errorf("step %q: %w", step.Name, err)
	}
	mcpMgr := e.buildMCPManager(cfg.MCPServers)
	r := runner.New(cfg, mcpMgr, provider, nil /* no local task queue */)

	task := queue.Task{ID: step.Name, Prompt: prompt}

	stepCtx := ctx
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	return r.RunTask(stepCtx, task)
}

// agentConfig builds a runtime config.Config from an ArkAgent spec.
// SystemPromptRef cannot be resolved locally (no k8s client); if set and
// SystemPrompt is empty, a warning is embedded in the system prompt field.
func agentConfig(agent *arkonisv1alpha1.ArkAgent) *config.Config {
	systemPrompt := agent.Spec.SystemPrompt
	if systemPrompt == "" && agent.Spec.SystemPromptRef != nil {
		systemPrompt = "[warning: systemPromptRef cannot be resolved locally — set systemPrompt inline for ark run]"
	}

	cfg := &config.Config{
		Model:            agent.Spec.Model,
		SystemPrompt:     systemPrompt,
		MaxTokensPerCall: 8000,
		TimeoutSeconds:   120,
	}

	if agent.Spec.Limits != nil {
		if agent.Spec.Limits.MaxTokensPerCall > 0 {
			cfg.MaxTokensPerCall = agent.Spec.Limits.MaxTokensPerCall
		}
		if agent.Spec.Limits.TimeoutSeconds > 0 {
			cfg.TimeoutSeconds = agent.Spec.Limits.TimeoutSeconds
		}
	}

	for _, s := range agent.Spec.MCPServers {
		cfg.MCPServers = append(cfg.MCPServers, config.MCPServerConfig{
			Name: s.Name,
			URL:  s.URL,
		})
	}

	return cfg
}

// buildMCPManager attempts to connect to each MCP server individually,
// silently skipping any that are unreachable. When e.NoMCP is true,
// returns an empty manager without attempting any connections.
func (e *Executor) buildMCPManager(servers []config.MCPServerConfig) *mcp.Manager {
	if e.NoMCP || len(servers) == 0 {
		m, _ := mcp.NewManager(nil)
		return m
	}
	var reachable []config.MCPServerConfig
	for _, s := range servers {
		if _, err := mcp.NewManager([]config.MCPServerConfig{s}); err == nil {
			reachable = append(reachable, s)
		}
	}
	m, _ := mcp.NewManager(reachable)
	return m
}
