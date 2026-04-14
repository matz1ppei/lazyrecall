package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

func makeReviewCards() []db.CardWithReview {
	return []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "hola", Back: "hello"}, Review: db.Review{ID: 1, CardID: 1, Interval: 1, EaseFactor: 2.5}},
		{Card: db.Card{ID: 2, Front: "perro", Back: "dog"}, Review: db.Review{ID: 2, CardID: 2, Interval: 1, EaseFactor: 2.5}},
		{Card: db.Card{ID: 3, Front: "gato", Back: "cat"}, Review: db.Review{ID: 3, CardID: 3, Interval: 1, EaseFactor: 2.5}},
	}
}

func buildReviewModel(cards []db.CardWithReview) ReviewModel {
	m := ReviewModel{
		state:       reviewStateQuestion,
		cards:       cards,
		choices:     []string{"hello", "dog", "cat"},
		correctIndex: 0,
		cursorIndex: 0,
		sessionMode: true, // no DB calls
	}
	return m
}

func pressReviewKey(m ReviewModel, key string) ReviewModel {
	var keyType tea.KeyType
	var runes []rune
	switch key {
	case "enter":
		keyType = tea.KeyEnter
	case "esc":
		keyType = tea.KeyEscape
	default:
		keyType = tea.KeyRunes
		runes = []rune(key)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: keyType, Runes: runes})
	return updated.(ReviewModel)
}

func TestReviewCursorMovement(t *testing.T) {
	m := buildReviewModel(makeReviewCards())

	// j moves down
	m = pressReviewKey(m, "j")
	if m.cursorIndex != 1 {
		t.Errorf("expected cursorIndex 1, got %d", m.cursorIndex)
	}

	// k moves up
	m = pressReviewKey(m, "k")
	if m.cursorIndex != 0 {
		t.Errorf("expected cursorIndex 0, got %d", m.cursorIndex)
	}

	// j wraps at bottom
	m.cursorIndex = len(m.choices) - 1
	m = pressReviewKey(m, "j")
	if m.cursorIndex != 0 {
		t.Errorf("expected wrap to 0, got %d", m.cursorIndex)
	}

	// k wraps at top
	m.cursorIndex = 0
	m = pressReviewKey(m, "k")
	if m.cursorIndex != len(m.choices)-1 {
		t.Errorf("expected wrap to %d, got %d", len(m.choices)-1, m.cursorIndex)
	}
}

func TestReviewCorrectAnswer(t *testing.T) {
	cards := makeReviewCards()
	m := buildReviewModel(cards)
	m.cursorIndex = m.correctIndex // select correct

	m = pressReviewKey(m, "enter")

	if !m.lastCorrect {
		t.Error("expected lastCorrect = true")
	}
	if m.state != reviewStateResult {
		t.Errorf("expected reviewStateResult, got %v", m.state)
	}
	if len(m.correctIDs) != 1 || m.correctIDs[0] != cards[0].Card.ID {
		t.Errorf("expected correctIDs = [%d], got %v", cards[0].Card.ID, m.correctIDs)
	}
}

func TestReviewWrongAnswer(t *testing.T) {
	cards := makeReviewCards()
	m := buildReviewModel(cards)
	m.cursorIndex = m.correctIndex + 1 // wrong choice

	m = pressReviewKey(m, "enter")

	if m.lastCorrect {
		t.Error("expected lastCorrect = false")
	}
	if len(m.correctIDs) != 0 {
		t.Errorf("expected correctIDs empty, got %v", m.correctIDs)
	}
}

func TestReviewResultResetAdvancesCard(t *testing.T) {
	cards := makeReviewCards()
	m := buildReviewModel(cards)
	m.state = reviewStateResult

	updated, _ := m.Update(msgReviewResultReset{})
	m = updated.(ReviewModel)

	if m.index != 1 {
		t.Errorf("expected index 1, got %d", m.index)
	}
	if m.reviewed != 1 {
		t.Errorf("expected reviewed 1, got %d", m.reviewed)
	}
}

func TestReviewResultResetCompletesSession(t *testing.T) {
	cards := makeReviewCards()
	m := buildReviewModel(cards)
	m.index = len(cards) - 1
	m.state = reviewStateResult
	m.ignoreLimit = true

	updated, _ := m.Update(msgReviewResultReset{})
	m = updated.(ReviewModel)

	if m.state != reviewStateSummary {
		t.Errorf("expected reviewStateSummary, got %v", m.state)
	}
}

func TestReviewSessionModeNoRateCard(t *testing.T) {
	// sessionMode=true: entering answer must NOT call rateCard (no DB).
	// Verify by using nil db — if rateCard were called it would panic on nil.
	cards := makeReviewCards()
	m := ReviewModel{
		db:          nil, // intentionally nil; rateCard would panic
		state:       reviewStateQuestion,
		cards:       cards,
		choices:     []string{"hello", "dog", "cat"},
		correctIndex: 0,
		cursorIndex: 0,
		sessionMode: true,
	}

	// Should not panic
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ReviewModel)

	if m.state != reviewStateResult {
		t.Errorf("expected reviewStateResult, got %v", m.state)
	}

	// cmd should be a single tick (not a Batch with rateCard)
	// We can verify it's a valid Cmd that eventually returns msgReviewResultReset
	if cmd == nil {
		t.Error("expected non-nil cmd (tick)")
	}

	// Execute the cmd in a controlled way to verify it returns msgReviewResultReset
	// (We can't easily wait 1.5s in a test, so just confirm no panic occurred)
	_ = time.Now() // test passed if no panic above
}


func TestTripleCorrectContainsID(t *testing.T) {
	ids := []int64{1, 3, 5}

	if !containsID(ids, 3) {
		t.Error("expected containsID(ids, 3) = true")
	}
	if containsID(ids, 2) {
		t.Error("expected containsID(ids, 2) = false")
	}
	if containsID(nil, 1) {
		t.Error("expected containsID(nil, 1) = false")
	}
}
