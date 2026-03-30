package tui

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/db"
	"github.com/ippei/poc-anki-claude/srs"
)

type reviewState int

const (
	reviewStateLoading reviewState = iota
	reviewStateFront
	reviewStateBack
	reviewStateSummary
	reviewStateEmpty
)

type msgDueCards struct {
	cards []db.CardWithReview
}

type ReviewModel struct {
	db       *sql.DB
	state    reviewState
	cards    []db.CardWithReview
	index    int
	reviewed int
}

func NewReviewModel(database *sql.DB) ReviewModel {
	return ReviewModel{
		db:    database,
		state: reviewStateLoading,
	}
}

func (m ReviewModel) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListDueCards(database)
		if err != nil {
			return msgDueCards{cards: nil}
		}
		return msgDueCards{cards: cards}
	}
}

func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgDueCards:
		m.cards = msg.cards
		if len(m.cards) == 0 {
			m.state = reviewStateEmpty
		} else {
			m.state = reviewStateFront
		}
		return m, nil

	case msgCardRated:
		return m.handleRated(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m ReviewModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case reviewStateEmpty, reviewStateSummary:
		if msg.String() == "enter" || msg.String() == " " {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case reviewStateFront:
		if msg.String() == " " {
			m.state = reviewStateBack
		}

	case reviewStateBack:
		key := msg.String()
		if key >= "0" && key <= "5" {
			rating := int(key[0] - '0')
			return m, m.rateCard(rating)
		}
	}
	return m, nil
}

func (m ReviewModel) rateCard(rating int) tea.Cmd {
	card := m.cards[m.index]
	database := m.db
	index := m.index
	reviewed := m.reviewed

	return func() tea.Msg {
		current := srs.CardState{
			Interval:    card.Review.Interval,
			EaseFactor:  card.Review.EaseFactor,
			Repetitions: card.Review.Repetitions,
		}
		next := srs.NextState(current, rating)

		dueDate := time.Now().AddDate(0, 0, next.Interval).Format("2006-01-02")
		updated := db.Review{
			ID:          card.Review.ID,
			CardID:      card.Review.CardID,
			DueDate:     dueDate,
			Interval:    next.Interval,
			EaseFactor:  next.EaseFactor,
			Repetitions: next.Repetitions,
			LastRating:  &rating,
		}
		if err := db.UpdateReview(database, updated); err != nil {
			log.Printf("UpdateReview error: %v", err)
		}
		return msgCardRated{index: index, reviewed: reviewed + 1}
	}
}

type msgCardRated struct {
	index    int
	reviewed int
}

func (m ReviewModel) handleRated(msg msgCardRated) (ReviewModel, tea.Cmd) {
	m.reviewed = msg.reviewed
	m.index = msg.index + 1
	if m.index >= len(m.cards) {
		m.state = reviewStateSummary
	} else {
		m.state = reviewStateFront
	}
	return m, nil
}

func (m ReviewModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Review Session"))
	b.WriteString("\n\n")

	switch m.state {
	case reviewStateLoading:
		b.WriteString(subtitleStyle.Render("Loading cards..."))

	case reviewStateEmpty:
		b.WriteString(successStyle.Render("No cards due today!"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case reviewStateFront:
		card := m.cards[m.index]
		progress := fmt.Sprintf("%d / %d", m.index+1, len(m.cards))
		b.WriteString(labelStyle.Render(progress))
		b.WriteString("\n\n")
		b.WriteString(cardFrontStyle.Render(card.Front))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[space] reveal answer"))

	case reviewStateBack:
		card := m.cards[m.index]
		progress := fmt.Sprintf("%d / %d", m.index+1, len(m.cards))
		b.WriteString(labelStyle.Render(progress))
		b.WriteString("\n\n")
		b.WriteString(cardFrontStyle.Render(card.Front))
		b.WriteString(cardBackStyle.Render(card.Back))
		if card.Hint != "" {
			b.WriteString(hintStyle.Render("Hint: "+card.Hint))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(ratingStyle.Render("Rate your recall:"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[0] blackout  [1] wrong  [2] hard  [3] ok  [4] good  [5] perfect"))

	case reviewStateSummary:
		b.WriteString(successStyle.Render(fmt.Sprintf("Session complete! %d card(s) reviewed.", m.reviewed)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))
	}

	return b.String()
}
