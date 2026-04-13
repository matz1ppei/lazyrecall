package srs

import (
	"testing"
	"time"
)

func TestRatingFromSM2(t *testing.T) {
	tests := []struct {
		sm2  int
		want Rating
	}{
		{0, RatingAgain},
		{1, RatingAgain},
		{2, RatingAgain},
		{3, RatingHard},
		{4, RatingGood},
		{5, RatingEasy},
		{6, RatingEasy},
	}
	for _, tt := range tests {
		got := RatingFromSM2(tt.sm2)
		if got != tt.want {
			t.Errorf("RatingFromSM2(%d) = %v, want %v", tt.sm2, got, tt.want)
		}
	}
}

func TestSchedule_NewCardGood(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	res := Schedule(CardState{}, RatingGood, now)
	// With EnableShortTerm=true (default), a new card enters Learning state first.
	// ScheduledDays=0 is expected; the due time is set to a short-term interval in minutes.
	if res.Stability <= 0 {
		t.Errorf("Stability = %f, want > 0", res.Stability)
	}
	if res.State == 0 {
		t.Errorf("State = 0 (New) after first review, want non-zero")
	}
	if res.Reps != 1 {
		t.Errorf("Reps = %d, want 1", res.Reps)
	}
	if res.Lapses != 0 {
		t.Errorf("Lapses = %d, want 0", res.Lapses)
	}
}

func TestSchedule_NewCardAgain(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	res := Schedule(CardState{}, RatingAgain, now)
	// go-fsrs: Again on a New card increments Reps but not Lapses (Learning, not Relearning)
	if res.Reps != 1 {
		t.Errorf("Reps = %d, want 1", res.Reps)
	}
	// State should transition out of New
	if res.State == 0 {
		t.Errorf("State = 0 (New) after first review, want non-zero")
	}
}

func TestSchedule_Deterministic(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	state := CardState{}
	first := Schedule(state, RatingGood, now)
	for i := 0; i < 99; i++ {
		got := Schedule(state, RatingGood, now)
		if got != first {
			t.Fatalf("iteration %d: non-deterministic result", i+2)
		}
	}
}

func TestSchedule_ConsecutiveGoodIncreasesStability(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state := CardState{}
	prevStability := 0.0
	for i := 0; i < 5; i++ {
		res := Schedule(state, RatingGood, now)
		if res.Stability <= prevStability {
			t.Errorf("step %d: Stability did not increase: %f -> %f", i+1, prevStability, res.Stability)
		}
		prevStability = res.Stability
		state = res.CardState
		now = res.Due
	}
}

func TestSchedule_DueMatchesScheduledDays(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Use Easy on a new card to get a long-term interval (ScheduledDays > 0).
	res := Schedule(CardState{}, RatingEasy, now)
	if res.ScheduledDays == 0 {
		t.Skip("ScheduledDays=0, short-term interval applies — skipping day-alignment check")
	}
	expectedDue := now.AddDate(0, 0, res.ScheduledDays)
	diff := res.Due.Sub(expectedDue)
	if diff < 0 {
		diff = -diff
	}
	if diff > 60*time.Minute {
		t.Errorf("Due %v is more than 1h from now+ScheduledDays %v", res.Due, expectedDue)
	}
}
