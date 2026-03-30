package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/ai"
	"github.com/ippei/poc-anki-claude/db"
)

type fetchState int

const (
	fetchStateIdle fetchState = iota
	fetchStateLoading
	fetchStatePreview
	fetchStateSaved
	fetchStateError
	fetchStateNoAI
)

type msgFetchResult struct {
	front string
	back  string
	hint  string
	err   error
}

type msgFetchSaved struct{ err error }

type FetchModel struct {
	db      *sql.DB
	ai      ai.Client
	state   fetchState
	input   textinput.Model
	spinner spinner.Model
	front   string
	back    string
	hint    string
	errMsg  string
}

func NewFetchModel(database *sql.DB, aiClient ai.Client) FetchModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. Spanish top 10 words"
	ti.CharLimit = 256

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = subtitleStyle

	m := FetchModel{
		db:      database,
		ai:      aiClient,
		input:   ti,
		spinner: sp,
	}
	if aiClient == nil {
		m.state = fetchStateNoAI
	} else {
		m.state = fetchStateIdle
	}
	return m
}

func (m FetchModel) Init() tea.Cmd {
	if m.state == fetchStateNoAI {
		return nil
	}
	return m.input.Focus()
}

func (m FetchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.state == fetchStateLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case msgFetchResult:
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = msg.err.Error()
		} else {
			m.state = fetchStatePreview
			m.front = msg.front
			m.back = msg.back
			m.hint = msg.hint
		}
		return m, nil

	case msgFetchSaved:
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = msg.err.Error()
		} else {
			m.state = fetchStateSaved
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == fetchStateIdle {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m FetchModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case fetchStateNoAI:
		if msg.String() == "enter" || msg.String() == "esc" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchStateIdle:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			topic := strings.TrimSpace(m.input.Value())
			if topic == "" {
				return m, nil
			}
			m.state = fetchStateLoading
			aiClient := m.ai
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					front, back, hint, err := aiClient.GenerateCard(context.Background(), topic)
					return msgFetchResult{front: front, back: back, hint: hint, err: err}
				},
			)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case fetchStatePreview:
		switch msg.String() {
		case "y", "enter":
			return m, m.saveCard()
		case "n", "esc":
			// reset to idle for another try
			m.state = fetchStateIdle
			m.input.SetValue("")
			return m, m.input.Focus()
		}

	case fetchStateSaved:
		if msg.String() == "enter" || msg.String() == " " {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchStateError:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.state = fetchStateIdle
			m.input.SetValue("")
			return m, m.input.Focus()
		}
	}
	return m, nil
}

func (m FetchModel) saveCard() tea.Cmd {
	front, back, hint := m.front, m.back, m.hint
	database := m.db
	return func() tea.Msg {
		id, err := db.CreateCard(database, front, back, hint)
		if err != nil {
			return msgFetchSaved{err: err}
		}
		_, _ = db.GetOrCreateReview(database, id)
		return msgFetchSaved{}
	}
}

func (m FetchModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Fetch with AI"))
	b.WriteString("\n\n")

	switch m.state {
	case fetchStateNoAI:
		b.WriteString(errorStyle.Render("AI not configured. Set AI_BACKEND and credentials."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchStateIdle:
		b.WriteString(inputLabelStyle.Render("Topic:"))
		b.WriteString("\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] generate  [esc] cancel"))

	case fetchStateLoading:
		b.WriteString(m.spinner.View() + " Generating card...")

	case fetchStatePreview:
		b.WriteString(subtitleStyle.Render("Generated card preview:"))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("Front: ") + m.front)
		b.WriteString("\n")
		b.WriteString(inputLabelStyle.Render("Back:  ") + m.back)
		b.WriteString("\n")
		if m.hint != "" {
			b.WriteString(inputLabelStyle.Render("Hint:  ") + m.hint)
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%s save  %s try again",
			keyStyle.Render("[y/enter]"),
			keyStyle.Render("[n/esc]"),
		))

	case fetchStateSaved:
		b.WriteString(successStyle.Render("Card saved!"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchStateError:
		b.WriteString(errorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] try again"))
	}

	return b.String()
}
