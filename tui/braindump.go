package tui

// braindump.go implements the Brain Dump phase: the learner types as many target-
// language words as they can recall in one go (comma-separated).
// The phase is intentionally not wired into FSRS scoring; its purpose is to
// build active recall habit without penalising imperfect free-recall.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/db"
)

type braindumpState int

const (
	braindumpStateInput  braindumpState = iota // waiting for comma-separated input
	braindumpStateResult                        // showing match count
)

// BrainDumpModel collects free recall from the learner and reports how many
// cards they correctly identified, without affecting FSRS scheduling.
type BrainDumpModel struct {
	cards      []db.Card
	input      textinput.Model
	state      braindumpState
	matchCount int
	totalCount int
	onComplete tea.Cmd
	label      string // "Brain Dump 1" or "Brain Dump 2"
}

func NewBrainDumpModel(cards []db.Card, label string, onComplete tea.Cmd) BrainDumpModel {
	ti := textinput.New()
	ti.Placeholder = "apple, banana, cherry..."
	ti.CharLimit = 1024
	ti.Focus()

	return BrainDumpModel{
		cards:      cards,
		input:      ti,
		state:      braindumpStateInput,
		totalCount: len(cards),
		onComplete: onComplete,
		label:      label,
	}
}

func (m BrainDumpModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m BrainDumpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case braindumpStateInput:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "enter" {
				m.matchCount = scoreInput(m.input.Value(), m.cards)
				m.state = braindumpStateResult
				return m, nil
			}
		}
		// Forward other messages to the text input.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case braindumpStateResult:
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "enter" || key.String() == " " {
				return m, m.onComplete
			}
		}
	}
	return m, nil
}

// scoreInput splits the raw input by commas, trims whitespace, and counts
// how many tokens match any card's Front (case-insensitive).
// Each card index is counted at most once to prevent inflated scores.
// Index-based deduplication is used instead of card ID because test cards
// (and any future in-memory cards) may share ID = 0.
func scoreInput(raw string, cards []db.Card) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	tokens := strings.Split(raw, ",")
	matchedIdx := make(map[int]bool)
	count := 0
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		for i, c := range cards {
			if matchedIdx[i] {
				continue
			}
			if strings.EqualFold(tok, c.Front) {
				matchedIdx[i] = true
				count++
				break
			}
		}
	}
	return count
}

func (m BrainDumpModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(m.label))
	b.WriteString("\n\n")

	switch m.state {
	case braindumpStateInput:
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Type as many of the %d words as you can remember (comma-separated):", m.totalCount)))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] submit"))

	case braindumpStateResult:
		if m.matchCount == m.totalCount {
			b.WriteString(successStyle.Render("Perfect! All words recalled!"))
		} else {
			b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d words recalled!", m.matchCount, m.totalCount)))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] continue"))
	}

	return b.String()
}
