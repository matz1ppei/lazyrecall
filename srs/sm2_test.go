package srs

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestNextState_Rating0_Reset(t *testing.T) {
	s := CardState{Interval: 10, EaseFactor: 2.5, Repetitions: 3}
	got := NextState(s, 0)
	if got.Interval != 1 || got.Repetitions != 0 {
		t.Errorf("rating=0: want interval=1 reps=0, got %+v", got)
	}
}

func TestNextState_Rating2_Reset(t *testing.T) {
	s := CardState{Interval: 6, EaseFactor: 2.5, Repetitions: 2}
	got := NextState(s, 2)
	if got.Interval != 1 || got.Repetitions != 0 {
		t.Errorf("rating=2: want interval=1 reps=0, got %+v", got)
	}
}

func TestNextState_Rating3_Advances(t *testing.T) {
	s := CardState{Interval: 1, EaseFactor: 2.5, Repetitions: 0}
	got := NextState(s, 3)
	if got.Interval != 1 || got.Repetitions != 1 {
		t.Errorf("rating=3 reps=0: want interval=1 reps=1, got %+v", got)
	}

	s2 := CardState{Interval: 1, EaseFactor: 2.5, Repetitions: 1}
	got2 := NextState(s2, 3)
	if got2.Interval != 6 || got2.Repetitions != 2 {
		t.Errorf("rating=3 reps=1: want interval=6 reps=2, got %+v", got2)
	}
}

func TestNextState_Rating5_IncreasesIntervalAndEase(t *testing.T) {
	s := CardState{Interval: 6, EaseFactor: 2.5, Repetitions: 2}
	got := NextState(s, 5)
	wantInterval := int(math.Round(float64(6) * 2.5))
	if got.Interval != wantInterval {
		t.Errorf("rating=5: want interval=%d, got %d", wantInterval, got.Interval)
	}
	wantEF := 2.5 + (0.1 - float64(0)*(0.08+float64(0)*0.02))
	if !approxEqual(got.EaseFactor, wantEF) {
		t.Errorf("rating=5: want ease=%.4f, got %.4f", wantEF, got.EaseFactor)
	}
	if got.Repetitions != 3 {
		t.Errorf("rating=5: want reps=3, got %d", got.Repetitions)
	}
}

func TestNextState_EaseFactorClamp(t *testing.T) {
	// rating=0 repeatedly should not drop ease below 1.3
	s := CardState{Interval: 1, EaseFactor: 1.3, Repetitions: 0}
	for i := 0; i < 10; i++ {
		s = NextState(s, 0)
		if s.EaseFactor < 1.3 {
			t.Errorf("ease factor dropped below 1.3: %.4f", s.EaseFactor)
		}
	}
}

func TestNextState_MultiStepSequence(t *testing.T) {
	// 3 correct answers then 1 failure → reset
	s := CardState{Interval: 1, EaseFactor: 2.5, Repetitions: 0}
	s = NextState(s, 5) // reps=1, interval=1
	s = NextState(s, 5) // reps=2, interval=6
	s = NextState(s, 5) // reps=3, interval=round(6*ef)
	if s.Repetitions != 3 || s.Interval < 10 {
		t.Errorf("after 3 correct: want reps=3 interval>=10, got %+v", s)
	}

	s = NextState(s, 1) // fail → reset
	if s.Repetitions != 0 || s.Interval != 1 {
		t.Errorf("after failure: want reps=0 interval=1, got %+v", s)
	}
}
