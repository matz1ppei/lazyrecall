package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
)

type addStep int

const (
	stepFront addStep = iota
	stepBack
	stepHint
	stepExample
	stepConfirm
)

type msgHintGenerated struct {
	hint        string
	translation string // set when hint is an example sentence
	exampleWord string // exact word form used in the example sentence
	err         error
}

type msgDupCheck struct {
	cards []db.Card
}

type AddModel struct {
	db                 *sql.DB
	ai                 ai.Client
	step               addStep
	inputs             [4]textinput.Model // front, back, hint, example
	exampleTranslation string
	exampleWord        string
	status             string
	loading            bool
	dupWarning         bool
	dupCards           []db.Card
}

func NewAddModel(database *sql.DB, aiClient ai.Client) AddModel {
	inputs := [4]textinput.Model{}
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].CharLimit = 512
	}
	inputs[0].Placeholder = "Front (question/word)"
	inputs[1].Placeholder = "Back (answer/meaning)"
	inputs[2].Placeholder = "Hint (optional)"
	inputs[3].Placeholder = "Example sentence (optional)"
	inputs[0].Focus()
	return AddModel{
		db:     database,
		ai:     aiClient,
		step:   stepFront,
		inputs: inputs,
	}
}

func (m AddModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m AddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgHintGenerated:
		m.loading = false
		if msg.err != nil {
			m.status = errorStyle.Render(fmt.Sprintf("AI error: %v", msg.err))
		} else {
			m.inputs[int(m.step)].SetValue(msg.hint)
			if msg.translation != "" {
				m.exampleTranslation = msg.translation
			}
			if msg.exampleWord != "" {
				m.exampleWord = msg.exampleWord
			}
			m.status = successStyle.Render("Generated!")
		}
		return m, nil

	case msgDupCheck:
		m.loading = false
		if len(msg.cards) > 0 {
			m.dupWarning = true
			m.dupCards = msg.cards
		} else {
			m.inputs[0].Blur()
			m.step = stepBack
			m.status = ""
			return m, m.inputs[1].Focus()
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
	// Duplicate warning: wait for [c] to continue or [esc] to edit front
	if m.dupWarning {
		switch msg.String() {
		case "c":
			m.dupWarning = false
			m.dupCards = nil
			m.inputs[0].Blur()
			m.step = stepBack
			m.status = ""
			return m, m.inputs[1].Focus()
		case "esc":
			m.dupWarning = false
			m.dupCards = nil
			m.status = ""
			return m, m.inputs[0].Focus()
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }

	case "enter":
		switch m.step {
		case stepFront:
			front := strings.TrimSpace(m.inputs[0].Value())
			if front == "" {
				m.status = errorStyle.Render("Front cannot be empty")
				return m, nil
			}
			m.loading = true
			m.status = subtitleStyle.Render("Checking for duplicates...")
			database := m.db
			return m, func() tea.Msg {
				cards, err := db.FindCardsByFront(database, front)
				if err != nil || len(cards) == 0 {
					return msgDupCheck{cards: nil}
				}
				return msgDupCheck{cards: cards}
			}

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
			m.step = stepExample
			m.status = ""
			return m, m.inputs[3].Focus()

		case stepExample:
			m.inputs[3].Blur()
			m.step = stepConfirm
			m.status = ""
			return m, nil

		case stepConfirm:
			return m, m.saveCard()
		}

	case "ctrl+g":
		if (m.step == stepHint || m.step == stepExample) && m.ai != nil && !m.loading {
			m.loading = true
			label := "hint"
			if m.step == stepExample {
				label = "example"
			}
			m.status = subtitleStyle.Render("Generating " + label + "...")
			front := m.inputs[0].Value()
			back := m.inputs[1].Value()
			aiClient := m.ai
			step := m.step
			return m, func() tea.Msg {
				if step == stepExample {
					example, translation, exampleWord, err := aiClient.GenerateExample(context.Background(), front, back)
					return msgHintGenerated{hint: example, translation: translation, exampleWord: exampleWord, err: err}
				}
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
	example := strings.TrimSpace(m.inputs[3].Value())
	exampleTranslation := m.exampleTranslation
	exampleWord := m.exampleWord
	database := m.db
	return func() tea.Msg {
		_, err := db.CreateCardWithReview(database, front, back, hint, example, exampleTranslation, exampleWord)
		if err != nil {
			// Surface error as a status message by returning to confirm step
			// We return a special message type here
			return msgSaveResult{err: err}
		}
		return MsgGotoScreen{Target: screenHome}
	}
}

type msgSaveResult struct{ err error }

func (m AddModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Add Card"))
	b.WriteString("\n\n")

	stepLabels := []string{"Front", "Back", "Hint", "Example"}
	for i, label := range stepLabels {
		if addStep(i) < m.step || m.step == stepConfirm {
			val := m.inputs[i].Value()
			if val == "" {
				val = "(empty)"
			}
			b.WriteString(labelStyle.Render(label+": ") + val)
			if addStep(i) == stepExample && m.exampleTranslation != "" && m.step == stepConfirm {
				b.WriteString("\n")
				b.WriteString(labelStyle.Render("Translation: ") + m.exampleTranslation)
			}
		} else if addStep(i) == m.step {
			b.WriteString(inputLabelStyle.Render(label + ":"))
			b.WriteString("\n")
			b.WriteString(m.inputs[i].View())
			if (i == int(stepHint) || i == int(stepExample)) && m.ai != nil {
				b.WriteString("\n" + helpStyle.Render("[ctrl+g] generate with AI"))
			}
		} else {
			b.WriteString(subtitleStyle.Render(label + ": ..."))
		}
		b.WriteString("\n")
	}

	if m.dupWarning {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("⚠ Duplicate found:"))
		b.WriteString("\n")
		for _, dup := range m.dupCards {
			b.WriteString(labelStyle.Render(fmt.Sprintf("  • %s → %s", dup.Front, dup.Back)))
			b.WriteString("\n")
		}
		b.WriteString(helpStyle.Render("[c] continue anyway  [esc] edit front"))
	} else if m.step == stepConfirm {
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
