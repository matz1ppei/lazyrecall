package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

// buildBlankModel creates a BlankModel in blankStatePlaying with synthetic cards.
func buildBlankModel(cards []db.Card) BlankModel {
	m := NewBlankModel(nil)
	if len(cards) == 0 {
		m.state = blankStateEmpty
		return m
	}
	m.cards = cards
	m.current = 0
	m.state = blankStatePlaying
	m.input.Focus()
	return m
}

func makeCard(front, back, example, translation string) db.Card {
	return db.Card{
		Front:              front,
		Back:               back,
		Example:            example,
		ExampleTranslation: translation,
	}
}

func TestBlankEmpty(t *testing.T) {
	m := buildBlankModel(nil)
	if m.state != blankStateEmpty {
		t.Errorf("expected blankStateEmpty, got %v", m.state)
	}
}

func TestBlankCorrectAnswer(t *testing.T) {
	cards := []db.Card{makeCard("hola", "hello", "Hola mundo.", "Hello world.")}
	m := buildBlankModel(cards)

	// Type the correct answer and press enter
	m.input.SetValue("hola")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := m2.(BlankModel)

	if result.state != blankStateResult {
		t.Errorf("expected blankStateResult, got %v", result.state)
	}
	if !result.lastCorrect {
		t.Errorf("expected lastCorrect=true")
	}
	if result.correct != 1 {
		t.Errorf("expected correct=1, got %d", result.correct)
	}
}

func TestBlankAccentNormalization(t *testing.T) {
	cards := []db.Card{makeCard("canción", "song", "La canción es bonita.", "The song is pretty.")}
	m := buildBlankModel(cards)

	// Answer without accent should still be correct
	m.input.SetValue("cancion")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := m2.(BlankModel)

	if !result.lastCorrect {
		t.Errorf("expected accent-normalized answer to be correct")
	}
}

func TestBlankWrongAnswer(t *testing.T) {
	cards := []db.Card{makeCard("hola", "hello", "Hola mundo.", "Hello world.")}
	m := buildBlankModel(cards)

	m.input.SetValue("wrong")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := m2.(BlankModel)

	if result.lastCorrect {
		t.Errorf("expected lastCorrect=false")
	}
	if result.correct != 0 {
		t.Errorf("expected correct=0, got %d", result.correct)
	}
	if result.lastAnswer != "wrong" {
		t.Errorf("expected lastAnswer='wrong', got %q", result.lastAnswer)
	}
}

func TestBlankCtrlHShowsHint(t *testing.T) {
	cards := []db.Card{makeCard("hola", "hello", "Hola mundo.", "Hello world.")}
	m := buildBlankModel(cards)

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	result := m2.(BlankModel)

	if !result.showHint {
		t.Errorf("expected showHint=true after ctrl+h")
	}
}

func TestBlankComplete(t *testing.T) {
	cards := []db.Card{
		makeCard("hola", "hello", "Hola mundo.", "Hello world."),
	}
	m := buildBlankModel(cards)

	// Answer and advance past last card
	m.input.SetValue("hola")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Simulate timer reset
	m3, _ := m2.(BlankModel).Update(msgBlankResultReset{})
	result := m3.(BlankModel)

	if result.state != blankStateComplete {
		t.Errorf("expected blankStateComplete, got %v", result.state)
	}
}

func TestBlankEscGoesHome(t *testing.T) {
	cards := []db.Card{makeCard("hola", "hello", "Hola mundo.", "Hello world.")}
	m := buildBlankModel(cards)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a cmd for esc")
	}
	msg := cmd()
	gotoMsg, ok := msg.(MsgGotoScreen)
	if !ok {
		t.Fatalf("expected MsgGotoScreen, got %T", msg)
	}
	if gotoMsg.Target != screenHome {
		t.Errorf("expected screenHome, got %v", gotoMsg.Target)
	}
}
