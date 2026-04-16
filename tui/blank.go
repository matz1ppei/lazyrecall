package tui

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

type blankState int

const (
	blankStateLoading blankState = iota
	blankStatePlaying
	blankStateResult
	blankStateComplete
	blankStateEmpty
)

type msgBlankCards []db.Card
type msgBlankResultReset struct{}

type BlankModel struct {
	db             *sql.DB
	state          blankState
	cards          []db.Card
	current        int
	correct        int
	correctIDs     []int64
	showHint       bool
	lastCorrect    bool
	lastAnswer     string
	input          textinput.Model
	preloadedCards []db.Card
	sessionMode    bool
	onComplete     tea.Cmd
}

func NewBlankModel(database *sql.DB) BlankModel {
	ti := textinput.New()
	ti.Placeholder = "Type the word..."
	ti.CharLimit = 256
	return BlankModel{
		db:    database,
		state: blankStateLoading,
		input: ti,
	}
}

func NewBlankModelWithCards(database *sql.DB, cards []db.Card, onComplete tea.Cmd) BlankModel {
	m := NewBlankModel(database)
	m.preloadedCards = cards
	m.sessionMode = true
	m.onComplete = onComplete
	return m
}

func (m BlankModel) Init() tea.Cmd {
	if m.preloadedCards != nil {
		cards := m.preloadedCards
		return func() tea.Msg {
			var eligible []db.Card
			for _, c := range cards {
				if canBlank(c) {
					eligible = append(eligible, c)
				}
			}
			return msgBlankCards(eligible)
		}
	}
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListCardsWithTranslation(database)
		if err != nil || len(cards) == 0 {
			return msgBlankCards(nil)
		}
		excluded, _ := config.LoadExcludedWords()
		var eligible []db.Card
		for _, c := range cards {
			if !excluded[strings.ToLower(c.Front)] && canBlank(c) {
				eligible = append(eligible, c)
			}
		}
		return msgBlankCards(eligible)
	}
}

// canBlank returns true when the card has enough data to show a fill-in-the-blank
// exercise. If example_word is set, it is used as the blank target (allows
// conjugated forms). Otherwise falls back to checking that the base form (front)
// appears in the example as a whole word.
func canBlank(c db.Card) bool {
	if c.Example == "" || c.ExampleTranslation == "" {
		return false
	}
	word := c.Front
	if c.ExampleWord != "" {
		word = c.ExampleWord
	}
	return blankSentence(c.Example, word) != c.Example
}

// blankWord returns the word to blank out in the example sentence.
// Uses example_word (conjugated form) if available, falls back to front.
func blankWord(c db.Card) string {
	if c.ExampleWord != "" {
		return c.ExampleWord
	}
	return c.Front
}

func (m BlankModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgBlankCards:
		if len(msg) == 0 {
			m.state = blankStateEmpty
			return m, nil
		}
		m.cards = []db.Card(msg)
		m.current = 0
		m.correct = 0
		m.state = blankStatePlaying
		m.input.Reset()
		return m, m.input.Focus()

	case msgBlankResultReset:
		m.current++
		if m.current >= len(m.cards) {
			m.state = blankStateComplete
			return m, nil
		}
		m.state = blankStatePlaying
		m.showHint = false
		m.input.Reset()
		return m, m.input.Focus()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == blankStatePlaying {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m BlankModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case blankStateEmpty:
		if msg.String() == "esc" || msg.String() == "enter" {
			if m.onComplete != nil {
				return m, m.onComplete
			}
			return m, func() tea.Msg {
				return MsgGotoScreen{Target: screenHome, Reason: "Blank fill skipped: no cards with example sentences and translations"}
			}
		}

	case blankStatePlaying:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "ctrl+h":
			m.showHint = true
			return m, nil
		case "enter":
			answer := strings.TrimSpace(m.input.Value())
			card := m.cards[m.current]
			m.lastAnswer = answer
			m.lastCorrect = ai.MatchAnswer(answer, blankWord(card))
			if m.lastCorrect {
				m.correct++
				m.correctIDs = append(m.correctIDs, card.ID)
			}
			m.state = blankStateResult
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return msgBlankResultReset{}
			})
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case blankStateResult:
		// waiting for auto-advance timer; ignore keys

	case blankStateComplete:
		switch msg.String() {
		case "esc", "enter":
			if m.onComplete != nil {
				return m, m.onComplete
			}
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "r":
			if m.sessionMode {
				return m, nil
			}
			return NewBlankModel(m.db), NewBlankModel(m.db).Init()
		}
	}
	return m, nil
}

// blankSentence replaces the target word in the example with underscores.
// Matches whole words only: the word must be preceded and followed by a
// non-letter/non-digit character (or start/end of string), so "a" in "cara"
// is left untouched while standalone "a" is blanked.
// Uses \pL (Unicode letter) to correctly handle accented chars like é, ñ.
func blankSentence(example, front string) string {
	pattern := `(?i)(^|[^\pL\d])` + regexp.QuoteMeta(front) + `($|[^\pL\d])`
	re := regexp.MustCompile(pattern)
	blanks := strings.Repeat("_", len([]rune(front)))
	return re.ReplaceAllString(example, `${1}`+blanks+`${2}`)
}

func (m BlankModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Fill in the Blank"))
	b.WriteString("\n\n")

	switch m.state {
	case blankStateLoading:
		b.WriteString(subtitleStyle.Render("Loading..."))

	case blankStateEmpty:
		b.WriteString(errorStyle.Render("No cards with translations. Use [g] on home to generate."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))

	case blankStatePlaying:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(blankSentence(card.Example, blankWord(card))))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(card.ExampleTranslation))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		if m.showHint {
			b.WriteString("\n")
			b.WriteString(hintStyle.Render("Hint: " + card.Back))
		} else {
			b.WriteString("\n")
			b.WriteString(helpStyle.Render("[ctrl+h] hint  [esc] quit"))
		}

	case blankStateResult:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(card.Example))
		b.WriteString("\n\n")
		if m.lastCorrect {
			b.WriteString(successStyle.Render("✓ " + blankWord(card)))
		} else {
			b.WriteString(errorStyle.Render("✗ " + m.lastAnswer))
			b.WriteString(" → ")
			b.WriteString(successStyle.Render(blankWord(card)))
		}

	case blankStateComplete:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d / %d correct", m.correct, len(m.cards))))
		b.WriteString("\n\n")
		if m.sessionMode {
			b.WriteString(helpStyle.Render("[enter] continue"))
		} else {
			b.WriteString(helpStyle.Render("[enter] back to home  [r] play again"))
		}
	}

	return b.String()
}
