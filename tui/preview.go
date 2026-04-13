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
	cursor     int // scroll offset for pagination
	done       bool
	onComplete tea.Cmd
	startTime  time.Time
}

const previewPageSize = 15

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
		case "down", "j":
			if m.cursor+previewPageSize < len(m.cards) {
				m.cursor += previewPageSize
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor -= previewPageSize
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
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

	// Table header
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("  %-30s %s", "Front", "Back")))
	b.WriteString("\n")

	end := m.cursor + previewPageSize
	if end > len(m.cards) {
		end = len(m.cards)
	}
	for i := m.cursor; i < end; i++ {
		c := m.cards[i]
		line := fmt.Sprintf("  %-30s %s", truncate(c.Front, 30), truncate(c.Back, 40))
		b.WriteString(labelStyle.Render(line))
		b.WriteString("\n")
	}

	if len(m.cards) > previewPageSize {
		total := len(m.cards)
		page := m.cursor/previewPageSize + 1
		pages := (total + previewPageSize - 1) / previewPageSize
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Page %d/%d", page, pages)))
		b.WriteString("\n")
	}

	elapsed := time.Since(m.startTime)
	remaining := 5 - int(elapsed.Seconds())
	if remaining < 0 {
		remaining = 0
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("[enter] skip  [↑↓/jk] scroll  auto-advance in %ds", remaining)))
	return b.String()
}
