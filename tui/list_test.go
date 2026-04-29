package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

func TestListEditCtrlXTogglesExcludedState(t *testing.T) {
	m := NewListModel(nil, nil)
	m.cards = []db.CardWithReview{{Card: db.Card{ID: 1, Front: "apple", Back: "りんご"}}}
	m.excluded = map[string]bool{"apple": true}
	m.initEditInputs(m.cards[0])
	m.state = listStateEdit

	view := m.View()
	if !strings.Contains(view, "Excluded: On") {
		t.Fatalf("expected excluded on in view, got: %s", view)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("expected no command for local toggle")
	}

	got := updated.(ListModel)
	if got.editExcluded {
		t.Fatal("expected editExcluded to toggle off")
	}
	if !strings.Contains(got.View(), "Excluded: Off") {
		t.Fatalf("expected excluded off in view, got: %s", got.View())
	}
}

func TestListEditSaveSyncsExcludedWordToEditedFront(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cardID, err := db.CreateCardWithReview(database, "apple", "りんご", "", "", "", "")
	if err != nil {
		t.Fatalf("CreateCardWithReview: %v", err)
	}
	if err := config.SetExcludedWord("apple", true); err != nil {
		t.Fatalf("SetExcludedWord(initial): %v", err)
	}

	cards, err := db.ListAllCardsWithReview(database)
	if err != nil {
		t.Fatalf("ListAllCardsWithReview: %v", err)
	}

	m := NewListModel(database, nil)
	m.cards = cards
	m.excluded = map[string]bool{"apple": true}
	m.cursor = 0
	m.initEditInputs(m.cards[0])
	m.state = listStateEdit
	m.editInputs[0].SetValue("banana")
	m.editInputs[4].SetValue("banana example translation")
	m.editInputs[5].SetValue("bananas")
	m.editFocus = 5

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd()
	done, ok := msg.(msgUpdateDone)
	if !ok {
		t.Fatalf("expected msgUpdateDone, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("save returned err: %v", done.err)
	}

	gotModel := updated.(ListModel)
	if gotModel.state != listStateLoading {
		t.Fatalf("state = %v, want %v", gotModel.state, listStateLoading)
	}

	card, err := db.GetCard(database, cardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Front != "banana" {
		t.Fatalf("card.Front = %q, want %q", card.Front, "banana")
	}
	if card.ExampleTranslation != "banana example translation" {
		t.Fatalf("card.ExampleTranslation = %q, want %q", card.ExampleTranslation, "banana example translation")
	}
	if card.ExampleWord != "bananas" {
		t.Fatalf("card.ExampleWord = %q, want %q", card.ExampleWord, "bananas")
	}

	excluded, err := config.LoadExcludedWords()
	if err != nil {
		t.Fatalf("LoadExcludedWords: %v", err)
	}
	if excluded["apple"] {
		t.Fatalf("expected old front to be removed from exclusion list: %v", excluded)
	}
	if !excluded["banana"] {
		t.Fatalf("expected new front to be excluded: %v", excluded)
	}
}

func TestListViewSearchFiltersByFrontAndBack(t *testing.T) {
	m := NewListModel(nil, nil)
	m.state = listStateNormal
	m.cards = []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "apple", Back: "りんご"}},
		{Card: db.Card{ID: 2, Front: "banana", Back: "黄色い果物"}},
		{Card: db.Card{ID: 3, Front: "cat", Back: "ねこ"}},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if cmd == nil {
		t.Fatal("expected focus command")
	}

	model := updated.(ListModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ね")})
	model = updated.(ListModel)
	view := model.View()
	if !strings.Contains(view, "cat") {
		t.Fatalf("expected matching card to remain visible, got: %s", view)
	}
	if strings.Contains(view, "apple") || strings.Contains(view, "banana") {
		t.Fatalf("expected non-matching cards to be hidden, got: %s", view)
	}
}

func TestListViewExcludedOnlyToggleFiltersRows(t *testing.T) {
	m := NewListModel(nil, nil)
	m.state = listStateNormal
	m.cards = []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "apple", Back: "りんご"}},
		{Card: db.Card{ID: 2, Front: "banana", Back: "バナナ"}},
	}
	m.excluded = map[string]bool{"banana": true}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	model := updated.(ListModel)
	view := model.View()
	if !strings.Contains(view, "banana") {
		t.Fatalf("expected excluded card to remain visible, got: %s", view)
	}
	if strings.Contains(view, "apple") {
		t.Fatalf("expected non-excluded card to be hidden, got: %s", view)
	}
}

func TestListViewDueOnlyToggleFiltersRows(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	m := NewListModel(nil, nil)
	m.state = listStateNormal
	m.cards = []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "apple", Back: "りんご"}, Review: db.Review{DueDate: today}},
		{Card: db.Card{ID: 2, Front: "banana", Back: "バナナ"}, Review: db.Review{DueDate: tomorrow}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	model := updated.(ListModel)
	view := model.View()
	if !strings.Contains(view, "apple") {
		t.Fatalf("expected due card to remain visible, got: %s", view)
	}
	if strings.Contains(view, "banana") {
		t.Fatalf("expected future card to be hidden, got: %s", view)
	}
}

func TestListViewSortCycleReordersRows(t *testing.T) {
	now := time.Now()
	m := NewListModel(nil, nil)
	m.state = listStateNormal
	m.cards = []db.CardWithReview{
		{Card: db.Card{ID: 1, Front: "banana", CreatedAt: now.Add(-2 * time.Hour)}, Review: db.Review{DueDate: "2026-04-26"}},
		{Card: db.Card{ID: 2, Front: "apple", CreatedAt: now}, Review: db.Review{DueDate: "2026-04-25"}},
		{Card: db.Card{ID: 3, Front: "cherry", CreatedAt: now.Add(-1 * time.Hour)}, Review: db.Review{DueDate: "2026-04-24"}},
	}

	view := m.View()
	if !strings.Contains(view, "Sort: new") || !strings.Contains(view, "> 2") {
		t.Fatalf("expected new sort to start with newest card 2, got: %s", view)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model := updated.(ListModel)
	view = model.View()
	if !strings.Contains(view, "Sort: due") || !strings.Contains(view, "> 3") {
		t.Fatalf("expected due sort to start with card 3, got: %s", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model = updated.(ListModel)
	view = model.View()
	if !strings.Contains(view, "Sort: front") || !strings.Contains(view, "> 2") {
		t.Fatalf("expected front sort to start with card 2, got: %s", view)
	}
}

func TestListSortCycleResetsPagerToFirstPage(t *testing.T) {
	m := NewListModel(nil, nil)
	m.state = listStateNormal
	for i := 0; i < 20; i++ {
		m.cards = append(m.cards, db.CardWithReview{
			Card:   db.Card{ID: int64(i + 1), Front: "card"},
			Review: db.Review{DueDate: "2026-04-24"},
		})
	}
	m.cursor = 16
	m.offset = 15

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model := updated.(ListModel)
	if model.cursor != 0 || model.offset != 0 {
		t.Fatalf("expected pager reset to first page, got cursor=%d offset=%d", model.cursor, model.offset)
	}
}

func TestListEditViewShowsExpandedExampleFields(t *testing.T) {
	m := NewListModel(nil, nil)
	m.cards = []db.CardWithReview{{
		Card: db.Card{
			ID:                 1,
			Front:              "apple",
			Back:               "りんご",
			Example:            "I ate an apple.",
			ExampleTranslation: "りんごを食べた。",
			ExampleWord:        "apple",
		},
	}}
	m.initEditInputs(m.cards[0])
	m.state = listStateEdit

	view := m.View()
	if !strings.Contains(view, "Example Translation:") {
		t.Fatalf("expected Example Translation field, got: %s", view)
	}
	if !strings.Contains(view, "Example Word:") {
		t.Fatalf("expected Example Word field, got: %s", view)
	}
}

func TestSuspiciousListViewShowsReasonColumn(t *testing.T) {
	m := NewSuspiciousListModel(nil, nil)
	m.state = listStateNormal
	m.cards = []db.CardWithReview{{
		Card: db.Card{
			ID:          1,
			Front:       "conmigo",
			Back:        "with me",
			Example:     "Por favor, acompañadme hasta la siguiente parada.",
			ExampleWord: "acompañadme",
		},
	}}
	m.suspiciousReasons = map[int64]string{1: "example word mismatch; front missing in example"}

	view := m.View()
	if !strings.Contains(view, "Suspicious Cards") {
		t.Fatalf("expected suspicious title, got: %s", view)
	}
	if !strings.Contains(view, "Reason") {
		t.Fatalf("expected reason column, got: %s", view)
	}
	if !strings.Contains(view, "example word mismatch") {
		t.Fatalf("expected suspicious reason, got: %s", view)
	}
}

func TestSuspiciousListEditFixRemovesCardAfterReload(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = db.CreateCardWithReview(database, "conmigo", "with me", "", "Por favor, acompañadme hasta la siguiente parada.", "", "acompañadme")
	if err != nil {
		t.Fatalf("CreateCardWithReview: %v", err)
	}

	m := NewSuspiciousListModel(database, nil)
	initMsg := m.reloadCmd()()
	updated, _ := m.Update(initMsg)
	model := updated.(ListModel)
	if len(model.cards) != 1 {
		t.Fatalf("expected 1 suspicious card, got %d", len(model.cards))
	}

	model.initEditInputs(model.cards[0])
	model.state = listStateEdit
	model.editInputs[3].SetValue("Ven conmigo a la estación.")
	model.editInputs[5].SetValue("conmigo")
	model.editFocus = 5

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected save command")
	}
	doneMsg := cmd()
	done, ok := doneMsg.(msgUpdateDone)
	if !ok {
		t.Fatalf("expected msgUpdateDone, got %T", doneMsg)
	}
	if done.err != nil {
		t.Fatalf("save returned err: %v", done.err)
	}

	afterSave := updated.(ListModel)
	reloadMsg := afterSave.reloadCmd()()
	updated, _ = afterSave.Update(reloadMsg)
	finalModel := updated.(ListModel)
	if finalModel.state != listStateEmpty {
		t.Fatalf("expected suspicious list to become empty, got state %v", finalModel.state)
	}
	if !strings.Contains(finalModel.View(), "No suspicious cards found.") {
		t.Fatalf("expected empty suspicious message, got: %s", finalModel.View())
	}
}
