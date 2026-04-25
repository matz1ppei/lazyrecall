package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
)

func TestAddModelAutoGeneratesFromWordWhenMeaningEmpty(t *testing.T) {
	mockAI := &ai.MockClient{
		CardBack: "dog",
		CardHint: "pet",
	}
	m := NewAddModel(nil, mockAI)
	m.inputs[0].SetValue("perro")
	m.step = stepBack

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(AddModel)
	if !got.loading {
		t.Fatal("expected loading while generating card details")
	}
	if cmd == nil {
		t.Fatal("expected generation command")
	}

	msg := cmd()
	updated, _ = got.Update(msg)
	got = updated.(AddModel)

	if got.step != stepConfirm {
		t.Fatalf("step = %v, want confirm", got.step)
	}
	if got.inputs[1].Value() != "dog" {
		t.Fatalf("back = %q, want dog", got.inputs[1].Value())
	}
	if got.inputs[2].Value() != "pet" {
		t.Fatalf("hint = %q, want pet", got.inputs[2].Value())
	}
}

func TestAddModelRequiresMeaningWithoutAI(t *testing.T) {
	m := NewAddModel(nil, nil)
	m.inputs[0].SetValue("perro")
	m.step = stepBack

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(AddModel)

	if cmd != nil {
		t.Fatal("expected no command without AI")
	}
	if !strings.Contains(got.status, "Meaning cannot be empty without AI") {
		t.Fatalf("unexpected status: %s", got.status)
	}
}

func TestAddModelCtrlGGeneratesDetailsForKnownMeaning(t *testing.T) {
	mockAI := &ai.MockClient{
		CardHint: "memory hook",
	}
	m := NewAddModel(nil, mockAI)
	m.inputs[0].SetValue("perro")
	m.inputs[1].SetValue("dog")
	m.step = stepBack

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	got := updated.(AddModel)
	if !got.loading {
		t.Fatal("expected loading while generating details")
	}
	if cmd == nil {
		t.Fatal("expected generation command")
	}

	msg := cmd()
	updated, _ = got.Update(msg)
	got = updated.(AddModel)
	if got.inputs[1].Value() != "dog" {
		t.Fatalf("back = %q, want dog", got.inputs[1].Value())
	}
	if got.inputs[2].Value() != "memory hook" {
		t.Fatalf("hint = %q, want memory hook", got.inputs[2].Value())
	}
	if got.step != stepConfirm {
		t.Fatalf("step = %v, want confirm", got.step)
	}
}
