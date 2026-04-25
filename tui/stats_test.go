package tui

import (
	"strings"
	"testing"
	"time"

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
	m.completedToday = 2
	m.sessionCounts = map[string]int{
		time.Now().Format("2006-01-02"): 2,
	}

	view := m.View()
	if !strings.Contains(view, "Daily Session: 2 / 2 ideal") {
		t.Fatalf("expected ideal Daily Session label, got: %s", view)
	}
	if !strings.Contains(view, "Streak counts days with at least one completed Daily Session.") {
		t.Fatalf("expected streak explanation, got: %s", view)
	}
	if !strings.Contains(view, "Activity shows completed Daily Sessions only. Partial phase progress does not count here.") {
		t.Fatalf("expected activity explanation, got: %s", view)
	}
	if !strings.Contains(view, "Daily Session and saved review updates count here.") {
		t.Fatalf("expected Today explanation, got: %s", view)
	}
	if !strings.Contains(view, "Standalone practice: 2 run(s), 15 items, 11 correct") {
		t.Fatalf("expected standalone summary, got: %s", view)
	}
	if strings.Contains(view, "Recent Standalone Practice") {
		t.Fatalf("expected recent standalone practice section to be removed, got: %s", view)
	}
}

func TestRenderCalendarUsesMinimumAndIdealGoalStyles(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	out := renderCalendar(map[string]int{
		today:     2,
		yesterday: 1,
	})

	if !strings.Contains(out, successStyle.Render("##")) {
		t.Fatalf("expected minimum-goal marker, got: %s", out)
	}
	if !strings.Contains(out, idealStyle.Render("##")) {
		t.Fatalf("expected ideal-goal marker, got: %s", out)
	}
}
