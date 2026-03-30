package db

import (
	"database/sql"
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

	id, err := CreateCard(db, "front1", "back1", "hint1")
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
		if _, err := CreateCard(db, "f", "b", ""); err != nil {
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

func TestDeleteCard(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "f", "b", "")
	if err := DeleteCard(db, id); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := GetCard(db, id); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestGetOrCreateReviewIdempotent(t *testing.T) {
	db := openTestDB(t)

	id, _ := CreateCard(db, "f", "b", "")

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

func TestListDueCards(t *testing.T) {
	db := openTestDB(t)

	// Card due today (default due_date = date('now'))
	id1, _ := CreateCard(db, "due", "back", "")
	_, _ = GetOrCreateReview(db, id1)

	// Card due tomorrow — update due_date to future
	id2, _ := CreateCard(db, "not due", "back", "")
	r2, _ := GetOrCreateReview(db, id2)
	r2.DueDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if err := UpdateReview(db, r2); err != nil {
		t.Fatalf("UpdateReview: %v", err)
	}

	due, err := ListDueCards(db)
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
