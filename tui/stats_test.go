package tui

import (
	"strings"
	"testing"

	"github.com/ippei/lazyrecall/db"
)

func TestStatsViewShowsTodayExplanationAndStandalonePractice(t *testing.T) {
	m := NewStatsModel(nil)
	m.ready = true
	m.stats = db.ReviewStats{
		Streak:        3,
		TotalCards:    20,
		MatureCards:   4,
		LearningCards: 7,
		NewCards:      9,
		ReviewedToday: 12,
		CorrectToday:  9,
	}
	m.practice = db.PracticeTodayStats{
		Runs:    2,
		Items:   15,
		Correct: 11,
	}
	m.recentRuns = []db.PracticeRun{
		{Mode: "match", FinishedAt: "2026-04-24 10:20:00", Total: 4, Correct: 3},
	}
	m.recentDates = map[string]bool{}

	view := m.View()
	if !strings.Contains(view, "Daily Session and saved review updates count here.") {
		t.Fatalf("expected Today explanation, got: %s", view)
	}
	if !strings.Contains(view, "Standalone practice: 2 run(s), 15 items, 11 correct") {
		t.Fatalf("expected standalone summary, got: %s", view)
	}
	if !strings.Contains(view, "Match") {
		t.Fatalf("expected recent standalone practice mode label, got: %s", view)
	}
}
