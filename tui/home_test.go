package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

func TestHomeViewEmphasizesDailySessionStatus(t *testing.T) {
	m := NewHomeModel(nil, nil, config.Config{})
	m.statsReady = true
	m.total = 42
	m.due = 7
	m.reviewedToday = 11
	m.completedToday = 1
	m.minimumWeekdays = 5
	m.idealWeekdays = 2
	m.resumeAvailable = true
	m.practiceToday = db.PracticeTodayStats{
		Runs:    3,
		Items:   18,
		Correct: 14,
	}

	view := m.View()

	if !strings.Contains(view, "Today: minimum reached. 1 more for ideal.") {
		t.Fatalf("expected primary Daily Session status, got: %s", view)
	}
	if !strings.Contains(view, "Reviewed: 11 / 100") {
		t.Fatalf("expected compact reviewed summary, got: %s", view)
	}
	if !strings.Contains(view, "Last 7 days: minimum 5/7   ideal 2/7") {
		t.Fatalf("expected weekly attainment summary, got: %s", view)
	}
	if !strings.Contains(view, "Resume available: unfinished Daily Session detected") {
		t.Fatalf("expected resume hint, got: %s", view)
	}
	if strings.Contains(view, "Standalone today:") {
		t.Fatalf("expected standalone detail to be removed from Home, got: %s", view)
	}
	if strings.Contains(view, "Session: Review") {
		t.Fatalf("expected phase checklist to be removed from Home, got: %s", view)
	}
}

func TestDailySessionStatusLabels(t *testing.T) {
	tests := []struct {
		name      string
		completed int
		want      string
	}{
		{name: "not started", completed: 0, want: "Today: start 1 Daily Session to reach minimum."},
		{name: "minimum reached", completed: 1, want: "Today: minimum reached. 1 more for ideal."},
		{name: "ideal reached", completed: 2, want: "Today: ideal reached (2 / 2 Daily Sessions)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := HomeModel{completedToday: tt.completed}
			got, _ := m.dailySessionStatus()
			if got != tt.want {
				t.Fatalf("dailySessionStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeWeeklySessionGoals(t *testing.T) {
	today := time.Now()
	counts := map[string]int{
		today.Format("2006-01-02"):                   2,
		today.AddDate(0, 0, -1).Format("2006-01-02"): 1,
		today.AddDate(0, 0, -3).Format("2006-01-02"): 2,
	}

	minimum, ideal := summarizeWeeklySessionGoals(counts)
	if minimum != 3 {
		t.Fatalf("minimum = %d, want 3", minimum)
	}
	if ideal != 2 {
		t.Fatalf("ideal = %d, want 2", ideal)
	}
}

func TestHomeToolsIncludesSuspiciousCardsEntry(t *testing.T) {
	m := NewHomeModel(nil, nil, config.Config{})
	m.state = homeStateTools

	view := m.View()
	if !strings.Contains(view, "Suspicious cards") {
		t.Fatalf("expected suspicious cards menu entry, got: %s", view)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected navigation command")
	}
	got := updated.(HomeModel)
	if got.state != homeStateTools {
		t.Fatalf("expected state to stay in tools until nav message, got %v", got.state)
	}
	msg := cmd()
	gotoMsg, ok := msg.(MsgGotoScreen)
	if !ok {
		t.Fatalf("expected MsgGotoScreen, got %T", msg)
	}
	if gotoMsg.Target != screenSuspiciousList {
		t.Fatalf("target = %v, want %v", gotoMsg.Target, screenSuspiciousList)
	}
}
