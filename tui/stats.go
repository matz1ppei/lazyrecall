package tui

import (
	"database/sql"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

type msgStatsLoaded struct {
	stats db.ReviewStats
	err   error
}

type StatsModel struct {
	db    *sql.DB
	stats db.ReviewStats
	ready bool
	err   string
}

func NewStatsModel(database *sql.DB) StatsModel {
	return StatsModel{db: database}
}

func (m StatsModel) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		s, err := db.GetReviewStats(database)
		return msgStatsLoaded{stats: s, err: err}
	}
}

func (m StatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgStatsLoaded:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.stats = msg.stats
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
	b.WriteString("\n\n")

	// Card breakdown
	b.WriteString(subtitleStyle.Render("Cards"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render(fmt.Sprintf("  Total: %d   Mature: %d   Learning: %d   New: %d",
		s.TotalCards, s.MatureCards, s.LearningCards, s.NewCards)))
	b.WriteString("\n\n")

	// Today
	b.WriteString(subtitleStyle.Render("Today"))
	b.WriteString("\n")
	correctRate := ""
	if s.ReviewedToday > 0 {
		rate := float64(s.CorrectToday) / float64(s.ReviewedToday) * 100
		correctRate = fmt.Sprintf("   Correct: %d / %d (%.0f%%)", s.CorrectToday, s.ReviewedToday, rate)
	}
	b.WriteString(labelStyle.Render(fmt.Sprintf("  Reviewed: %d / %d%s",
		s.ReviewedToday, dailyReviewLimit, correctRate)))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("[esc] back"))
	return b.String()
}
