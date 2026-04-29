package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

func TestSessionInitResumesSavedPhaseAndCardOrder(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", homeDir+"/.config")

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	id1, _ := db.CreateCardWithReview(database, "alpha", "A", "", "", "", "")
	id2, _ := db.CreateCardWithReview(database, "beta", "B", "", "", "", "")
	id3, _ := db.CreateCardWithReview(database, "gamma", "C", "", "", "", "")

	startedAt := time.Date(2026, 4, 24, 10, 0, 0, 0, time.Local)
	if err := config.SaveDailySessionSnapshot(config.DailySessionSnapshot{
		Date:             time.Now().Format("2006-01-02"),
		CardIDs:          []int64{id2, id1, id3},
		ReviewSessionID:  77,
		Phase:            "match",
		ReviewCorrectIDs: []int64{id2, id1},
		MatchWrongIDs:    []int64{id3},
		StartedAt:        startedAt,
	}); err != nil {
		t.Fatalf("SaveDailySessionSnapshot: %v", err)
	}

	model := NewSessionModel(database, nil)
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}
	msg := cmd()
	ready, ok := msg.(msgSessionReady)
	if !ok {
		t.Fatalf("expected msgSessionReady, got %T", msg)
	}
	if !ready.resumed {
		t.Fatal("expected resumed session")
	}
	if ready.phase != sessionPhaseMatch {
		t.Fatalf("phase = %v, want %v", ready.phase, sessionPhaseMatch)
	}
	if ready.reviewSessionID != 77 {
		t.Fatalf("reviewSessionID = %d, want 77", ready.reviewSessionID)
	}
	if len(ready.cards) != 3 || ready.cards[0].Card.ID != id2 || ready.cards[1].Card.ID != id1 || ready.cards[2].Card.ID != id3 {
		t.Fatalf("unexpected card order: %+v", ready.cards)
	}
	if len(ready.reviewCorrectIDs) != 2 || ready.reviewCorrectIDs[0] != id2 || ready.reviewCorrectIDs[1] != id1 {
		t.Fatalf("unexpected reviewCorrectIDs: %+v", ready.reviewCorrectIDs)
	}
	if !ready.startedAt.Equal(startedAt) {
		t.Fatalf("startedAt = %v, want %v", ready.startedAt, startedAt)
	}
}

func TestSessionMsgMarkDoneClearsSnapshot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", homeDir+"/.config")

	if err := config.SaveDailySessionSnapshot(config.DailySessionSnapshot{
		Date:    time.Now().Format("2006-01-02"),
		CardIDs: []int64{1},
		Phase:   "review",
	}); err != nil {
		t.Fatalf("SaveDailySessionSnapshot: %v", err)
	}

	model := NewSessionModel(nil, nil)
	model.cards = []db.CardWithReview{{Card: db.Card{ID: 1}}}

	updated, _ := model.Update(msgMarkDone{})
	got := updated.(SessionModel)
	if got.phase != sessionPhaseDone {
		t.Fatalf("phase = %v, want %v", got.phase, sessionPhaseDone)
	}

	snapshot, err := config.LoadDailySessionSnapshot()
	if err != nil {
		t.Fatalf("LoadDailySessionSnapshot: %v", err)
	}
	if snapshot.Date != "" || len(snapshot.CardIDs) != 0 {
		t.Fatalf("expected snapshot to be cleared, got %+v", snapshot)
	}
}

func TestSessionInitUsesCompletedDailySessionsForDayNumber(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := db.CreateCardWithReview(database, "alpha", "A", "", "", "", ""); err != nil {
		t.Fatalf("CreateCardWithReview: %v", err)
	}
	completedID, err := db.StartReviewSession(database, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession(completed): %v", err)
	}
	if _, err := database.Exec(`UPDATE review_sessions SET ended_at = datetime('now') WHERE id = ?`, completedID); err != nil {
		t.Fatalf("UPDATE completed session: %v", err)
	}
	if _, err := db.StartReviewSession(database, "daily_session", 9); err != nil {
		t.Fatalf("StartReviewSession(incomplete): %v", err)
	}

	model := NewSessionModel(database, nil)
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}
	msg := cmd()
	ready, ok := msg.(msgSessionReady)
	if !ok {
		t.Fatalf("expected msgSessionReady, got %T", msg)
	}
	if ready.daySessionNo != 2 {
		t.Fatalf("daySessionNo = %d, want 2", ready.daySessionNo)
	}
}

func TestSessionDoneViewShowsMinimumReached(t *testing.T) {
	model := NewSessionModel(nil, nil)
	model.phase = sessionPhaseDone
	model.cards = []db.CardWithReview{{Card: db.Card{ID: 1}}}
	model.reviewDone = true
	model.matchDone = true
	model.reverseReviewDone = true
	model.blankDone = true
	model.daySessionNo = 1

	view := model.View()
	if !strings.Contains(view, "Minimum reached! Today's Daily Session is done.") {
		t.Fatalf("expected minimum reached message, got: %s", view)
	}
}

func TestSessionDoneViewShowsIdealReached(t *testing.T) {
	model := NewSessionModel(nil, nil)
	model.phase = sessionPhaseDone
	model.cards = []db.CardWithReview{{Card: db.Card{ID: 1}}}
	model.reviewDone = true
	model.matchDone = true
	model.reverseReviewDone = true
	model.blankDone = true
	model.daySessionNo = 2

	view := model.View()
	if !strings.Contains(view, "Ideal reached! Daily Session 2 / 2 complete.") {
		t.Fatalf("expected ideal reached message, got: %s", view)
	}
}
