package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mcpToolsListHandler(tools []map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tools/list" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tools": tools})
	}
}

func TestDiscoverTools(t *testing.T) {
	srv := httptest.NewServer(mcpToolsListHandler([]map[string]any{
		{"name": "search", "description": "web search", "inputSchema": json.RawMessage(`{"type":"object"}`)},
		{"name": "fetch", "description": "fetch url", "inputSchema": json.RawMessage(`{"type":"object"}`)},
	}))
	defer srv.Close()

	tools, err := discoverTools(MCPServerConfig{Name: "web", URL: srv.URL})
	if err != nil {
		t.Fatalf("discoverTools error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	if tools[0].Name != "web__search" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "web__search")
	}
	if tools[0].OriginalName != "search" {
		t.Errorf("tools[0].OriginalName = %q, want %q", tools[0].OriginalName, "search")
	}
	if tools[0].ServerURL != srv.URL {
		t.Errorf("tools[0].ServerURL = %q, want %q", tools[0].ServerURL, srv.URL)
	}
}

func TestDiscoverTools_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := discoverTools(MCPServerConfig{Name: "bad", URL: srv.URL})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestNewMCPManager_NoServers(t *testing.T) {
	m, err := NewMCPManager(nil)
	if err != nil {
		t.Fatalf("NewMCPManager(nil) error: %v", err)
	}
	if len(m.Tools()) != 0 {
		t.Errorf("Tools() len = %d, want 0", len(m.Tools()))
	}
}

func TestCallTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tools": []map[string]any{
					{"name": "echo", "description": "echoes input", "inputSchema": json.RawMessage(`{}`)},
				},
			})
		case "/tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "hello from tool"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m, err := NewMCPManager([]MCPServerConfig{{Name: "test", URL: srv.URL}})
	if err != nil {
		t.Fatalf("NewMCPManager error: %v", err)
	}

	result, err := m.CallTool(context.Background(), "test__echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result != "hello from tool" {
		t.Errorf("CallTool = %q, want %q", result, "hello from tool")
	}
}

func TestCallTool_NotFound(t *testing.T) {
	m := &MCPManager{}
	_, err := m.CallTool(context.Background(), "nonexistent__tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}
