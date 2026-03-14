package providers_test

import (
	"testing"

	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/providers"
	"github.com/arkonis-dev/ark-operator/runtime/agent/internal/providers/anthropic"
)

func TestNew_Anthropic(t *testing.T) {
	for _, name := range []string{"anthropic", ""} {
		p, err := providers.New(name)
		if err != nil {
			t.Errorf("New(%q) error: %v", name, err)
		}
		if p == nil {
			t.Errorf("New(%q) returned nil", name)
		}
		if _, ok := p.(*anthropic.Provider); !ok {
			t.Errorf("New(%q) = %T, want *anthropic.Provider", name, p)
		}
	}
}

func TestNew_Unknown(t *testing.T) {
	_, err := providers.New("gemini")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
