package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

// makeCardFront builds a minimal db.Card with only a Front value set.
// Distinct from blank_test.go's makeCard which requires all four fields.
func makeCardFront(front string) db.Card {
	return db.Card{Front: front}
}

// buildBrainDumpModel returns a BrainDumpModel wired to a completion sentinel.
func buildBrainDumpModel(cards []db.Card) (BrainDumpModel, *bool) {
	fired := new(bool)
	onComplete := tea.Cmd(func() tea.Msg {
		*fired = true
		return msgSessionPhaseComplete{}
	})
	m := NewBrainDumpModel(cards, "Brain Dump 1", onComplete)
	return m, fired
}

// setInputValue directly sets the textinput value and returns the modified model.
// This is more reliable than simulating keystrokes in tests because textinput
// internal focus state can interfere with rune delivery.
func setInputValue(m BrainDumpModel, text string) BrainDumpModel {
	m.input.SetValue(text)
	return m
}

// submitInput presses Enter on the model to confirm the current input.
func submitInput(m BrainDumpModel) (BrainDumpModel, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(BrainDumpModel), cmd
}

// --- Tests ---

// TestBrainDumpCaseFoldMatch verifies that matching is case-insensitive.
func TestBrainDumpCaseFoldMatch(t *testing.T) {
	cards := []db.Card{makeCardFront("Hello"), makeCardFront("World")}
	count := scoreInput("hello, WORLD", cards)
	if count != 2 {
		t.Errorf("expected 2 case-insensitive matches, got %d", count)
	}
}

// TestBrainDumpTrimWhitespace verifies that tokens are trimmed before matching.
func TestBrainDumpTrimWhitespace(t *testing.T) {
	cards := []db.Card{makeCardFront("hello"), makeCardFront("world")}
	count := scoreInput("  hello ,  world  ", cards)
	if count != 2 {
		t.Errorf("expected 2 matches after whitespace trim, got %d", count)
	}
}

// TestBrainDumpZeroInput verifies that an empty or blank input scores 0.
func TestBrainDumpZeroInput(t *testing.T) {
	cards := []db.Card{makeCardFront("hello")}
	for _, raw := range []string{"", "   ", ",,,", "  ,  "} {
		count := scoreInput(raw, cards)
		if count != 0 {
			t.Errorf("scoreInput(%q) = %d, want 0", raw, count)
		}
	}
}

// TestBrainDumpNoDuplicateCount verifies that the same token repeated only counts once.
func TestBrainDumpNoDuplicateCount(t *testing.T) {
	cards := []db.Card{makeCardFront("hello")}
	count := scoreInput("hello, hello, Hello", cards)
	if count != 1 {
		t.Errorf("expected 1 (no double-counting), got %d", count)
	}
}

// TestBrainDumpResultViewNormal verifies the "N/M words recalled" message when not perfect.
func TestBrainDumpResultViewNormal(t *testing.T) {
	cards := []db.Card{makeCardFront("apple"), makeCardFront("banana"), makeCardFront("cherry")}
	m, _ := buildBrainDumpModel(cards)

	// Type only one matching word, then submit.
	m = setInputValue(m,"apple")
	m, _ = submitInput(m)

	view := m.View()
	// Should show partial score, not perfect message.
	if strings.Contains(view, "Perfect") {
		t.Error("expected partial score message, got Perfect")
	}
	if !strings.Contains(view, "1") || !strings.Contains(view, "3") {
		t.Errorf("View() should contain 1 and 3; got: %s", view)
	}
}

// TestBrainDumpPerfectMessage verifies the special congratulation when all words are recalled.
func TestBrainDumpPerfectMessage(t *testing.T) {
	cards := []db.Card{makeCardFront("apple"), makeCardFront("banana")}
	m, _ := buildBrainDumpModel(cards)

	m = setInputValue(m,"apple, banana")
	m, _ = submitInput(m)

	view := m.View()
	if !strings.Contains(view, "Perfect") {
		t.Errorf("expected 'Perfect' message for full recall; got: %s", view)
	}
}

// TestBrainDumpEnterInResultFiresOnComplete verifies that pressing Enter in the
// result state fires the onComplete command.
func TestBrainDumpEnterInResultFiresOnComplete(t *testing.T) {
	cards := []db.Card{makeCardFront("apple")}
	m, fired := buildBrainDumpModel(cards)

	// Transition to result state.
	m, _ = submitInput(m)

	// Press Enter to continue.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected onComplete cmd after Enter in result state, got nil")
	}
	result := cmd()
	if _, ok := result.(msgSessionPhaseComplete); !ok {
		t.Errorf("expected msgSessionPhaseComplete, got %T", result)
	}
	_ = fired // not used directly; cmd() call above covers it
}

// TestBrainDumpInputView verifies that the input screen shows the label and help text.
func TestBrainDumpInputView(t *testing.T) {
	cards := []db.Card{makeCardFront("apple")}
	m, _ := buildBrainDumpModel(cards)

	view := m.View()
	if !strings.Contains(view, "Brain Dump 1") {
		t.Errorf("View() missing label 'Brain Dump 1'; got: %s", view)
	}
	if !strings.Contains(view, "enter") {
		t.Errorf("View() missing help hint '[enter]'; got: %s", view)
	}
}
