package tui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

type msgStatsLoaded struct {
	stats          db.ReviewStats
	practice       db.PracticeTodayStats
	sessionCounts  map[string]int
	completedToday int
	err            error
}

type StatsModel struct {
	db             *sql.DB
	stats          db.ReviewStats
	practice       db.PracticeTodayStats
	sessionCounts  map[string]int
	completedToday int
	ready          bool
	err            string
}

func NewStatsModel(database *sql.DB) StatsModel {
	return StatsModel{db: database}
}

func (m StatsModel) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		s, err := db.GetReviewStats(database)
		if err != nil {
			return msgStatsLoaded{err: err}
		}
		practice, err := db.GetTodayPracticeStats(database)
		if err != nil {
			return msgStatsLoaded{err: err}
		}
		sessionCounts, err := db.GetRecentCompletedDailySessionCounts(database, 28)
		if err != nil {
			return msgStatsLoaded{err: err}
		}
		completedToday, err := db.CountCompletedDailySessionsToday(database)
		return msgStatsLoaded{
			stats: s, practice: practice, sessionCounts: sessionCounts,
			completedToday: completedToday, err: err,
		}
	}
}

func (m StatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgStatsLoaded:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.stats = msg.stats
			m.practice = msg.practice
			m.sessionCounts = msg.sessionCounts
			m.completedToday = msg.completedToday
			m.ready = true
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "q" || msg.String() == "enter" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}
	}
	return m, nil
}

func (m StatsModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Stats"))
	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: " + m.err))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))
		return b.String()
	}

	if !m.ready {
		b.WriteString(subtitleStyle.Render("Loading..."))
		return b.String()
	}

	s := m.stats

	// Streak
	streakLabel := fmt.Sprintf("%d day(s)", s.Streak)
	if s.Streak == 0 {
		streakLabel = "—"
	}
	b.WriteString(subtitleStyle.Render("Streak"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render(fmt.Sprintf("  %s", streakLabel)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  Streak counts days with at least one completed Daily Session."))
	b.WriteString("\n\n")

	// Activity calendar
	b.WriteString(subtitleStyle.Render("Activity (last 4 weeks)"))
	b.WriteString("\n")
	b.WriteString(renderCalendar(m.sessionCounts))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  Activity shows completed Daily Sessions only. Partial phase progress does not count here."))
	b.WriteString("\n\n")

	// Today
	b.WriteString(subtitleStyle.Render("Today"))
	b.WriteString("\n")
	sessionGoalLabel := fmt.Sprintf("  Daily Session: %d / %d minimum", m.completedToday, dailySessionMinimumGoal)
	sessionGoalStyle := labelStyle
	if m.completedToday >= dailySessionMinimumGoal {
		sessionGoalStyle = successStyle
	}
	if m.completedToday >= dailySessionIdealGoal {
		sessionGoalLabel = fmt.Sprintf("  Daily Session: %d / %d ideal", m.completedToday, dailySessionIdealGoal)
		sessionGoalStyle = idealStyle
	}
	b.WriteString(sessionGoalStyle.Render(sessionGoalLabel))
	b.WriteString("\n")
	if m.completedToday < dailySessionIdealGoal {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  Ideal line: %d / %d completed Daily Sessions.", m.completedToday, dailySessionIdealGoal)))
	} else {
		b.WriteString(idealStyle.Render("  Ideal line reached."))
	}
	b.WriteString("\n")
	correctRate := ""
	if s.ReviewedToday > 0 {
		rate := float64(s.CorrectToday) / float64(s.ReviewedToday) * 100
		correctRate = fmt.Sprintf("   Correct: %d / %d (%.0f%%)", s.CorrectToday, s.ReviewedToday, rate)
	}
	b.WriteString(labelStyle.Render(fmt.Sprintf("  Reviewed: %d / %d%s",
		s.ReviewedToday, dailyReviewLimit, correctRate)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  Daily Session and saved review updates count here."))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render(fmt.Sprintf("  Standalone practice: %d run(s), %d items, %d correct",
		m.practice.Runs, m.practice.Items, m.practice.Correct)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  Correct uses the final saved result. Standalone stays separate."))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("[esc] back"))
	return b.String()
}

// renderCalendar は過去4週間の学習カレンダーを生成する。
// 最新週が一番下、今日が右下端になるよう配置する。
func renderCalendar(sessionCounts map[string]int) string {
	var b strings.Builder
	today := time.Now()
	todayStr := today.Format("2006-01-02")

	// 今週の日曜を起点に4週前の日曜を算出
	weekday := int(today.Weekday())           // 0=Sun, 1=Mon, ...
	sunday := today.AddDate(0, 0, -weekday)   // 今週日曜
	startSunday := sunday.AddDate(0, 0, -7*3) // 4週前の日曜

	b.WriteString(helpStyle.Render("Su Mo Tu We Th Fr Sa"))
	b.WriteString("\n")

	for week := 0; week < 4; week++ {
		var row strings.Builder
		for day := 0; day < 7; day++ {
			d := startSunday.AddDate(0, 0, week*7+day)
			dStr := d.Format("2006-01-02")
			if day > 0 {
				row.WriteString(" ")
			}
			if dStr > todayStr {
				// 未来は空白（ヘッダの2文字幅に合わせる）
				row.WriteString("  ")
			} else {
				count := sessionCounts[dStr]
				switch {
				case count >= dailySessionIdealGoal:
					row.WriteString(idealStyle.Render("##"))
				case count >= dailySessionMinimumGoal:
					row.WriteString(successStyle.Render("##"))
				default:
					row.WriteString(helpStyle.Render("--"))
				}
			}
		}
		b.WriteString(row.String())
		b.WriteString("\n")
	}
	return b.String()
}
