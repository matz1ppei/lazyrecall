package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

func TestAddModelAutoGeneratesFromWordAfterFrontEntry(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	mockAI := &ai.MockClient{
		CardBack: "dog",
		CardHint: "pet",
	}
	m := NewAddModel(database, mockAI, config.Config{AutoAdd: config.AutoAdd{LangName: "Spanish"}})
	m.inputs[0].SetValue("perro")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(AddModel)
	if !got.loading {
		t.Fatal("expected loading while checking duplicates")
	}
	if cmd == nil {
		t.Fatal("expected duplicate-check command")
	}

	msg := cmd()
	updated, cmd = got.Update(msg)
	got = updated.(AddModel)
	if !got.loading {
		t.Fatal("expected loading while generating card details")
	}
	if cmd == nil {
		t.Fatal("expected generation command after duplicate check")
	}

	msg = cmd()
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
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	m := NewAddModel(database, nil, config.Config{})
	m.inputs[0].SetValue("perro")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(AddModel)

	if !got.loading {
		t.Fatal("expected loading while checking duplicates")
	}
	if cmd == nil {
		t.Fatal("expected duplicate-check command")
	}

	msg := cmd()
	updated, cmd = got.Update(msg)
	got = updated.(AddModel)
	if got.step != stepBack {
		t.Fatalf("step = %v, want back", got.step)
	}
	if got.inputs[1].Value() != "" {
		t.Fatalf("expected empty meaning input, got %q", got.inputs[1].Value())
	}
}

func TestAddModelCtrlGGeneratesDetailsForKnownMeaning(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	mockAI := &ai.MockClient{
		CardHint: "memory hook",
	}
	m := NewAddModel(database, mockAI, config.Config{AutoAdd: config.AutoAdd{LangName: "Spanish"}})
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

func TestAddModelDuplicateContinueAlsoGeneratesFromWord(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	mockAI := &ai.MockClient{
		CardBack: "dog",
	}
	m := NewAddModel(database, mockAI, config.Config{AutoAdd: config.AutoAdd{LangName: "Spanish"}})
	m.dupWarning = true
	m.dupCards = []db.Card{{Front: "perro", Back: "dog"}}
	m.inputs[0].SetValue("perro")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got := updated.(AddModel)
	if !got.loading {
		t.Fatal("expected loading while generating after duplicate continue")
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
}

func TestAddModelWithAIAndNoLanguageFallsBackToMeaningInput(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	m := NewAddModel(database, &ai.MockClient{}, config.Config{})
	m.inputs[0].SetValue("patineta")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(AddModel)
	if !got.loading || cmd == nil {
		t.Fatal("expected duplicate-check command")
	}

	msg := cmd()
	updated, cmd = got.Update(msg)
	got = updated.(AddModel)
	if got.step != stepBack {
		t.Fatalf("step = %v, want back", got.step)
	}
}
