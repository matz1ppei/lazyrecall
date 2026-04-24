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
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
)

type braindumpState int

const (
	braindumpStateInput  braindumpState = iota // waiting for comma-separated input
	braindumpStateResult                       // showing match count
)

// BrainDumpModel collects free recall from the learner and reports how many
// cards they correctly identified, without affecting FSRS scheduling.
type BrainDumpModel struct {
	cards      []db.Card
	input      textinput.Model
	state      braindumpState
	matched    []bool // matched[i] = true if cards[i] was recalled
	matchCount int
	totalCount int
	onComplete tea.Cmd
	label      string // "Brain Dump 1", "Brain Dump 2", or "Brain Dump 3"
	hints      string // comma-separated first letters; empty = no hint
}

func NewBrainDumpModel(cards []db.Card, label string, hints string, onComplete tea.Cmd) BrainDumpModel {
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
		hints:      hints,
	}
}

// wordShapeHint returns a masked representation of a word showing the first and last
// letter with underscores for middle letters (e.g. "hola" → "h__a", "hacer" → "h___r").
// Words of 1–2 runes are returned as-is.
func wordShapeHint(word string) string {
	r := []rune(word)
	if len(r) <= 2 {
		return word
	}
	mid := strings.Repeat("_", len(r)-2)
	return string(r[0]) + mid + string(r[len(r)-1])
}

// wordShapeHints returns a comma-separated string of wordShapeHint for each card's Front.
// Used for BD1 to show first+last letter with masked middle.
func wordShapeHints(cards []db.Card) string {
	var hints []string
	for _, c := range cards {
		hints = append(hints, wordShapeHint(c.Front))
	}
	return strings.Join(hints, ", ")
}

// firstLetterHints returns a comma-separated string of the first letter (rune) of each
// card's Front. If excludeMatched is non-nil, cards where excludeMatched[i]==true are skipped.
// BD2 passes BD1's matched slice to show only unrecalled cards.
func firstLetterHints(cards []db.Card, excludeMatched []bool) string {
	var letters []string
	for i, c := range cards {
		if excludeMatched != nil && i < len(excludeMatched) && excludeMatched[i] {
			continue
		}
		r := []rune(c.Front)
		if len(r) > 0 {
			letters = append(letters, string(r[0]))
		}
	}
	return strings.Join(letters, ", ")
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
				m.matched, m.matchCount = scoreInput(m.input.Value(), m.cards)
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

// scoreInput splits the raw input by commas, trims whitespace, and checks
// which cards were recalled (case-insensitive match against Front).
// Returns a bool slice (matched[i] = true if cards[i] was recalled) and the count.
// Index-based deduplication is used instead of card ID because test cards
// (and any future in-memory cards) may share ID = 0.
func scoreInput(raw string, cards []db.Card) ([]bool, int) {
	matched := make([]bool, len(cards))
	if strings.TrimSpace(raw) == "" {
		return matched, 0
	}
	tokens := strings.Split(raw, ",")
	count := 0
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		for i, c := range cards {
			if matched[i] {
				continue
			}
			if ai.MatchAnswer(tok, c.Front) {
				matched[i] = true
				count++
				break
			}
		}
	}
	return matched, count
}

func (m BrainDumpModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(m.label))
	b.WriteString("\n\n")

	switch m.state {
	case braindumpStateInput:
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Type as many of the %d words as you can remember (comma-separated):", m.totalCount)))
		b.WriteString("\n\n")
		if m.hints != "" {
			b.WriteString(helpStyle.Render("Hints: " + m.hints))
			b.WriteString("\n")
		}
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] submit"))

	case braindumpStateResult:
		if m.matchCount == m.totalCount {
			b.WriteString(successStyle.Render(fmt.Sprintf("Perfect! All %d words recalled!", m.totalCount)))
		} else {
			b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d words recalled!", m.matchCount, m.totalCount)))
		}
		b.WriteString("\n\n")
		for i, c := range m.cards {
			if m.matched[i] {
				b.WriteString(successStyle.Render("✓ " + c.Front))
			} else {
				b.WriteString(errorStyle.Render("✗ " + c.Front))
			}
			b.WriteString("  ")
			b.WriteString(helpStyle.Render(c.Back))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] continue"))
	}

	return b.String()
}
