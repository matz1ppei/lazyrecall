package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

// buildPlayingModel creates a MatchModel in matchStatePlaying with synthetic
// pairs. The right column is reversed so items are misaligned with the left.
func buildPlayingModel(pairs []struct {
	id          int64
	front, back string
}) MatchModel {
	m := MatchModel{
		state:        matchStatePlaying,
		startTime:    time.Now(),
		wrongCardIDs: make(map[int64]bool),
	}
	m.leftItems = make([]matchItem, len(pairs))
	m.rightItems = make([]matchItem, len(pairs))
	for i, p := range pairs {
		m.leftItems[i] = matchItem{cardID: p.id, text: p.front}
		m.rightItems[i] = matchItem{cardID: p.id, text: p.back}
	}
	// Reverse right column so index 0 left != index 0 right (for len > 1)
	for i, j := 0, len(m.rightItems)-1; i < j; i, j = i+1, j-1 {
		m.rightItems[i], m.rightItems[j] = m.rightItems[j], m.rightItems[i]
	}
	return m
}

// pressKey sends a rune key press.
func pressKey(m MatchModel, key string) MatchModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(MatchModel)
}

// pressEnter sends the Enter key.
func pressEnter(m MatchModel) MatchModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(MatchModel)
}

// pressMatchArrow sends a special navigation key (up/down/left/right/enter/esc).
func pressMatchArrow(m MatchModel, key string) MatchModel {
	var keyType tea.KeyType
	switch key {
	case "up":
		keyType = tea.KeyUp
	case "down":
		keyType = tea.KeyDown
	case "left":
		keyType = tea.KeyLeft
	case "right":
		keyType = tea.KeyRight
	case "enter":
		keyType = tea.KeyEnter
	case "esc":
		keyType = tea.KeyEscape
	}
	updated, _ := m.Update(tea.KeyMsg{Type: keyType})
	return updated.(MatchModel)
}

// selectLeft navigates left column cursor to idx and presses enter.
func selectLeft(m MatchModel, idx int) MatchModel {
	// ensure left column is active
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(MatchModel)
	for m.leftCursor < idx {
		m = pressMatchArrow(m, "down")
	}
	for m.leftCursor > idx {
		m = pressMatchArrow(m, "up")
	}
	return pressMatchArrow(m, "enter")
}

// selectRight navigates right column cursor to idx and presses enter.
func selectRight(m MatchModel, idx int) MatchModel {
	// switch to right column
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(MatchModel)
	for m.rightCursor < idx {
		m = pressMatchArrow(m, "down")
	}
	for m.rightCursor > idx {
		m = pressMatchArrow(m, "up")
	}
	return pressMatchArrow(m, "enter")
}

// ---------------------------------------------------------------------------

func TestMatchModel_NilCards_Empty(t *testing.T) {
	m := MatchModel{state: matchStateLoading}
	updated, _ := m.Update(msgMatchCards(nil))
	got := updated.(MatchModel)
	if got.state != matchStateEmpty {
		t.Errorf("expected matchStateEmpty, got %v", got.state)
	}
}

func TestMatchModel_OneCard_Empty(t *testing.T) {
	m := MatchModel{state: matchStateLoading}
	cards := msgMatchCards([]db.Card{{ID: 1, Front: "a", Back: "b"}})
	updated, _ := m.Update(cards)
	got := updated.(MatchModel)
	if got.state != matchStateEmpty {
		t.Errorf("expected matchStateEmpty for 1 card, got %v", got.state)
	}
}

func TestMatchModel_CorrectPair(t *testing.T) {
	// 4 pairs; right column reversed: right[0]=pair[3], right[1]=pair[2], ...
	pairs := []struct {
		id          int64
		front, back string
	}{
		{1, "hello", "hola"},
		{2, "dog", "perro"},
		{3, "cat", "gato"},
		{4, "house", "casa"},
	}
	m := buildPlayingModel(pairs)

	// Find which right index holds the card matching left[0]
	var rightIdx int
	for i, ri := range m.rightItems {
		if ri.cardID == m.leftItems[0].cardID {
			rightIdx = i
			break
		}
	}

	// Select left[0] (cursor already at 0, activeCol=0)
	m = pressMatchArrow(m, "enter")
	if m.selected == nil {
		t.Fatal("expected selection after pressing enter on left[0]")
	}

	// Select matching right item
	m = selectRight(m, rightIdx)

	if m.totalMatched != 1 {
		t.Errorf("expected totalMatched=1, got %d", m.totalMatched)
	}
	if m.selected != nil {
		t.Error("expected selection cleared after correct match")
	}
}

func TestMatchModel_WrongPair(t *testing.T) {
	pairs := []struct {
		id          int64
		front, back string
	}{
		{1, "hello", "hola"},
		{2, "dog", "perro"},
		{3, "cat", "gato"},
		{4, "house", "casa"},
	}
	m := buildPlayingModel(pairs)

	// Find a right index that does NOT match left[0]
	var wrongIdx int
	for i, ri := range m.rightItems {
		if ri.cardID != m.leftItems[0].cardID {
			wrongIdx = i
			break
		}
	}

	m = pressMatchArrow(m, "enter") // select left[0]
	m = selectRight(m, wrongIdx)

	if m.state != matchStateWrong {
		t.Errorf("expected matchStateWrong, got %v", m.state)
	}
	if m.mistakes != 1 {
		t.Errorf("expected mistakes=1, got %d", m.mistakes)
	}
}

func TestMatchModel_WrongReset(t *testing.T) {
	m := MatchModel{state: matchStateWrong, mistakes: 1}
	updated, _ := m.Update(msgMatchWrongReset{})
	got := updated.(MatchModel)
	if got.state != matchStatePlaying {
		t.Errorf("expected matchStatePlaying after reset, got %v", got.state)
	}
	if got.selected != nil {
		t.Error("expected selected cleared after wrong reset")
	}
}

func TestMatchModel_AllPairsComplete(t *testing.T) {
	pairs := []struct {
		id          int64
		front, back string
	}{
		{1, "a", "A"},
		{2, "b", "B"},
	}
	m := buildPlayingModel(pairs)

	// Helper: select the pair at leftItems[leftIdx]
	matchPair := func(m MatchModel, leftIdx int) MatchModel {
		m = selectLeft(m, leftIdx)
		var ri int
		for i, item := range m.rightItems {
			if item.cardID == m.leftItems[leftIdx].cardID {
				ri = i
				break
			}
		}
		return selectRight(m, ri)
	}

	m = matchPair(m, 0)
	if m.state == matchStateComplete {
		t.Errorf("should not complete after 1 of 2 pairs")
	}
	m = matchPair(m, 1)
	if m.state != matchStateComplete {
		t.Errorf("expected matchStateComplete after all pairs matched, got %v", m.state)
	}
}

func TestMatchModel_SameColumnMovesSelection(t *testing.T) {
	pairs := []struct {
		id          int64
		front, back string
	}{{1, "a", "A"}, {2, "b", "B"}}
	m := buildPlayingModel(pairs)

	// Select left[0] (cursor at 0, activeCol=0)
	m = pressMatchArrow(m, "enter")
	if m.selected == nil || m.selected.index != 0 {
		t.Fatal("expected selection at index 0")
	}

	// Move cursor down to left[1] and press enter — should move selection within same column
	m = pressMatchArrow(m, "down")
	m = pressMatchArrow(m, "enter")
	if m.selected == nil || m.selected.index != 1 {
		t.Errorf("expected selection moved to index 1, got %+v", m.selected)
	}
}

func TestMatchModel_BlockedDuringWrongFlash(t *testing.T) {
	pairs := []struct {
		id          int64
		front, back string
	}{{1, "a", "A"}, {2, "b", "B"}}
	m := buildPlayingModel(pairs)
	m.state = matchStateWrong

	// Key presses should be ignored during wrong flash (except esc)
	m = pressKey(m, "a")
	if m.selected != nil {
		t.Error("selection should not change during wrong flash")
	}
	if m.state != matchStateWrong {
		t.Errorf("state should remain matchStateWrong, got %v", m.state)
	}
}
