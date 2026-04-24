package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
)

type composeState int

const (
	composeStateLoading composeState = iota
	composeStateNoAI
	composeStatePlaying    // showing translation, waiting for input
	composeStateEvaluating // AI call in progress
	composeStateResult     // showing feedback
	composeStateComplete
	composeStateEmpty
)

type msgComposeCards []db.Card
type msgComposeNoAI struct{}
type msgComposeEval struct {
	feedback string
	ok       bool
	err      error
}

type ComposeModel struct {
	db           *sql.DB
	ai           ai.Client
	feedbackLang string
	state        composeState
	cards        []db.Card
	current      int
	correct      int
	input        textinput.Model
	width        int
	// last result
	lastInput    string
	lastFeedback string
	lastOK       bool
	lastErr      string
}

func NewComposeModel(database *sql.DB, aiClient ai.Client, width int, feedbackLang string) ComposeModel {
	ti := textinput.New()
	ti.Placeholder = "Type the original sentence..."
	ti.CharLimit = 512
	return ComposeModel{
		db:           database,
		ai:           aiClient,
		feedbackLang: feedbackLang,
		state:        composeStateLoading,
		input:        ti,
		width:        width,
	}
}

func (m ComposeModel) Init() tea.Cmd {
	if m.ai == nil {
		return func() tea.Msg { return msgComposeNoAI{} }
	}
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListCardsWithTranslation(database)
		if err != nil {
			return msgComposeCards(nil)
		}
		return msgComposeCards(cards)
	}
}

func (m ComposeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case msgComposeNoAI:
		m.state = composeStateNoAI
		return m, nil
	case msgComposeCards:
		if len(msg) == 0 {
			m.state = composeStateEmpty
			return m, nil
		}
		m.cards = []db.Card(msg)
		m.current = 0
		m.correct = 0
		m.state = composeStatePlaying
		m.input.Reset()
		return m, m.input.Focus()

	case msgComposeEval:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			m.lastFeedback = msg.feedback
			m.lastOK = msg.ok
			m.lastErr = ""
			if msg.ok {
				m.correct++
			}
		}
		m.state = composeStateResult
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == composeStatePlaying {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m ComposeModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case composeStateNoAI:
		if msg.String() == "esc" || msg.String() == "enter" {
			return m, func() tea.Msg {
				return MsgGotoScreen{Target: screenHome, Reason: "Compose requires AI configuration"}
			}
		}

	case composeStateEmpty:
		if msg.String() == "esc" || msg.String() == "enter" {
			return m, func() tea.Msg {
				return MsgGotoScreen{Target: screenHome, Reason: "Compose skipped: no cards with example translations"}
			}
		}

	case composeStatePlaying:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			userSentence := strings.TrimSpace(m.input.Value())
			if userSentence == "" {
				return m, nil
			}
			m.lastInput = userSentence
			card := m.cards[m.current]
			aiClient := m.ai
			m.state = composeStateEvaluating
			feedbackLang := m.feedbackLang
			return m, func() tea.Msg {
				feedback, ok, err := aiClient.EvaluateTranslation(
					context.Background(), card.Front, card.Back, card.Example, userSentence, feedbackLang,
				)
				return msgComposeEval{feedback: feedback, ok: ok, err: err}
			}
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case composeStateEvaluating:
		// waiting for AI; ignore keys

	case composeStateResult:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter", " ":
			m.current++
			if m.current >= len(m.cards) {
				m.state = composeStateComplete
				return m, nil
			}
			m.state = composeStatePlaying
			m.lastFeedback = ""
			m.lastErr = ""
			m.input.Reset()
			return m, m.input.Focus()
		}

	case composeStateComplete:
		switch msg.String() {
		case "esc", "enter":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "r":
			fresh := NewComposeModel(m.db, m.ai, m.width, m.feedbackLang)
			return fresh, fresh.Init()
		}
	}
	return m, nil
}

func (m ComposeModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Compose"))
	b.WriteString("\n\n")

	switch m.state {
	case composeStateLoading:
		b.WriteString(subtitleStyle.Render("Loading..."))

	case composeStateNoAI:
		b.WriteString(errorStyle.Render("AI not configured. Compose requires an AI backend."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back"))

	case composeStateEmpty:
		b.WriteString(errorStyle.Render("No cards with example translations. Use [g] on home to generate."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))

	case composeStatePlaying:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("Word:  ") + subtitleStyle.Render(card.Front) + "  " + helpStyle.Render(card.Back))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("Translate into the original language:"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(card.ExampleTranslation))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] submit  [esc] quit"))

	case composeStateEvaluating:
		b.WriteString(subtitleStyle.Render("Evaluating..."))

	case composeStateResult:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		wrapWidth := m.width - 2
		if wrapWidth < 40 {
			wrapWidth = 40
		}
		wrapStyle := lipgloss.NewStyle().Width(wrapWidth)
		if m.lastErr != "" {
			b.WriteString(errorStyle.Render("AI error: " + m.lastErr))
		} else {
			if m.lastOK {
				b.WriteString(successStyle.Render("✓ Correct"))
			} else {
				b.WriteString(errorStyle.Render("✗ Needs improvement"))
			}
			b.WriteString("\n\n")
			b.WriteString(inputLabelStyle.Render("Feedback:"))
			b.WriteString("\n")
			b.WriteString(wrapStyle.Render(strings.ReplaceAll(m.lastFeedback, "\n", " ")))
			b.WriteString("\n\n")
			b.WriteString(inputLabelStyle.Render("Your answer:"))
			b.WriteString("\n")
			b.WriteString(wrapStyle.Foreground(lipgloss.Color("214")).Render(m.lastInput))
			b.WriteString("\n\n")
			b.WriteString(inputLabelStyle.Render("Original:"))
			b.WriteString("\n")
			b.WriteString(wrapStyle.Foreground(lipgloss.Color("250")).Render(card.Example))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] next  [esc] quit"))

	case composeStateComplete:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d / %d correct", m.correct, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home  [r] play again"))
	}

	return b.String()
}
