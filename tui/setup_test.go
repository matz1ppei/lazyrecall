package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
)

func TestSetupPromptWithoutAIShowsManualAddPath(t *testing.T) {
	m := NewSetupModel(nil, nil, config.Config{})

	view := m.View()
	if !strings.Contains(view, "AI is not configured") {
		t.Fatalf("expected AI warning, got: %s", view)
	}
	if !strings.Contains(view, "[a] Add your first card manually") {
		t.Fatalf("expected manual add hint, got: %s", view)
	}
}

func TestSetupPromptWithoutAIOnYShowsInlineError(t *testing.T) {
	m := NewSetupModel(nil, nil, config.Config{})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd != nil {
		t.Fatal("expected no command")
	}
	got := updated.(SetupModel)
	if got.state != setupStatePrompt {
		t.Fatalf("state = %v, want %v", got.state, setupStatePrompt)
	}
	if got.inlineErr == "" {
		t.Fatal("expected inlineErr")
	}
}

func TestSetupPromptWithoutAIOnAGoesToAdd(t *testing.T) {
	m := NewSetupModel(nil, nil, config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	gotoMsg, ok := msg.(MsgGotoScreen)
	if !ok {
		t.Fatalf("expected MsgGotoScreen, got %T", msg)
	}
	if gotoMsg.Target != screenAdd {
		t.Fatalf("target = %v, want %v", gotoMsg.Target, screenAdd)
	}
}
