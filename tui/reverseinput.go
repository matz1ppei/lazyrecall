package tui

// reverseinput.go implements Reverse Review as a free-text input phase.
// The learner sees the meaning (Back) and types the target-language word (Front).
// Matching uses ai.MatchAnswer so accent marks can be omitted.
// FSRS rating is handled by the session (correctIDs) or inline (standalone mode).

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/srs"
)

type reverseInputState int

const (
	reverseInputLoading reverseInputState = iota
	reverseInputQuestion
	reverseInputResult
	reverseInputSummary
	reverseInputEmpty
)

type msgReverseInputCards struct {
	cards []db.CardWithReview
}

type msgReverseInputResultReset struct{}

type ReverseInputModel struct {
	db             *sql.DB
	state          reverseInputState
	cards          []db.CardWithReview
	index          int
	correct        int
	input          textinput.Model
	lastCorrect    bool
	lastAnswer     string
	correctIDs     []int64
	preloadedCards []db.CardWithReview
	sessionMode    bool
	onComplete     tea.Cmd
}

func NewReverseInputModel(database *sql.DB) ReverseInputModel {
	return ReverseInputModel{
		db:    database,
		state: reverseInputLoading,
		input: newReverseTextInput(),
	}
}

func NewReverseInputModelWithCards(database *sql.DB, cards []db.CardWithReview, onComplete tea.Cmd) ReverseInputModel {
	return ReverseInputModel{
		db:             database,
		state:          reverseInputLoading,
		input:          newReverseTextInput(),
		preloadedCards: cards,
		sessionMode:    true,
		onComplete:     onComplete,
	}
}

func newReverseTextInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Type the word..."
	ti.CharLimit = 256
	return ti
}

func (m ReverseInputModel) Init() tea.Cmd {
	if len(m.preloadedCards) > 0 {
		cards := m.preloadedCards
		return func() tea.Msg { return msgReverseInputCards{cards: cards} }
	}
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListDueCards(database, reviewSessionSize)
		if err != nil || len(cards) == 0 {
			return msgReverseInputCards{cards: nil}
		}
		return msgReverseInputCards{cards: cards}
	}
}

func (m ReverseInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgReverseInputCards:
		m.cards = msg.cards
		m.index = 0
		m.correct = 0
		if len(m.cards) == 0 {
			m.state = reverseInputEmpty
			return m, nil
		}
		m.state = reverseInputQuestion
		m.input.Reset()
		return m, m.input.Focus()

	case msgReverseInputResultReset:
		m.index++
		if m.index >= len(m.cards) {
			m.state = reverseInputSummary
			return m, nil
		}
		m.state = reverseInputQuestion
		m.input.Reset()
		return m, m.input.Focus()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m ReverseInputModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case reverseInputQuestion:
		switch msg.String() {
		case "enter":
			answer := strings.TrimSpace(m.input.Value())
			card := m.cards[m.index]
			m.lastAnswer = answer
			m.lastCorrect = ai.MatchAnswer(answer, card.Front)
			if m.lastCorrect {
				m.correct++
				m.correctIDs = append(m.correctIDs, card.Card.ID)
			}
			m.state = reverseInputResult
			delay := 600 * time.Millisecond
			if !m.lastCorrect {
				delay = 1500 * time.Millisecond
			}
			tick := tea.Tick(delay, func(time.Time) tea.Msg {
				return msgReverseInputResultReset{}
			})
			if m.sessionMode {
				return m, tick
			}
			rating := 0
			if m.lastCorrect {
				rating = 4
			}
			return m, tea.Batch(m.rateCard(rating), tick)
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case reverseInputResult:
		// waiting for auto-advance timer; ignore keys

	case reverseInputSummary, reverseInputEmpty:
		if msg.String() == "enter" || msg.String() == " " || msg.String() == "esc" {
			if m.onComplete != nil {
				return m, m.onComplete
			}
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}
	}
	return m, nil
}

func (m ReverseInputModel) rateCard(rating int) tea.Cmd {
	card := m.cards[m.index]
	database := m.db
	return func() tea.Msg {
		current := db.ReviewToSRS(card.Review)
		result := srs.Schedule(current, srs.RatingFromSM2(rating), time.Now())
		updated := card.Review
		db.ApplySRSResult(&updated, result)
		updated.LastRating = &rating
		if err := db.UpdateReview(database, updated); err != nil {
			log.Printf("UpdateReview error: %v", err)
		}
		return nil
	}
}

func (m ReverseInputModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Reverse Review"))
	b.WriteString("\n\n")

	switch m.state {
	case reverseInputLoading:
		b.WriteString(subtitleStyle.Render("Loading cards..."))

	case reverseInputEmpty:
		b.WriteString(successStyle.Render("No cards due today!"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case reverseInputQuestion:
		card := m.cards[m.index]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.index+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(cardFrontStyle.Render(card.Back))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] submit  [esc] quit"))

	case reverseInputResult:
		card := m.cards[m.index]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.index+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(card.Back))
		b.WriteString("\n\n")
		if m.lastCorrect {
			b.WriteString(successStyle.Render("✓ " + card.Front))
		} else {
			b.WriteString(errorStyle.Render("✗ " + m.lastAnswer))
			b.WriteString(" → ")
			b.WriteString(successStyle.Render(card.Front))
		}

	case reverseInputSummary:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d / %d correct", m.correct, len(m.cards))))
		b.WriteString("\n\n")
		if m.sessionMode {
			b.WriteString(helpStyle.Render("[enter] continue"))
		} else {
			b.WriteString(helpStyle.Render("[enter] back to home"))
		}
	}

	return b.String()
}
