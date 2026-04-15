package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

type listState int

const (
	listStateLoading listState = iota
	listStateNormal
	listStateConfirmDelete
	listStateConfirmExclude
	listStateEdit
	listStateEmpty
)

type msgListCards struct {
	cards []db.CardWithReview
}

type msgDeleteDone struct{ err error }
type msgUpdateDone struct{ err error }
type msgExcludeDone struct{ err error }

type msgEditGenerated struct {
	text        string
	translation string
	err         error
}

type msgExcludedWords struct{ excluded map[string]bool }

type ListModel struct {
	db              *sql.DB
	ai              ai.Client
	state           listState
	cards           []db.CardWithReview
	excluded        map[string]bool
	cursor          int
	offset          int
	errMsg          string
	editInputs      [4]textinput.Model // front, back, hint, example
	editFocus       int
	editLoading     bool
	editTranslation string
}

const listPageSize = 15

func NewListModel(database *sql.DB, aiClient ai.Client) ListModel {
	return ListModel{db: database, ai: aiClient, state: listStateLoading}
}

func loadExcludedCmd() tea.Cmd {
	return func() tea.Msg {
		excluded, _ := config.LoadExcludedWords()
		return msgExcludedWords{excluded: excluded}
	}
}

func (m ListModel) Init() tea.Cmd {
	database := m.db
	return tea.Batch(
		func() tea.Msg {
			cards, err := db.ListAllCardsWithReview(database)
			if err != nil {
				return msgListCards{cards: nil}
			}
			return msgListCards{cards: cards}
		},
		loadExcludedCmd(),
	)
}

func (m ListModel) reloadCmd() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListAllCardsWithReview(database)
		if err != nil {
			return msgListCards{cards: nil}
		}
		return msgListCards{cards: cards}
	}
}

func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgListCards:
		m.cards = msg.cards
		if len(m.cards) == 0 {
			m.state = listStateEmpty
		} else {
			m.state = listStateNormal
		}
		return m, nil

	case msgDeleteDone:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.state = listStateNormal
			return m, nil
		}
		return m, m.reloadCmd()

	case msgUpdateDone:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.state = listStateNormal
			return m, nil
		}
		return m, m.reloadCmd()

	case msgExcludedWords:
		m.excluded = msg.excluded
		return m, nil

	case msgExcludeDone:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		}
		m.state = listStateNormal
		return m, loadExcludedCmd()

	case msgEditGenerated:
		m.editLoading = false
		if msg.err != nil {
			m.errMsg = "AI error: " + msg.err.Error()
		} else {
			m.editInputs[m.editFocus].SetValue(msg.text)
			if msg.translation != "" {
				m.editTranslation = msg.translation
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to focused edit input
	if m.state == listStateEdit {
		var cmd tea.Cmd
		m.editInputs[m.editFocus], cmd = m.editInputs[m.editFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *ListModel) initEditInputs(card db.CardWithReview) {
	labels := []string{"Front", "Back", "Hint", "Example"}
	values := []string{card.Front, card.Back, card.Hint, card.Example}
	for i := range m.editInputs {
		ti := textinput.New()
		ti.Placeholder = labels[i]
		ti.CharLimit = 512
		ti.SetValue(values[i])
		m.editInputs[i] = ti
	}
	m.editFocus = 0
	m.editInputs[0].Focus()
	m.editTranslation = card.Card.ExampleTranslation
}

func (m ListModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case listStateEmpty:
		if msg.String() == "esc" || msg.String() == "q" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case listStateNormal:
		total := len(m.cards)
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset--
				}
			}
		case "down", "j":
			if m.cursor < total-1 {
				m.cursor++
				if m.cursor >= m.offset+listPageSize {
					m.offset++
				}
			}
		case "right", "l":
			nextOffset := m.offset + listPageSize
			if nextOffset < total {
				m.offset = nextOffset
				m.cursor = m.offset
			}
		case "left", "h":
			if m.offset > 0 {
				m.offset -= listPageSize
				if m.offset < 0 {
					m.offset = 0
				}
				m.cursor = m.offset
			}
		case "e", "enter":
			m.initEditInputs(m.cards[m.cursor])
			m.state = listStateEdit
			m.errMsg = ""
			return m, textinput.Blink
		case "d":
			m.state = listStateConfirmDelete
			m.errMsg = ""
		case "x":
			m.state = listStateConfirmExclude
			m.errMsg = ""
		}

	case listStateConfirmExclude:
		switch msg.String() {
		case "y", "enter":
			word := m.cards[m.cursor].Front
			return m, func() tea.Msg {
				err := config.AppendExcludedWord(word)
				return msgExcludeDone{err: err}
			}
		case "n", "esc":
			m.state = listStateNormal
		}

	case listStateConfirmDelete:
		switch msg.String() {
		case "y", "enter":
			card := m.cards[m.cursor]
			database := m.db
			if m.cursor > 0 && m.cursor >= len(m.cards)-1 {
				m.cursor--
				if m.offset > 0 {
					m.offset--
				}
			}
			m.state = listStateLoading
			return m, func() tea.Msg {
				err := db.DeleteCard(database, card.Card.ID)
				return msgDeleteDone{err: err}
			}
		case "n", "esc":
			m.state = listStateNormal
		}

	case listStateEdit:
		switch msg.String() {
		case "esc":
			m.state = listStateNormal
			return m, nil
		case "tab", "down":
			m.editInputs[m.editFocus].Blur()
			m.editFocus = (m.editFocus + 1) % 4
			return m, m.editInputs[m.editFocus].Focus()
		case "shift+tab", "up":
			m.editInputs[m.editFocus].Blur()
			m.editFocus = (m.editFocus + 3) % 4
			return m, m.editInputs[m.editFocus].Focus()
		case "ctrl+g":
			// AI generate for Hint (index 2) or Example (index 3)
			if (m.editFocus == 2 || m.editFocus == 3) && m.ai != nil && !m.editLoading {
				m.editLoading = true
				front := m.editInputs[0].Value()
				back := m.editInputs[1].Value()
				aiClient := m.ai
				focus := m.editFocus
				return m, func() tea.Msg {
					if focus == 3 {
						example, translation, err := aiClient.GenerateExample(context.Background(), front, back)
						return msgEditGenerated{text: example, translation: translation, err: err}
					}
					text, err := aiClient.GenerateHint(context.Background(), front, back)
					return msgEditGenerated{text: text, err: err}
				}
			}
		case "ctrl+s", "enter":
			if m.editFocus < 3 {
				// Tab to next field on enter (except last)
				m.editInputs[m.editFocus].Blur()
				m.editFocus++
				return m, m.editInputs[m.editFocus].Focus()
			}
			// Save on enter at last field
			card := m.cards[m.cursor]
			front := strings.TrimSpace(m.editInputs[0].Value())
			back := strings.TrimSpace(m.editInputs[1].Value())
			hint := strings.TrimSpace(m.editInputs[2].Value())
			example := strings.TrimSpace(m.editInputs[3].Value())
			if front == "" || back == "" {
				m.errMsg = "Front and Back cannot be empty"
				return m, nil
			}
			database := m.db
			id := card.Card.ID
			editTranslation := m.editTranslation
			m.state = listStateLoading
			return m, func() tea.Msg {
				err := db.UpdateCard(database, id, front, back, hint, example, editTranslation)
				return msgUpdateDone{err: err}
			}
		default:
			var cmd tea.Cmd
			m.editInputs[m.editFocus], cmd = m.editInputs[m.editFocus].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func (m ListModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Card List"))
	b.WriteString("\n\n")

	switch m.state {
	case listStateLoading:
		b.WriteString(subtitleStyle.Render("Loading..."))

	case listStateEmpty:
		b.WriteString(subtitleStyle.Render("No cards registered yet."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))

	case listStateEdit:
		card := m.cards[m.cursor]
		b.WriteString(labelStyle.Render(fmt.Sprintf("Editing card #%d", card.Card.ID)))
		b.WriteString("\n\n")

		fieldNames := []string{"Front", "Back", "Hint", "Example"}
		for i, name := range fieldNames {
			if i == m.editFocus {
				b.WriteString(inputLabelStyle.Render(name + ":"))
			} else {
				b.WriteString(labelStyle.Render(name + ":"))
			}
			b.WriteString("\n")
			b.WriteString(m.editInputs[i].View())
			if i == m.editFocus && (i == 2 || i == 3) && m.ai != nil {
				if m.editLoading {
					b.WriteString("  " + subtitleStyle.Render("Generating..."))
				} else {
					b.WriteString("  " + helpStyle.Render("[ctrl+g] generate with AI"))
				}
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[tab/↑↓] move  [enter] next/save  [ctrl+s] save  [esc] cancel"))
		if m.errMsg != "" {
			b.WriteString("\n" + errorStyle.Render(m.errMsg))
		}

	case listStateNormal, listStateConfirmDelete, listStateConfirmExclude:
		total := len(m.cards)
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d cards total", total)))
		b.WriteString("\n\n")

		b.WriteString(subtitleStyle.Render(fmt.Sprintf("  %-4s %-3s %-12s %-20s %-30s %s", "ID", " x", "Front", "Back", "Example", "Due")))
		b.WriteString("\n")

		end := m.offset + listPageSize
		if end > total {
			end = total
		}
		for i := m.offset; i < end; i++ {
			c := m.cards[i]
			mark := "   "
			if m.excluded[strings.ToLower(c.Front)] {
				mark = "[x]"
			}
			line := fmt.Sprintf("%-4d %s %-12s %-20s %-30s %s",
				c.Card.ID,
				mark,
				truncate(c.Front, 12),
				truncate(c.Back, 20),
				truncate(c.Example, 30),
				c.Review.DueDate,
			)
			if i == m.cursor {
				b.WriteString(inputLabelStyle.Render("> " + line))
			} else {
				b.WriteString(menuItemStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}

		if total > listPageSize {
			b.WriteString(helpStyle.Render(fmt.Sprintf("(%d-%d / %d)", m.offset+1, end, total)))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		switch m.state {
		case listStateConfirmDelete:
			card := m.cards[m.cursor]
			b.WriteString(errorStyle.Render(fmt.Sprintf("Delete \"%s\"? ", truncate(card.Front, 30))))
			b.WriteString(helpStyle.Render("[y/enter] yes  [n/esc] no"))
		case listStateConfirmExclude:
			card := m.cards[m.cursor]
			b.WriteString(subtitleStyle.Render(fmt.Sprintf("Add \"%s\" to exclusion list? ", truncate(card.Front, 30))))
			b.WriteString(helpStyle.Render("[y/enter] yes  [n/esc] no"))
		default:
			b.WriteString(helpStyle.Render("[↑/↓] scroll  [←/→] page  [e/enter] edit  [d] delete  [x] exclude  [esc] back"))
		}

		if m.errMsg != "" {
			b.WriteString("\n" + errorStyle.Render("Error: "+m.errMsg))
		}
	}

	return b.String()
}
