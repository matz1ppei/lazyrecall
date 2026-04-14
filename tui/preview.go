package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

// msgPreviewTick is sent after the 5-second auto-advance timer fires.
type msgPreviewTick struct{}

// PreviewModel shows all session cards (front + back) before the session begins.
// Its purpose is to lower cognitive load by letting the learner survey the material first.
type PreviewModel struct {
	cards      []db.CardWithReview
	done       bool
	onComplete tea.Cmd
	startTime  time.Time
}

func NewPreviewModel(cards []db.CardWithReview, onComplete tea.Cmd) PreviewModel {
	return PreviewModel{
		cards:      cards,
		onComplete: onComplete,
		startTime:  time.Now(),
	}
}

func (m PreviewModel) Init() tea.Cmd {
	// Schedule auto-advance after 5 seconds so users who don't press Enter
	// still proceed automatically without interrupting the reading flow.
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return msgPreviewTick{}
	})
}

func (m PreviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgPreviewTick:
		if !m.done {
			m.done = true
			return m, m.onComplete
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.done {
				m.done = true
				return m, m.onComplete
			}
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}
	}
	return m, nil
}

func (m PreviewModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Preview — Today's Cards"))
	b.WriteString("\n\n")

	if len(m.cards) == 0 {
		b.WriteString(subtitleStyle.Render("No cards to preview."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] continue"))
		return b.String()
	}

	b.WriteString(subtitleStyle.Render(fmt.Sprintf("  %-30s %s", "Front", "Back")))
	b.WriteString("\n")

	for _, c := range m.cards {
		line := fmt.Sprintf("  %-30s %s", truncate(c.Front, 30), truncate(c.Back, 40))
		b.WriteString(labelStyle.Render(line))
		b.WriteString("\n")
	}

	elapsed := time.Since(m.startTime)
	remaining := 5 - int(elapsed.Seconds())
	if remaining < 0 {
		remaining = 0
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("[enter] skip  auto-advance in %ds", remaining)))
	return b.String()
}
