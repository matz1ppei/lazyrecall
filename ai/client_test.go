package ai

import (
	"os"
	"testing"
)

func TestNewClient_NoEnv_ReturnsNil(t *testing.T) {
	os.Unsetenv("AI_BACKEND")
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OLLAMA_BASE_URL")
	os.Unsetenv("OLLAMA_MODEL")

	// Default backend is ollama; nil, nil means "not an error, just disabled"
	// When AI_BACKEND is unset we still get an OllamaClient (not nil),
	// but when AI_BACKEND is explicitly set to an unknown value we get nil.
	os.Setenv("AI_BACKEND", "none")
	c, err := NewClient("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil client for unknown backend, got %T", c)
	}
	os.Unsetenv("AI_BACKEND")
}

func TestNewClient_ClaudeWithoutKey_ReturnsNil(t *testing.T) {
	os.Setenv("AI_BACKEND", "claude")
	os.Unsetenv("ANTHROPIC_API_KEY")

	c, err := NewClient("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil when ANTHROPIC_API_KEY missing, got %T", c)
	}
	os.Unsetenv("AI_BACKEND")
}

func TestMockClient_ImplementsInterface(t *testing.T) {
	var _ Client = &MockClient{}
}
