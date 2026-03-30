package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/ai"
	"github.com/ippei/poc-anki-claude/db"
)

type addStep int

const (
	stepFront addStep = iota
	stepBack
	stepHint
	stepConfirm
)

type msgHintGenerated struct {
	hint string
	err  error
}

type AddModel struct {
	db       *sql.DB
	ai       ai.Client
	step     addStep
	inputs   [3]textinput.Model // front, back, hint
	status   string
	loading  bool
}

func NewAddModel(database *sql.DB, aiClient ai.Client) AddModel {
	inputs := [3]textinput.Model{}
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].CharLimit = 512
	}
	inputs[0].Placeholder = "Front (question/word)"
	inputs[1].Placeholder = "Back (answer/meaning)"
	inputs[2].Placeholder = "Hint (optional)"
	return AddModel{
		db:     database,
		ai:     aiClient,
		step:   stepFront,
		inputs: inputs,
	}
}

func (m AddModel) Init() tea.Cmd {
	return m.inputs[0].Focus()
}

func (m AddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgHintGenerated:
		m.loading = false
		if msg.err != nil {
			m.status = errorStyle.Render(fmt.Sprintf("AI error: %v", msg.err))
		} else {
			m.inputs[2].SetValue(msg.hint)
			m.status = successStyle.Render("Hint generated!")
		}
		return m, nil

	case msgSaveResult:
		m.status = errorStyle.Render(fmt.Sprintf("Save error: %v", msg.err))
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.step < stepConfirm {
		var cmd tea.Cmd
		m.inputs[int(m.step)], cmd = m.inputs[int(m.step)].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m AddModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }

	case "enter":
		switch m.step {
		case stepFront:
			if strings.TrimSpace(m.inputs[0].Value()) == "" {
				m.status = errorStyle.Render("Front cannot be empty")
				return m, nil
			}
			m.inputs[0].Blur()
			m.step = stepBack
			m.status = ""
			return m, m.inputs[1].Focus()

		case stepBack:
			if strings.TrimSpace(m.inputs[1].Value()) == "" {
				m.status = errorStyle.Render("Back cannot be empty")
				return m, nil
			}
			m.inputs[1].Blur()
			m.step = stepHint
			m.status = ""
			return m, m.inputs[2].Focus()

		case stepHint:
			m.inputs[2].Blur()
			m.step = stepConfirm
			m.status = ""
			return m, nil

		case stepConfirm:
			return m, m.saveCard()
		}

	case "g":
		if m.step == stepHint && m.ai != nil && !m.loading {
			m.loading = true
			m.status = subtitleStyle.Render("Generating hint...")
			front := m.inputs[0].Value()
			back := m.inputs[1].Value()
			aiClient := m.ai
			return m, func() tea.Msg {
				hint, err := aiClient.GenerateHint(context.Background(), front, back)
				return msgHintGenerated{hint: hint, err: err}
			}
		}

	case "y":
		if m.step == stepConfirm {
			return m, m.saveCard()
		}

	case "n":
		if m.step == stepConfirm {
			// go back to edit hint
			m.step = stepHint
			return m, m.inputs[2].Focus()
		}
	}

	// Forward key events to active input
	if m.step < stepConfirm {
		var cmd tea.Cmd
		m.inputs[int(m.step)], cmd = m.inputs[int(m.step)].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m AddModel) saveCard() tea.Cmd {
	front := strings.TrimSpace(m.inputs[0].Value())
	back := strings.TrimSpace(m.inputs[1].Value())
	hint := strings.TrimSpace(m.inputs[2].Value())
	database := m.db
	return func() tea.Msg {
		id, err := db.CreateCard(database, front, back, hint)
		if err != nil {
			// Surface error as a status message by returning to confirm step
			// We return a special message type here
			return msgSaveResult{err: err}
		}
		// Create the review row immediately so it shows in due list
		_, _ = db.GetOrCreateReview(database, id)
		return MsgGotoScreen{Target: screenHome}
	}
}

type msgSaveResult struct{ err error }

func (m AddModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Add Card"))
	b.WriteString("\n\n")

	stepLabels := []string{"Front", "Back", "Hint"}
	for i, label := range stepLabels {
		if addStep(i) < m.step || m.step == stepConfirm {
			val := m.inputs[i].Value()
			if val == "" {
				val = "(empty)"
			}
			b.WriteString(labelStyle.Render(label+": ") + val)
		} else if addStep(i) == m.step {
			b.WriteString(inputLabelStyle.Render(label+":"))
			b.WriteString("\n")
			b.WriteString(m.inputs[i].View())
			if i == int(stepHint) && m.ai != nil {
				b.WriteString("\n" + helpStyle.Render("[g] generate with AI"))
			}
		} else {
			b.WriteString(subtitleStyle.Render(label+": ..."))
		}
		b.WriteString("\n")
	}

	if m.step == stepConfirm {
		b.WriteString("\n")
		b.WriteString(successStyle.Render("Save this card?"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[y/enter] save  [n] edit hint  [esc] cancel"))
	} else if m.step < stepHint {
		b.WriteString("\n" + helpStyle.Render("[enter] next  [esc] cancel"))
	} else {
		b.WriteString("\n" + helpStyle.Render("[enter] next  [esc] cancel"))
	}

	if m.status != "" {
		b.WriteString("\n" + m.status)
	}

	return b.String()
}
