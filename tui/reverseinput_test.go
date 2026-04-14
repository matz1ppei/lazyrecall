package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

func makeReverseInputCards() []db.CardWithReview {
	return []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "hola", Back: "hello"}, Review: db.Review{ID: 1, CardID: 1}},
		{Card: db.Card{ID: 2, Front: "perro", Back: "dog"}, Review: db.Review{ID: 2, CardID: 2}},
		{Card: db.Card{ID: 3, Front: "gato", Back: "cat"}, Review: db.Review{ID: 3, CardID: 3}},
	}
}

func buildReverseInputModel(cards []db.CardWithReview) ReverseInputModel {
	m := ReverseInputModel{
		state:       reverseInputQuestion,
		cards:       cards,
		index:       0,
		sessionMode: true,
		input:       newReverseTextInput(),
	}
	return m
}

func pressReverseInputKey(m ReverseInputModel, key string) ReverseInputModel {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	} else if key == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	updated, _ := m.Update(msg)
	return updated.(ReverseInputModel)
}

func TestReverseInputViewShowsBack(t *testing.T) {
	cards := makeReverseInputCards()
	m := buildReverseInputModel(cards)

	view := m.View()
	// The question shows card.Back (the meaning)
	if !strings.Contains(view, "hello") {
		t.Errorf("expected Back 'hello' in reverse input view, got:\n%s", view)
	}
	if !strings.Contains(view, "Reverse Review") {
		t.Errorf("expected 'Reverse Review' title, got:\n%s", view)
	}
}

func TestReverseInputCorrectAnswer(t *testing.T) {
	cards := makeReverseInputCards()
	m := buildReverseInputModel(cards)

	// Simulate typing the correct answer
	m.input.SetValue("hola")

	m = pressReverseInputKey(m, "enter")

	if !m.lastCorrect {
		t.Error("expected lastCorrect = true for exact match")
	}
	if len(m.correctIDs) != 1 || m.correctIDs[0] != cards[0].Card.ID {
		t.Errorf("expected correctIDs = [%d], got %v", cards[0].Card.ID, m.correctIDs)
	}
	if m.state != reverseInputResult {
		t.Errorf("expected state reverseInputResult, got %v", m.state)
	}
}

func TestReverseInputWrongAnswer(t *testing.T) {
	cards := makeReverseInputCards()
	m := buildReverseInputModel(cards)

	m.input.SetValue("perro") // wrong answer

	m = pressReverseInputKey(m, "enter")

	if m.lastCorrect {
		t.Error("expected lastCorrect = false for wrong answer")
	}
	if len(m.correctIDs) != 0 {
		t.Errorf("expected correctIDs empty, got %v", m.correctIDs)
	}
}

func TestReverseInputAccentInsensitive(t *testing.T) {
	cards := []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "café", Back: "coffee"}, Review: db.Review{ID: 1, CardID: 1}},
	}
	m := buildReverseInputModel(cards)

	// Type without accent
	m.input.SetValue("cafe")

	m = pressReverseInputKey(m, "enter")

	if !m.lastCorrect {
		t.Error("expected lastCorrect = true for accent-insensitive match")
	}
}

func TestReverseInputAdvancesOnReset(t *testing.T) {
	cards := makeReverseInputCards()
	m := buildReverseInputModel(cards)
	m.input.SetValue("hola")
	m = pressReverseInputKey(m, "enter")

	// Simulate the auto-advance timer firing
	updated, _ := m.Update(msgReverseInputResultReset{})
	m = updated.(ReverseInputModel)

	if m.index != 1 {
		t.Errorf("expected index=1 after reset, got %d", m.index)
	}
	if m.state != reverseInputQuestion {
		t.Errorf("expected reverseInputQuestion after reset, got %v", m.state)
	}
}

func TestReverseInputSummaryAfterAllCards(t *testing.T) {
	cards := makeReverseInputCards()
	m := buildReverseInputModel(cards)
	m.index = len(cards) - 1 // last card
	m.input.SetValue("gato")
	m = pressReverseInputKey(m, "enter")

	// Simulate timer
	updated, _ := m.Update(msgReverseInputResultReset{})
	m = updated.(ReverseInputModel)

	if m.state != reverseInputSummary {
		t.Errorf("expected reverseInputSummary, got %v", m.state)
	}
}

func TestReverseInputOnCompleteCalledFromSummary(t *testing.T) {
	cards := makeReverseInputCards()
	called := false
	m := buildReverseInputModel(cards)
	m.onComplete = func() tea.Msg { called = true; return nil }
	m.state = reverseInputSummary

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from summary enter")
	}
	cmd() // invoke the command
	if !called {
		t.Error("expected onComplete to be called")
	}
}
