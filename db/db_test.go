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

func TestCreateCardWithReviewCreatesReviewRow(t *testing.T) {
	db := openTestDB(t)

	id, err := CreateCardWithReview(db, "front1", "back1", "hint1", "", "", "")
	if err != nil {
		t.Fatalf("CreateCardWithReview: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	card, err := GetCard(db, id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Front != "front1" {
		t.Fatalf("unexpected card: %+v", card)
	}

	review, err := GetOrCreateReview(db, id)
	if err != nil {
		t.Fatalf("GetOrCreateReview: %v", err)
	}
	if review.CardID != id {
		t.Fatalf("review.CardID = %d, want %d", review.CardID, id)
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

	// Card due now (default due_date = datetime('now'))
	id1, _ := CreateCard(db, "due", "back", "", "", "", "")
	_, _ = GetOrCreateReview(db, id1)

	// Card due later today — should not be returned yet
	id2, _ := CreateCard(db, "future", "back", "", "", "", "")
	r2, _ := GetOrCreateReview(db, id2)
	r2.DueDate = time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05")
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

func TestListSuspiciousCards(t *testing.T) {
	db := openTestDB(t)

	mismatchID, _ := CreateCardWithReview(db, "conmigo", "with me", "", "Ven conmigo.", "", "acompanadme")
	missingFrontID, _ := CreateCardWithReview(db, "bonjour", "hello", "", "Salut tout le monde.", "", "bonjour")
	normalID, _ := CreateCardWithReview(db, "hola", "hello", "", "Hola, amigo.", "", "hola")

	cards, err := ListSuspiciousCards(db)
	if err != nil {
		t.Fatalf("ListSuspiciousCards: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 suspicious cards, got %d", len(cards))
	}

	got := map[int64]string{}
	for _, card := range cards {
		got[card.Card.ID] = card.Reason
	}

	if got[mismatchID] != "example word mismatch" {
		t.Fatalf("mismatch reason = %q", got[mismatchID])
	}
	if got[missingFrontID] != "front missing in example" {
		t.Fatalf("missing-front reason = %q", got[missingFrontID])
	}
	if _, ok := got[normalID]; ok {
		t.Fatalf("normal card should not be suspicious: %+v", got)
	}
}

func TestSelectSessionCardsPrioritizesDueBeforeNew(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()

	overdueID, _ := CreateCard(db, "overdue", "back", "", "", "", "")
	learningID, _ := CreateCard(db, "learning", "back", "", "", "", "")
	reviewID, _ := CreateCard(db, "review", "back", "", "", "", "")
	new1ID, _ := CreateCard(db, "new-1", "back", "", "", "", "")
	new2ID, _ := CreateCard(db, "new-2", "back", "", "", "", "")

	for _, id := range []int64{overdueID, learningID, reviewID, new1ID, new2ID} {
		if _, err := GetOrCreateReview(db, id); err != nil {
			t.Fatalf("GetOrCreateReview(%d): %v", id, err)
		}
	}

	if _, err := db.Exec(
		`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 5 WHERE card_id = ?`,
		now.Add(-24*time.Hour).Format("2006-01-02 15:04:05"),
		now.Add(-48*time.Hour).Format("2006-01-02 15:04:05"),
		overdueID,
	); err != nil {
		t.Fatalf("update overdue: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 5 WHERE card_id = ?`,
		now.Add(-1*time.Hour).Format("2006-01-02 15:04:05"),
		now.Add(-24*time.Hour).Format("2006-01-02 15:04:05"),
		learningID,
	); err != nil {
		t.Fatalf("update learning: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 30 WHERE card_id = ?`,
		now.Add(-30*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-72*time.Hour).Format("2006-01-02 15:04:05"),
		reviewID,
	); err != nil {
		t.Fatalf("update review: %v", err)
	}

	cards, err := SelectSessionCards(db, 3)
	if err != nil {
		t.Fatalf("SelectSessionCards: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("len(cards) = %d, want 3", len(cards))
	}
	want := []string{"overdue", "learning", "review"}
	for i, front := range want {
		if cards[i].Front != front {
			t.Fatalf("cards[%d].Front = %q, want %q", i, cards[i].Front, front)
		}
	}
}

func TestSelectSessionCardsCapsNewCardsPerSession(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()

	dueID, _ := CreateCard(db, "due-learning", "back", "", "", "", "")
	if _, err := GetOrCreateReview(db, dueID); err != nil {
		t.Fatalf("GetOrCreateReview(due): %v", err)
	}
	if _, err := db.Exec(
		`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 5 WHERE card_id = ?`,
		now.Add(-1*time.Hour).Format("2006-01-02 15:04:05"),
		now.Add(-24*time.Hour).Format("2006-01-02 15:04:05"),
		dueID,
	); err != nil {
		t.Fatalf("update due: %v", err)
	}

	for i := 0; i < 3; i++ {
		id, _ := CreateCard(db, fmt.Sprintf("new-%d", i), "back", "", "", "", "")
		if _, err := GetOrCreateReview(db, id); err != nil {
			t.Fatalf("GetOrCreateReview(new-%d): %v", i, err)
		}
		if _, err := db.Exec(
			`UPDATE reviews SET due_date = ?, reviewed_at = NULL WHERE card_id = ?`,
			now.Add(24*time.Hour).Format("2006-01-02 15:04:05"),
			id,
		); err != nil {
			t.Fatalf("update new-%d: %v", i, err)
		}
	}

	for i := 0; i < 3; i++ {
		id, _ := CreateCard(db, fmt.Sprintf("review-%d", i), "back", "", "", "", "")
		if _, err := GetOrCreateReview(db, id); err != nil {
			t.Fatalf("GetOrCreateReview(review-%d): %v", i, err)
		}
		if _, err := db.Exec(
			`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 30 WHERE card_id = ?`,
			now.Add(24*time.Hour).Format("2006-01-02 15:04:05"),
			now.Add(-72*time.Hour).Format("2006-01-02 15:04:05"),
			id,
		); err != nil {
			t.Fatalf("update review-%d: %v", i, err)
		}
	}

	cards, err := SelectSessionCards(db, 6)
	if err != nil {
		t.Fatalf("SelectSessionCards: %v", err)
	}
	if len(cards) != 6 {
		t.Fatalf("len(cards) = %d, want 6", len(cards))
	}

	newCount := 0
	for _, c := range cards {
		if c.Review.ReviewedAt == nil {
			newCount++
		}
	}
	if newCount != 2 {
		t.Fatalf("newCount = %d, want 2", newCount)
	}
}

func TestSelectSessionCardsCapsNewCardsEvenWhenNewAreDueNow(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()

	dueID, _ := CreateCard(db, "due-learning", "back", "", "", "", "")
	if _, err := GetOrCreateReview(db, dueID); err != nil {
		t.Fatalf("GetOrCreateReview(due): %v", err)
	}
	if _, err := db.Exec(
		`UPDATE reviews SET due_date = ?, reviewed_at = ?, stability = 5 WHERE card_id = ?`,
		now.Add(-1*time.Hour).Format("2006-01-02 15:04:05"),
		now.Add(-24*time.Hour).Format("2006-01-02 15:04:05"),
		dueID,
	); err != nil {
		t.Fatalf("update due: %v", err)
	}

	for i := 0; i < 8; i++ {
		id, _ := CreateCard(db, fmt.Sprintf("new-due-%d", i), "back", "", "", "", "")
		if _, err := GetOrCreateReview(db, id); err != nil {
			t.Fatalf("GetOrCreateReview(new-due-%d): %v", i, err)
		}
		if _, err := db.Exec(
			`UPDATE reviews SET due_date = ?, reviewed_at = NULL WHERE card_id = ?`,
			now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"),
			id,
		); err != nil {
			t.Fatalf("update new-due-%d: %v", i, err)
		}
	}

	cards, err := SelectSessionCards(db, 6)
	if err != nil {
		t.Fatalf("SelectSessionCards: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("len(cards) = %d, want 3 (1 due + 2 new)", len(cards))
	}

	newCount := 0
	for _, c := range cards {
		if c.Review.ReviewedAt == nil {
			newCount++
		}
	}
	if newCount != 2 {
		t.Fatalf("newCount = %d, want 2", newCount)
	}
}

func TestCountDueCardsUsesDatetimePrecision(t *testing.T) {
	db := openTestDB(t)

	nowID, _ := CreateCard(db, "now", "back", "", "", "", "")
	laterID, _ := CreateCard(db, "later", "back", "", "", "", "")
	_, _ = GetOrCreateReview(db, nowID)
	later, _ := GetOrCreateReview(db, laterID)
	later.DueDate = time.Now().Add(90 * time.Minute).Format("2006-01-02 15:04:05")
	if err := UpdateReview(db, later); err != nil {
		t.Fatalf("UpdateReview(later): %v", err)
	}

	count, err := CountDueCards(db)
	if err != nil {
		t.Fatalf("CountDueCards: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountDueCards = %d, want 1", count)
	}
}

func TestCountOverdueCardsCountsPastDaysOnly(t *testing.T) {
	db := openTestDB(t)

	pastID, _ := CreateCard(db, "past", "back", "", "", "", "")
	todayID, _ := CreateCard(db, "today", "back", "", "", "", "")
	past, _ := GetOrCreateReview(db, pastID)
	today, _ := GetOrCreateReview(db, todayID)
	past.DueDate = time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	today.DueDate = time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	if err := UpdateReview(db, past); err != nil {
		t.Fatalf("UpdateReview(past): %v", err)
	}
	if err := UpdateReview(db, today); err != nil {
		t.Fatalf("UpdateReview(today): %v", err)
	}

	count, err := CountOverdueCards(db)
	if err != nil {
		t.Fatalf("CountOverdueCards: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountOverdueCards = %d, want 1", count)
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

func TestApplySessionResultsUpdatesReviewedTodayAndEndsSession(t *testing.T) {
	db := openTestDB(t)

	cardID1, _ := CreateCard(db, "front1", "back1", "", "", "", "")
	cardID2, _ := CreateCard(db, "front2", "back2", "", "", "", "")

	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}

	err = ApplySessionResults(db, []SessionResult{
		{CardID: cardID1, Rating: 4},
		{CardID: cardID2, Rating: 0},
	}, sessionID)
	if err != nil {
		t.Fatalf("ApplySessionResults: %v", err)
	}

	reviewedToday, err := CountReviewedToday(db)
	if err != nil {
		t.Fatalf("CountReviewedToday: %v", err)
	}
	if reviewedToday != 2 {
		t.Fatalf("CountReviewedToday = %d, want 2", reviewedToday)
	}

	var endedAt sql.NullString
	if err := db.QueryRow(`SELECT ended_at FROM review_sessions WHERE id = ?`, sessionID).Scan(&endedAt); err != nil {
		t.Fatalf("review_sessions ended_at: %v", err)
	}
	if !endedAt.Valid || endedAt.String == "" {
		t.Fatal("expected ended_at to be set")
	}

	r1, err := GetOrCreateReview(db, cardID1)
	if err != nil {
		t.Fatalf("GetOrCreateReview(card1): %v", err)
	}
	if r1.LastRating == nil || *r1.LastRating != 4 {
		t.Fatalf("card1 LastRating = %v, want 4", r1.LastRating)
	}

	r2, err := GetOrCreateReview(db, cardID2)
	if err != nil {
		t.Fatalf("GetOrCreateReview(card2): %v", err)
	}
	if r2.LastRating == nil || *r2.LastRating != 0 {
		t.Fatalf("card2 LastRating = %v, want 0", r2.LastRating)
	}
}

func TestApplySessionResultsRollsBackOnError(t *testing.T) {
	db := openTestDB(t)

	cardID, _ := CreateCard(db, "front", "back", "", "", "", "")
	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}

	err = ApplySessionResults(db, []SessionResult{
		{CardID: cardID, Rating: 4},
		{CardID: 999999, Rating: 0},
	}, sessionID)
	if err == nil {
		t.Fatal("expected ApplySessionResults to fail for missing card")
	}

	reviewedToday, err := CountReviewedToday(db)
	if err != nil {
		t.Fatalf("CountReviewedToday: %v", err)
	}
	if reviewedToday != 0 {
		t.Fatalf("CountReviewedToday = %d, want 0 after rollback", reviewedToday)
	}

	var endedAt sql.NullString
	if err := db.QueryRow(`SELECT ended_at FROM review_sessions WHERE id = ?`, sessionID).Scan(&endedAt); err != nil {
		t.Fatalf("review_sessions ended_at: %v", err)
	}
	if endedAt.Valid {
		t.Fatalf("expected ended_at to stay NULL after rollback, got %q", endedAt.String)
	}
}

func TestCountReviewedTodayIncludesEndedSessionEvents(t *testing.T) {
	db := openTestDB(t)

	cardID1, _ := CreateCard(db, "front1", "back1", "", "", "", "")
	cardID2, _ := CreateCard(db, "front2", "back2", "", "", "", "")

	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}
	if err := InsertReviewEvent(db, sessionID, cardID1, 1, 500, true); err != nil {
		t.Fatalf("InsertReviewEvent(card1): %v", err)
	}
	if err := InsertReviewEvent(db, sessionID, cardID2, 2, 500, true); err != nil {
		t.Fatalf("InsertReviewEvent(card2): %v", err)
	}
	if err := EndReviewSession(db, sessionID); err != nil {
		t.Fatalf("EndReviewSession: %v", err)
	}

	reviewedToday, err := CountReviewedToday(db)
	if err != nil {
		t.Fatalf("CountReviewedToday: %v", err)
	}
	if reviewedToday != 2 {
		t.Fatalf("CountReviewedToday = %d, want 2", reviewedToday)
	}
}

func TestCountReviewedTodayAddsStandaloneAndSessionCountsWithoutDoubleCounting(t *testing.T) {
	db := openTestDB(t)

	sessionCardID, _ := CreateCard(db, "session-front", "session-back", "", "", "", "")
	standaloneCardID, _ := CreateCard(db, "standalone-front", "standalone-back", "", "", "", "")

	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}
	if err := InsertReviewEvent(db, sessionID, sessionCardID, 1, 500, true); err != nil {
		t.Fatalf("InsertReviewEvent(session): %v", err)
	}
	if err := EndReviewSession(db, sessionID); err != nil {
		t.Fatalf("EndReviewSession: %v", err)
	}

	if err := ApplySessionResults(db, []SessionResult{{CardID: sessionCardID, Rating: 4}}, sessionID); err != nil {
		t.Fatalf("ApplySessionResults: %v", err)
	}

	review, err := GetOrCreateReview(db, standaloneCardID)
	if err != nil {
		t.Fatalf("GetOrCreateReview(standalone): %v", err)
	}
	rating := 4
	review.LastRating = &rating
	if err := UpdateReview(db, review); err != nil {
		t.Fatalf("UpdateReview(standalone): %v", err)
	}

	reviewedToday, err := CountReviewedToday(db)
	if err != nil {
		t.Fatalf("CountReviewedToday: %v", err)
	}
	if reviewedToday != 2 {
		t.Fatalf("CountReviewedToday = %d, want 2", reviewedToday)
	}

	stats, err := GetReviewStats(db)
	if err != nil {
		t.Fatalf("GetReviewStats: %v", err)
	}
	if stats.ReviewedToday != 2 {
		t.Fatalf("GetReviewStats.ReviewedToday = %d, want 2", stats.ReviewedToday)
	}
}

func TestCountReviewedTodayUsesSessionCompletionDay(t *testing.T) {
	db := openTestDB(t)

	cardID, _ := CreateCard(db, "front", "back", "", "", "", "")

	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}
	if err := InsertReviewEvent(db, sessionID, cardID, 1, 500, true); err != nil {
		t.Fatalf("InsertReviewEvent: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE review_sessions
		 SET started_at = datetime('now', '-1 day'),
		     ended_at = datetime('now')
		 WHERE id = ?`,
		sessionID,
	); err != nil {
		t.Fatalf("UPDATE review_sessions: %v", err)
	}

	reviewedToday, err := CountReviewedToday(db)
	if err != nil {
		t.Fatalf("CountReviewedToday: %v", err)
	}
	if reviewedToday != 1 {
		t.Fatalf("CountReviewedToday = %d, want 1 for a session that ended today", reviewedToday)
	}

	stats, err := GetReviewStats(db)
	if err != nil {
		t.Fatalf("GetReviewStats: %v", err)
	}
	if stats.ReviewedToday != 1 {
		t.Fatalf("GetReviewStats.ReviewedToday = %d, want 1", stats.ReviewedToday)
	}
	if stats.CorrectToday != 0 {
		t.Fatalf("GetReviewStats.CorrectToday = %d, want 0 without final saved rating", stats.CorrectToday)
	}
}

func TestGetReviewStatsCorrectTodayUsesFinalDailySessionRating(t *testing.T) {
	db := openTestDB(t)

	cardID, _ := CreateCard(db, "front", "back", "", "", "", "")

	sessionID, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession: %v", err)
	}
	if err := InsertReviewEvent(db, sessionID, cardID, 1, 500, true); err != nil {
		t.Fatalf("InsertReviewEvent: %v", err)
	}
	if err := EndReviewSession(db, sessionID); err != nil {
		t.Fatalf("EndReviewSession: %v", err)
	}
	if err := ApplySessionResults(db, []SessionResult{{CardID: cardID, Rating: 0}}, sessionID); err != nil {
		t.Fatalf("ApplySessionResults: %v", err)
	}

	stats, err := GetReviewStats(db)
	if err != nil {
		t.Fatalf("GetReviewStats: %v", err)
	}
	if stats.ReviewedToday != 1 {
		t.Fatalf("GetReviewStats.ReviewedToday = %d, want 1", stats.ReviewedToday)
	}
	if stats.CorrectToday != 0 {
		t.Fatalf("GetReviewStats.CorrectToday = %d, want 0", stats.CorrectToday)
	}
}

func TestCompletedDailySessionCountsDriveGoalsAndStreak(t *testing.T) {
	db := openTestDB(t)

	sessionToday1, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession(today1): %v", err)
	}
	sessionToday2, err := StartReviewSession(db, "daily_session", 2)
	if err != nil {
		t.Fatalf("StartReviewSession(today2): %v", err)
	}
	sessionYesterday, err := StartReviewSession(db, "daily_session", 1)
	if err != nil {
		t.Fatalf("StartReviewSession(yesterday): %v", err)
	}
	nowUTC := time.Now().UTC()
	if _, err := db.Exec(
		`UPDATE review_sessions
		 SET ended_at = ?
		 WHERE id IN (?, ?)`,
		nowUTC.Format("2006-01-02 15:04:05"),
		sessionToday1, sessionToday2,
	); err != nil {
		t.Fatalf("UPDATE review_sessions(today): %v", err)
	}
	if _, err := db.Exec(
		`UPDATE review_sessions
		 SET started_at = ?,
		     ended_at = ?
		 WHERE id = ?`,
		nowUTC.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"),
		nowUTC.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"),
		sessionYesterday,
	); err != nil {
		t.Fatalf("UPDATE review_sessions(yesterday): %v", err)
	}

	completedToday, err := CountCompletedDailySessionsToday(db)
	if err != nil {
		t.Fatalf("CountCompletedDailySessionsToday: %v", err)
	}
	if completedToday != 2 {
		t.Fatalf("CountCompletedDailySessionsToday = %d, want 2", completedToday)
	}

	counts, err := GetRecentCompletedDailySessionCounts(db, 7)
	if err != nil {
		t.Fatalf("GetRecentCompletedDailySessionCounts: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if counts[today] != 2 {
		t.Fatalf("counts[today] = %d, want 2", counts[today])
	}
	if counts[yesterday] != 1 {
		t.Fatalf("counts[yesterday] = %d, want 1", counts[yesterday])
	}

	stats, err := GetReviewStats(db)
	if err != nil {
		t.Fatalf("GetReviewStats: %v", err)
	}
	if stats.Streak != 2 {
		t.Fatalf("GetReviewStats.Streak = %d, want 2", stats.Streak)
	}
}

func TestPracticeRunLoggingAndQueries(t *testing.T) {
	db := openTestDB(t)

	if err := LogPracticeRun(db, "review", time.Now().UTC().Add(-15*time.Minute).Format("2006-01-02 15:04:05"), time.Now().UTC().Add(-10*time.Minute).Format("2006-01-02 15:04:05"), 8, 6); err != nil {
		t.Fatalf("LogPracticeRun(review): %v", err)
	}
	if err := LogPracticeRun(db, "blank", time.Now().UTC().Add(-5*time.Minute).Format("2006-01-02 15:04:05"), time.Now().UTC().Format("2006-01-02 15:04:05"), 4, 3); err != nil {
		t.Fatalf("LogPracticeRun(blank): %v", err)
	}

	stats, err := GetTodayPracticeStats(db)
	if err != nil {
		t.Fatalf("GetTodayPracticeStats: %v", err)
	}
	if stats.Runs != 2 || stats.Items != 12 || stats.Correct != 9 {
		t.Fatalf("unexpected practice stats: %+v", stats)
	}

	runs, err := ListRecentPracticeRuns(db, 5)
	if err != nil {
		t.Fatalf("ListRecentPracticeRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].Mode != "blank" {
		t.Fatalf("runs[0].Mode = %q, want blank", runs[0].Mode)
	}
}
