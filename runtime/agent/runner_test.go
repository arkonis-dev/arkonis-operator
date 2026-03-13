package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type mockProvider struct {
	result string
	err    error
}

func (m *mockProvider) RunTask(
	_ context.Context,
	_ *Config,
	_ Task,
	_ []MCPTool,
	_ func(context.Context, string, json.RawMessage) (string, error),
) (string, error) {
	return m.result, m.err
}

func TestRunner_RunTask_Success(t *testing.T) {
	cfg := &Config{Model: "claude-sonnet-4-6", SystemPrompt: "test"}
	mgr := &MCPManager{}
	provider := &mockProvider{result: "done"}

	runner := NewRunner(cfg, mgr, provider)
	result, err := runner.RunTask(context.Background(), Task{ID: "1", Prompt: "hello"})
	if err != nil {
		t.Fatalf("RunTask error: %v", err)
	}
	if result != "done" {
		t.Errorf("RunTask = %q, want %q", result, "done")
	}
}

func TestRunner_RunTask_ProviderError(t *testing.T) {
	cfg := &Config{Model: "claude-sonnet-4-6", SystemPrompt: "test"}
	mgr := &MCPManager{}
	provider := &mockProvider{err: errors.New("api error")}

	runner := NewRunner(cfg, mgr, provider)
	_, err := runner.RunTask(context.Background(), Task{ID: "2", Prompt: "hello"})
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}
}
