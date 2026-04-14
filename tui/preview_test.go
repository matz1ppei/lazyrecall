package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

func makeCardWithReview(front, back string) db.CardWithReview {
	return db.CardWithReview{
		Card: db.Card{Front: front, Back: back},
	}
}

func buildPreviewModel(cards []db.CardWithReview) (PreviewModel, bool) {
	completed := false
	onComplete := tea.Cmd(func() tea.Msg {
		completed = true
		return msgSessionPhaseComplete{}
	})
	m := NewPreviewModel(cards, onComplete)
	return m, completed
}

// TestPreviewEnterSkips verifies that pressing Enter immediately fires onComplete.
func TestPreviewEnterSkips(t *testing.T) {
	cards := []db.CardWithReview{
		makeCardWithReview("hello", "こんにちは"),
	}
	m, _ := buildPreviewModel(cards)

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterMsg)
	_ = updated

	if cmd == nil {
		t.Fatal("expected a cmd after Enter, got nil")
	}
	// Execute the returned cmd and verify it produces msgSessionPhaseComplete.
	result := cmd()
	if _, ok := result.(msgSessionPhaseComplete); !ok {
		t.Errorf("expected msgSessionPhaseComplete, got %T", result)
	}
}

// TestPreviewViewShowsCards verifies that View() renders both Front and Back of each card.
func TestPreviewViewShowsCards(t *testing.T) {
	cards := []db.CardWithReview{
		makeCardWithReview("apple", "りんご"),
		makeCardWithReview("banana", "バナナ"),
	}
	m, _ := buildPreviewModel(cards)

	view := m.View()
	for _, c := range cards {
		if !strings.Contains(view, c.Front) {
			t.Errorf("View() missing Front %q", c.Front)
		}
		if !strings.Contains(view, c.Back) {
			t.Errorf("View() missing Back %q", c.Back)
		}
	}
}

// TestPreviewTickAutoAdvances verifies that msgPreviewTick fires onComplete.
func TestPreviewTickAutoAdvances(t *testing.T) {
	cards := []db.CardWithReview{
		makeCardWithReview("cat", "ねこ"),
	}
	m, _ := buildPreviewModel(cards)

	_, cmd := m.Update(msgPreviewTick{})
	if cmd == nil {
		t.Fatal("expected cmd from tick, got nil")
	}
	result := cmd()
	if _, ok := result.(msgSessionPhaseComplete); !ok {
		t.Errorf("expected msgSessionPhaseComplete, got %T", result)
	}
}

// TestPreviewEscGoesHome verifies that Esc navigates to the Home screen.
func TestPreviewEscGoesHome(t *testing.T) {
	m, _ := buildPreviewModel(nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected cmd from esc, got nil")
	}
	result := cmd()
	msg, ok := result.(MsgGotoScreen)
	if !ok {
		t.Fatalf("expected MsgGotoScreen, got %T", result)
	}
	if msg.Target != screenHome {
		t.Errorf("expected screenHome, got %v", msg.Target)
	}
}
