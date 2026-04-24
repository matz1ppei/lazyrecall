package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposeInitWithoutAIRendersNoAIState(t *testing.T) {
	m := NewComposeModel(nil, nil, 80, "")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init cmd")
	}
	msg := cmd()

	updated, _ := m.Update(msg)
	got := updated.(ComposeModel)
	if got.state != composeStateNoAI {
		t.Fatalf("state = %v, want %v", got.state, composeStateNoAI)
	}
	view := got.View()
	if !strings.Contains(view, "AI not configured") {
		t.Fatalf("expected AI warning in view, got: %s", view)
	}
}

func TestComposeNoAIEnterGoesHome(t *testing.T) {
	m := NewComposeModel(nil, nil, 80, "")
	m.state = composeStateNoAI

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	gotoMsg, ok := msg.(MsgGotoScreen)
	if !ok {
		t.Fatalf("expected MsgGotoScreen, got %T", msg)
	}
	if gotoMsg.Target != screenHome {
		t.Fatalf("target = %v, want %v", gotoMsg.Target, screenHome)
	}
}
