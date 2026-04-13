package srs

import (
	"time"

	gofsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// CardState is the persistent SRS state for a single card.
// Zero value represents a brand-new card; go-fsrs initializes S/D on first Repeat.
type CardState struct {
	Stability     float64
	Difficulty    float64
	State         int // 0=New 1=Learning 2=Review 3=Relearning
	Reps          int
	Lapses        int
	LastReview    time.Time // zero value means never reviewed
	ScheduledDays int       // days until next review; mirrors Interval column
}

// Result is the outcome of scheduling a review.
type Result struct {
	CardState
	Due time.Time // absolute due timestamp
}

// Rating is the user's self-assessed recall quality.
type Rating int

const (
	RatingAgain Rating = 1
	RatingHard  Rating = 2
	RatingGood  Rating = 3
	RatingEasy  Rating = 4
)

// fsrsInstance is a package-level scheduler with deterministic behaviour.
// EnableFuzz=false ensures test reproducibility.
var fsrsInstance = func() *gofsrs.FSRS {
	p := gofsrs.DefaultParam()
	p.EnableFuzz = false
	return gofsrs.NewFSRS(p)
}()

// RatingFromSM2 maps the legacy SM-2 0–5 scale to an FSRS Rating.
// Existing call sites that pass rating=4 (correct) or rating=0 (wrong)
// continue to work without change.
func RatingFromSM2(sm2 int) Rating {
	switch {
	case sm2 <= 2:
		return RatingAgain
	case sm2 == 3:
		return RatingHard
	case sm2 == 4:
		return RatingGood
	default: // 5+
		return RatingEasy
	}
}

// Schedule computes the next card state given the current state, rating, and current time.
// Pure function: no I/O, deterministic when EnableFuzz=false.
func Schedule(current CardState, rating Rating, now time.Time) Result {
	card := gofsrs.Card{
		Stability:     current.Stability,
		Difficulty:    current.Difficulty,
		State:         gofsrs.State(current.State),
		Reps:          uint64(current.Reps),
		Lapses:        uint64(current.Lapses),
		ScheduledDays: uint64(current.ScheduledDays),
		LastReview:    current.LastReview,
	}

	recordLog := fsrsInstance.Repeat(card, now)
	next := recordLog[gofsrs.Rating(rating)].Card

	return Result{
		CardState: CardState{
			Stability:     next.Stability,
			Difficulty:    next.Difficulty,
			State:         int(next.State),
			Reps:          int(next.Reps),
			Lapses:        int(next.Lapses),
			ScheduledDays: int(next.ScheduledDays),
			LastReview:    now,
		},
		Due: next.Due,
	}
}
