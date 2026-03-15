// ark is a local development CLI for ark-operator.
// It lets developers run, test, and debug ArkFlow pipelines without a
// Kubernetes cluster, reusing the same YAML files that kubectl apply -f accepts.
//
// Usage:
//
//	ark run quickstart.yaml
//	ark run quickstart.yaml --mock
//	ark run quickstart.yaml --dry-run
//	ark run quickstart.yaml --watch
//	ark run quickstart.yaml --output json
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
	"github.com/arkonis-dev/ark-operator/internal/agent/providers"
	_ "github.com/arkonis-dev/ark-operator/internal/agent/providers/anthropic"
	_ "github.com/arkonis-dev/ark-operator/internal/agent/providers/mock"
	_ "github.com/arkonis-dev/ark-operator/internal/agent/providers/openai"
	"github.com/arkonis-dev/ark-operator/internal/ark"
	"github.com/arkonis-dev/ark-operator/internal/ark/local"
	"github.com/arkonis-dev/ark-operator/internal/flow"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
// Falls back to "dev" for local builds.
var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "ark",
		Short:   "Local development CLI for ark-operator",
		Version: version,
		Long: `ark lets you run, test, and debug ArkFlow pipelines locally
without a Kubernetes cluster. The same YAML files work unchanged
with kubectl apply -f when you are ready to deploy.`,
	}
	root.AddCommand(runCmd())
	root.AddCommand(validateCmd())
	root.AddCommand(initCmd())
	return root
}

// runFlags holds the parsed flags for the run subcommand.
type runFlags struct {
	provider string
	flow     string
	dryRun   bool
	watch    bool
	noMCP    bool
	output   string
	input    string
}

func runCmd() *cobra.Command {
	var f runFlags

	cmd := &cobra.Command{
		Use:   "run <file>",
		Short: "Execute an ArkFlow locally",
		Long: `Run an ArkFlow defined in a multi-document YAML file.
The file may contain ArkAgent, ArkFlow, and other resource definitions —
only ArkAgent and ArkFlow documents are used; everything else is ignored.

Examples:
  ark run quickstart.yaml
  ark run quickstart.yaml --provider mock
  ark run quickstart.yaml --provider openai --watch
  ark run quickstart.yaml --watch --output json
  ark run pipeline.yaml --input '{"topic":"Kubernetes operators"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFlow(cmd.Context(), args[0], f)
		},
	}

	cmd.Flags().StringVar(&f.provider, "provider", "auto", "LLM provider: auto, anthropic, openai, or mock")
	cmd.Flags().StringVar(&f.flow, "flow", "", "Flow name to run when the file contains multiple ArkFlow resources")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Validate YAML and print a summary without executing")
	cmd.Flags().BoolVar(&f.watch, "watch", false, "Stream step-by-step output as the flow executes")
	cmd.Flags().BoolVar(&f.noMCP, "no-mcp", false, "Skip MCP tool server connections")
	cmd.Flags().StringVar(&f.output, "output", "text", "Output format: text or json")
	cmd.Flags().StringVar(&f.input, "input", "", `Flow input as a JSON object, e.g. '{"topic":"Kubernetes"}'`)

	return cmd
}

func runFlow(ctx context.Context, path string, flags runFlags) error {
	flows, agents, err := ark.LoadFile(path)
	if err != nil {
		return err
	}
	if len(flows) == 0 {
		return fmt.Errorf("no ArkFlow resources found in %s", path)
	}

	f, err := selectFlow(flows, flags.flow)
	if err != nil {
		return err
	}

	// Apply --input override.
	if flags.input != "" {
		var inputMap map[string]string
		if err := json.Unmarshal([]byte(flags.input), &inputMap); err != nil {
			return fmt.Errorf("--input must be a JSON object of string values: %w", err)
		}
		f.Spec.Input = inputMap
	}

	if flags.dryRun {
		return dryRun(f, agents)
	}

	exec := &local.Executor{
		Provider: buildProviderFunc(flags.provider),
		NoMCP:    flags.noMCP,
	}

	start := time.Now()
	onEvent := buildEventHandler(flags.watch, flags.output)

	if err := exec.Run(ctx, f, agents, onEvent); err != nil {
		return err
	}

	return printResult(f, time.Since(start), flags.output)
}

// selectFlow picks the flow to run.
// If name is set, it looks up that flow by name. If the file has multiple flows
// and no name was given, it warns the user and picks the first one.
func selectFlow(flows []*arkonisv1alpha1.ArkFlow, name string) (*arkonisv1alpha1.ArkFlow, error) {
	if name != "" {
		for _, f := range flows {
			if f.Name == name {
				return f, nil
			}
		}
		names := make([]string, len(flows))
		for i, f := range flows {
			names[i] = f.Name
		}
		return nil, fmt.Errorf("flow %q not found; available: %v", name, names)
	}
	if len(flows) > 1 {
		names := make([]string, len(flows))
		for i, f := range flows {
			names[i] = f.Name
		}
		fmt.Fprintf(os.Stderr, "warning: %d flows found %v — running %q (use --flow <name> to pick)\n",
			len(flows), names, flows[0].Name)
	}
	return flows[0], nil
}

// validateCmd returns the `ark validate` subcommand.
func validateCmd() *cobra.Command {
	var flowName string
	cmd := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate an ArkFlow YAML without executing it",
		Long: `Parse the YAML, check DAG integrity, and verify that every step
references a known ArkAgent. Exits 0 on success, non-zero on error.

Examples:
  ark validate quickstart.yaml
  ark validate pipeline.yaml --flow my-flow`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flows, agents, err := ark.LoadFile(args[0])
			if err != nil {
				return err
			}
			if len(flows) == 0 {
				return fmt.Errorf("no ArkFlow resources found in %s", args[0])
			}
			f, err := selectFlow(flows, flowName)
			if err != nil {
				return err
			}
			return dryRun(f, agents)
		},
	}
	cmd.Flags().StringVar(&flowName, "flow", "", "Flow name to validate when the file contains multiple ArkFlow resources")
	return cmd
}

// dryRun validates the flow and prints a summary without executing.
// It checks DAG integrity and that every step references a known ArkAgent.
func dryRun(f *arkonisv1alpha1.ArkFlow, agents map[string]*arkonisv1alpha1.ArkAgent) error {
	if err := flow.ValidateDAG(f); err != nil {
		return fmt.Errorf("invalid DAG: %w", err)
	}

	var missingAgents []string
	fmt.Printf("Flow:   %s\n", f.Name)
	fmt.Printf("Steps:  %d\n", len(f.Spec.Steps))
	for _, step := range f.Spec.Steps {
		agent, ok := agents[step.ArkAgent]
		agentModel := "<not found>"
		if ok {
			agentModel = agent.Spec.Model
		} else {
			missingAgents = append(missingAgents,
				fmt.Sprintf("step %q references unknown ArkAgent %q", step.Name, step.ArkAgent))
		}
		deps := "-"
		if len(step.DependsOn) > 0 {
			deps = fmt.Sprintf("%v", step.DependsOn)
		}
		fmt.Printf("  %-20s  agent=%-20s  deps=%s\n", step.Name, agentModel, deps)
	}
	if len(missingAgents) > 0 {
		for _, msg := range missingAgents {
			fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		}
		return fmt.Errorf("%d missing agent(s) — add the ArkAgent definitions to your YAML", len(missingAgents))
	}
	fmt.Println("✓ YAML is valid — use `ark run` to execute")
	return nil
}

// buildProviderFunc returns a per-model provider lookup function.
//
// When flag is "auto" (the default), the provider is detected from the model
// name at step execution time — claude-* uses anthropic, gpt-*/o* uses openai.
// Any other value pins every step to that specific provider.
func buildProviderFunc(flag string) func(model string) (providers.LLMProvider, error) {
	return func(model string) (providers.LLMProvider, error) {
		name := flag
		if name == "auto" {
			name = providers.Detect(model)
		}
		p, err := providers.New(name)
		if err != nil {
			return nil, fmt.Errorf("unknown provider %q — supported: anthropic, openai, mock", name)
		}
		return p, nil
	}
}

// buildEventHandler returns a function that prints progress for --watch mode.
// In PR 4 this will be replaced with rich terminal output; for now it emits
// simple one-line status updates.
func buildEventHandler(watch bool, outputFmt string) func(local.StepEvent) {
	if !watch || outputFmt == "json" {
		return func(local.StepEvent) {} // silent until flow completes
	}
	return func(evt local.StepEvent) {
		switch evt.Phase {
		case arkonisv1alpha1.ArkFlowStepPhaseRunning:
			fmt.Printf("  %-20s [running]\n", evt.Step)
		case arkonisv1alpha1.ArkFlowStepPhaseSucceeded:
			tokens := evt.Tokens.InputTokens + evt.Tokens.OutputTokens
			fmt.Printf("  %-20s [done]    %d tokens  %.1fs\n", evt.Step, tokens, evt.Elapsed.Seconds())
			if evt.Output != "" {
				preview := evt.Output
				if len(preview) > 120 {
					preview = preview[:120] + "..."
				}
				fmt.Printf("    └─ %s\n", preview)
			}
		case arkonisv1alpha1.ArkFlowStepPhaseSkipped:
			fmt.Printf("  %-20s [skipped]\n", evt.Step)
		case arkonisv1alpha1.ArkFlowStepPhaseFailed:
			fmt.Printf("  %-20s [failed]  %v\n", evt.Step, evt.Err)
		}
	}
}

// printResult prints the final flow result in the requested output format.
func printResult(f *arkonisv1alpha1.ArkFlow, elapsed time.Duration, outputFmt string) error {
	if outputFmt == "json" {
		return printJSON(f, elapsed)
	}
	return printText(f, elapsed)
}

func printText(f *arkonisv1alpha1.ArkFlow, elapsed time.Duration) error {
	total := int64(0)
	if f.Status.TotalTokenUsage != nil {
		total = f.Status.TotalTokenUsage.TotalTokens
	}
	phase := string(f.Status.Phase)
	fmt.Printf("\nFlow %s in %.1fs — total: %d tokens\n", phase, elapsed.Seconds(), total)
	if f.Status.Output != "" {
		fmt.Printf("\nOutput:\n%s\n", f.Status.Output)
	}
	if f.Status.Phase == arkonisv1alpha1.ArkFlowPhaseFailed {
		return fmt.Errorf("flow failed")
	}
	return nil
}

// initCmd returns the `ark init` subcommand.
func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <project>",
		Short: "Scaffold a new ark project",
		Long: `Create a new directory with a ready-to-run ArkFlow project.

The generated project contains:
  quickstart.yaml   — ArkAgent + ArkFlow definition
  .env.example      — required environment variables
  docker-compose.yml — local Redis for task queue

Examples:
  ark init my-agent
  cd my-agent && ark run quickstart.yaml --provider mock`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return initProject(args[0])
		},
	}
}

func initProject(name string) error {
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}
	if err := os.MkdirAll(name, 0755); err != nil {
		return err
	}

	files := map[string]string{
		"quickstart.yaml":    initQuickstartYAML(name),
		".env.example":       initEnvExample,
		"docker-compose.yml": initDockerCompose,
	}
	for filename, content := range files {
		path := fmt.Sprintf("%s/%s", name, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	fmt.Printf("Created project %q\n\n", name)
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", name)
	fmt.Println("  ark run quickstart.yaml --provider mock --watch")
	fmt.Println("  ark run quickstart.yaml --provider anthropic --watch")
	return nil
}

func initQuickstartYAML(name string) string {
	return fmt.Sprintf(`apiVersion: arkonis.dev/v1alpha1
kind: ArkAgent
metadata:
  name: %s-agent
spec:
  model: claude-sonnet-4-20250514
  systemPrompt: |
    You are a helpful assistant. Answer questions clearly and concisely.
  limits:
    maxTokensPerCall: 4000
    timeoutSeconds: 60
---
apiVersion: arkonis.dev/v1alpha1
kind: ArkFlow
metadata:
  name: %s-flow
spec:
  input:
    question: "What is a Kubernetes operator?"
  steps:
    - name: answer
      arkAgent: %s-agent
      inputs:
        prompt: "{{ .pipeline.input.question }}"
  output: "{{ .steps.answer.output }}"
`, name, name, name)
}

const initEnvExample = `# Copy this file to .env and fill in your API keys.

# Required: LLM provider API key.
ANTHROPIC_API_KEY=sk-ant-...
# OPENAI_API_KEY=sk-...

# Required for Kubernetes deployments (not needed for ark run locally).
TASK_QUEUE_URL=redis.ark-system.svc.cluster.local:6379
`

const initDockerCompose = `# Local Redis for task queue — used when running the operator in a cluster.
# Not required for ark run locally.
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
`

func printJSON(f *arkonisv1alpha1.ArkFlow, elapsed time.Duration) error {
	type stepOut struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Tokens int64  `json:"tokens"`
		Output string `json:"output,omitempty"`
	}
	type result struct {
		Status      string    `json:"status"`
		DurationMS  int64     `json:"duration_ms"`
		TotalTokens int64     `json:"total_tokens"`
		Output      string    `json:"output,omitempty"`
		Steps       []stepOut `json:"steps"`
	}

	r := result{
		Status:     string(f.Status.Phase),
		DurationMS: elapsed.Milliseconds(),
	}
	if f.Status.TotalTokenUsage != nil {
		r.TotalTokens = f.Status.TotalTokenUsage.TotalTokens
	}
	r.Output = f.Status.Output
	for _, st := range f.Status.Steps {
		tokens := int64(0)
		if st.TokenUsage != nil {
			tokens = st.TokenUsage.TotalTokens
		}
		r.Steps = append(r.Steps, stepOut{
			Name:   st.Name,
			Status: string(st.Phase),
			Tokens: tokens,
			Output: st.Output,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if f.Status.Phase == arkonisv1alpha1.ArkFlowPhaseFailed {
		return fmt.Errorf("flow failed")
	}
	return nil
}
