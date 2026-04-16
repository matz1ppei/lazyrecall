package db

import (
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateGetCard(t *testing.T) {
	db := openTestDB(t)

	id, err := CreateCard(db, "front1", "back1", "hint1", "", "", "")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	card, err := GetCard(db, id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Front != "front1" || card.Back != "back1" || card.Hint != "hint1" {
		t.Errorf("unexpected card: %+v", card)
	}
}

func TestListCards(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 3; i++ {
		if _, err := CreateCard(db, "front", "back", "", "", "", ""); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}

	cards, err := ListCards(db)
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if len(cards) != 3 {
		t.Errorf("expected 3 cards, got %d", len(cards))
	}
}

func TestCountCards(t *testing.T) {
	database := openTestDB(t)

	// Empty DB must return 0.
	count, err := CountCards(database)
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Insert 3 cards and verify count.
	for i := 0; i < 3; i++ {
		if _, err := CreateCard(database, fmt.Sprintf("f%d", i), "back", "", "", "", ""); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}
	count, err = CountCards(database)
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestDeleteCard(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "front", "back", "", "", "", "")
	if err := DeleteCard(db, id); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := GetCard(db, id); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestGetOrCreateReviewIdempotent(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "front", "back", "", "", "", "")

	r1, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("first GetOrCreateReview: %v", err)
	}
	r2, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("second GetOrCreateReview: %v", err)
	}
	if r1.ID != r2.ID {
		t.Errorf("expected same review row, got ids %d and %d", r1.ID, r2.ID)
	}
}

func TestListRandomCards(t *testing.T) {
	t.Run("zero cards returns empty slice", func(t *testing.T) {
		d := openTestDB(t)
		cards, err := ListRandomCards(d, 4)
		if err != nil {
			t.Fatalf("ListRandomCards: %v", err)
		}
		if len(cards) != 0 {
			t.Errorf("expected 0 cards, got %d", len(cards))
		}
	})

	t.Run("fewer cards than requested returns all", func(t *testing.T) {
		d := openTestDB(t)
		for i := 0; i < 2; i++ {
			if _, err := CreateCard(d, fmt.Sprintf("f%d", i), "b", "", "", "", ""); err != nil {
				t.Fatalf("CreateCard: %v", err)
			}
		}
		cards, err := ListRandomCards(d, 4)
		if err != nil {
			t.Fatalf("ListRandomCards: %v", err)
		}
		if len(cards) != 2 {
			t.Errorf("expected 2 cards, got %d", len(cards))
		}
	})

	t.Run("more cards than requested returns exactly n", func(t *testing.T) {
		d := openTestDB(t)
		for i := 0; i < 10; i++ {
			if _, err := CreateCard(d, fmt.Sprintf("f%d", i), "b", "", "", "", ""); err != nil {
				t.Fatalf("CreateCard: %v", err)
			}
		}
		cards, err := ListRandomCards(d, 4)
		if err != nil {
			t.Fatalf("ListRandomCards: %v", err)
		}
		if len(cards) != 4 {
			t.Errorf("expected 4 cards, got %d", len(cards))
		}
	})
}

func TestListDueCards(t *testing.T) {
	db := openTestDB(t)

	// Card due today (default due_date = date('now'))
	id1, _ := CreateCard(db, "due", "back", "", "", "", "")
	_, _ = GetOrCreateReview(db, id1)

	// Card due tomorrow — update due_date to future
	id2, _ := CreateCard(db, "future", "back", "", "", "", "")
	r2, _ := GetOrCreateReview(db, id2)
	r2.DueDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if err := UpdateReview(db, r2); err != nil {
		t.Fatalf("UpdateReview: %v", err)
	}

	due, err := ListDueCards(db, 20)
	if err != nil {
		t.Fatalf("ListDueCards: %v", err)
	}
	if len(due) != 1 {
		t.Errorf("expected 1 due card, got %d", len(due))
	}
	if due[0].Front != "due" {
		t.Errorf("expected 'due', got %q", due[0].Front)
	}
}

func TestCreateCardWithTranslation(t *testing.T) {
	db := openTestDB(t)

	id, err := CreateCard(db, "hola", "hello", "", "Hola mundo.", "Hello world.", "")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	card, err := GetCard(db, id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Example != "Hola mundo." {
		t.Errorf("expected example 'Hola mundo.', got %q", card.Example)
	}
	if card.ExampleTranslation != "Hello world." {
		t.Errorf("expected translation 'Hello world.', got %q", card.ExampleTranslation)
	}
}

func TestListCardsWithTranslation(t *testing.T) {
	db := openTestDB(t)

	CreateCard(db, "hola", "hello", "", "Hola mundo.", "Hello world.", "")
	CreateCard(db, "perro", "dog", "", "El perro corre.", "", "")
	CreateCard(db, "gato", "cat", "", "", "", "")

	cards, err := ListCardsWithTranslation(db)
	if err != nil {
		t.Fatalf("ListCardsWithTranslation: %v", err)
	}
	if len(cards) != 1 {
		t.Errorf("expected 1 card with translation, got %d", len(cards))
	}
	if cards[0].Front != "hola" {
		t.Errorf("expected 'hola', got %q", cards[0].Front)
	}
}

func TestListCardsNeedingTranslation(t *testing.T) {
	db := openTestDB(t)

	CreateCard(db, "hola", "hello", "", "Hola mundo.", "Hello world.", "")
	CreateCard(db, "perro", "dog", "", "El perro corre.", "", "")
	CreateCard(db, "gato", "cat", "", "", "", "")

	cards, err := ListCardsNeedingTranslation(db)
	if err != nil {
		t.Fatalf("ListCardsNeedingTranslation: %v", err)
	}
	if len(cards) != 1 {
		t.Errorf("expected 1 card needing translation, got %d", len(cards))
	}
	if cards[0].Front != "perro" {
		t.Errorf("expected 'perro', got %q", cards[0].Front)
	}
}

func TestUpdateCardTranslation(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "perro", "dog", "", "El perro corre.", "", "")
	if err := UpdateCardTranslation(db, id, "The dog runs."); err != nil {
		t.Fatalf("UpdateCardTranslation: %v", err)
	}

	card, err := GetCard(db, id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.ExampleTranslation != "The dog runs." {
		t.Errorf("expected 'The dog runs.', got %q", card.ExampleTranslation)
	}
}

func TestDailySessionCRUD(t *testing.T) {
	db := openTestDB(t)

	// Initially no session
	s, err := GetTodaySession(db)
	if err != nil {
		t.Fatalf("GetTodaySession: %v", err)
	}
	if s.ReviewDone || s.MatchDone || s.ReverseDone || s.BlankDone {
		t.Error("expected all false on empty session")
	}

	// Mark review done
	if err := MarkReviewDone(db); err != nil {
		t.Fatalf("MarkReviewDone: %v", err)
	}
	s, _ = GetTodaySession(db)
	if !s.ReviewDone {
		t.Error("expected ReviewDone = true")
	}
	if s.MatchDone || s.ReverseDone || s.BlankDone {
		t.Error("expected MatchDone, ReverseDone and BlankDone still false")
	}

	// Mark all done (idempotent)
	MarkMatchDone(db)
	MarkBlankDone(db)
	MarkReviewDone(db) // idempotent
	s, _ = GetTodaySession(db)
	if !s.ReviewDone || !s.MatchDone || !s.BlankDone {
		t.Errorf("expected all true: %+v", s)
	}
}

func TestMarkReverseDone(t *testing.T) {
	db := openTestDB(t)

	// Initially false
	s, err := GetTodaySession(db)
	if err != nil {
		t.Fatalf("GetTodaySession: %v", err)
	}
	if s.ReverseDone {
		t.Error("expected ReverseDone = false initially")
	}

	// Mark reverse done
	if err := MarkReverseDone(db); err != nil {
		t.Fatalf("MarkReverseDone: %v", err)
	}
	s, _ = GetTodaySession(db)
	if !s.ReverseDone {
		t.Error("expected ReverseDone = true after MarkReverseDone")
	}

	// Other phases not affected
	if s.ReviewDone || s.MatchDone || s.BlankDone {
		t.Errorf("expected other phases still false: %+v", s)
	}

	// Idempotent
	if err := MarkReverseDone(db); err != nil {
		t.Fatalf("MarkReverseDone idempotent: %v", err)
	}
	s, _ = GetTodaySession(db)
	if !s.ReverseDone {
		t.Error("expected ReverseDone = true after second MarkReverseDone")
	}
}

func TestUpdateReviewWithFSRSFields(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "front", "back", "", "", "", "")
	r, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("GetOrCreateReview: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	r.Stability = 3.17
	r.Difficulty = 5.5
	r.FSRSState = 1
	r.Lapses = 2
	r.LastReview = &now
	if err := UpdateReview(db, r); err != nil {
		t.Fatalf("UpdateReview: %v", err)
	}

	r2, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("second GetOrCreateReview: %v", err)
	}
	if r2.Stability != 3.17 {
		t.Errorf("Stability: got %f, want 3.17", r2.Stability)
	}
	if r2.Difficulty != 5.5 {
		t.Errorf("Difficulty: got %f, want 5.5", r2.Difficulty)
	}
	if r2.FSRSState != 1 {
		t.Errorf("FSRSState: got %d, want 1", r2.FSRSState)
	}
	if r2.Lapses != 2 {
		t.Errorf("Lapses: got %d, want 2", r2.Lapses)
	}
	if r2.LastReview == nil {
		t.Fatal("LastReview is nil after update")
	}
	if !r2.LastReview.Equal(now) {
		t.Errorf("LastReview: got %v, want %v", *r2.LastReview, now)
	}
}

func TestGetOrCreateReviewFSRSDefaults(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "front", "back", "", "", "", "")
	r, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("GetOrCreateReview: %v", err)
	}
	if r.Stability != 0 {
		t.Errorf("Stability: got %f, want 0", r.Stability)
	}
	if r.Difficulty != 0 {
		t.Errorf("Difficulty: got %f, want 0", r.Difficulty)
	}
	if r.FSRSState != 0 {
		t.Errorf("FSRSState: got %d, want 0", r.FSRSState)
	}
	if r.Lapses != 0 {
		t.Errorf("Lapses: got %d, want 0", r.Lapses)
	}
	if r.LastReview != nil {
		t.Errorf("LastReview: expected nil, got %v", *r.LastReview)
	}
}

func TestListDueCardsCarriesFSRSFields(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "front", "back", "", "", "", "")
	r, _ := GetOrCreateReview(db, id)

	now := time.Now().UTC().Truncate(time.Second)
	r.Stability = 7.0
	r.Difficulty = 4.2
	r.FSRSState = 2
	r.Lapses = 1
	r.LastReview = &now
	UpdateReview(db, r)

	due, err := ListDueCards(db, 10)
	if err != nil {
		t.Fatalf("ListDueCards: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due card, got %d", len(due))
	}
	got := due[0].Review
	if got.Stability != 7.0 {
		t.Errorf("Stability: got %f, want 7.0", got.Stability)
	}
	if got.Difficulty != 4.2 {
		t.Errorf("Difficulty: got %f, want 4.2", got.Difficulty)
	}
	if got.FSRSState != 2 {
		t.Errorf("FSRSState: got %d, want 2", got.FSRSState)
	}
	if got.Lapses != 1 {
		t.Errorf("Lapses: got %d, want 1", got.Lapses)
	}
}

func TestCalcStreak(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	threeDaysAgo := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")

	tests := []struct {
		name  string
		dates []string
		want  int
	}{
		{"empty", nil, 0},
		{"today only", []string{today}, 1},
		{"yesterday only", []string{yesterday}, 1},
		{"two consecutive ending today", []string{today, yesterday}, 2},
		{"three consecutive", []string{today, yesterday, twoDaysAgo}, 3},
		{"gap: today and 3 days ago", []string{today, threeDaysAgo}, 1},
		{"too old: only 3 days ago", []string{threeDaysAgo}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calcStreak(tt.dates); got != tt.want {
				t.Errorf("calcStreak(%v) = %d, want %d", tt.dates, got, tt.want)
			}
		})
	}
}
