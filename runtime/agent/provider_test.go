package main

import (
	"testing"
)

func TestNewProvider_Anthropic(t *testing.T) {
	for _, name := range []string{"anthropic", ""} {
		p, err := NewProvider(name)
		if err != nil {
			t.Errorf("NewProvider(%q) error: %v", name, err)
		}
		if p == nil {
			t.Errorf("NewProvider(%q) returned nil", name)
		}
		if _, ok := p.(*AnthropicProvider); !ok {
			t.Errorf("NewProvider(%q) = %T, want *AnthropicProvider", name, p)
		}
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider("openai")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
