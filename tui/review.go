package tui

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/srs"
)

type reviewState int

const (
	reviewStateLoading reviewState = iota
	reviewStateQuestion
	reviewStateResult
	reviewStateSummary
	reviewStateEmpty
	reviewStateLimitReached
)

const reviewSessionSize = 20

type msgDueCards struct {
	cards         []db.CardWithReview
	reviewedToday int
	limitReached  bool
}

type msgReviewChoicesLoaded struct {
	choices      []string
	correctIndex int
}

type msgReviewResultReset struct{}

type ReviewModel struct {
	db             *sql.DB
	state          reviewState
	cards          []db.CardWithReview
	index          int
	reviewed       int
	reviewedToday  int
	preloadedCards []db.CardWithReview
	ignoreLimit    bool
	onComplete     tea.Cmd
	sessionMode    bool
	choices        []string
	correctIndex   int
	cursorIndex    int
	lastCorrect    bool
	correctIDs     []int64
	quitting       bool
}

func NewReviewModel(database *sql.DB) ReviewModel {
	return ReviewModel{
		db:    database,
		state: reviewStateLoading,
	}
}

func NewReviewModelWithCards(database *sql.DB, cards []db.CardWithReview, onComplete tea.Cmd) ReviewModel {
	return ReviewModel{
		db:             database,
		state:          reviewStateLoading,
		preloadedCards: cards,
		ignoreLimit:    true,
		onComplete:     onComplete,
		sessionMode:    true,
	}
}

func (m ReviewModel) Init() tea.Cmd {
	if len(m.preloadedCards) > 0 {
		cards := m.preloadedCards
		return func() tea.Msg { return msgDueCards{cards: cards} }
	}
	database := m.db
	return func() tea.Msg {
		reviewedToday, err := db.CountReviewedToday(database)
		if err != nil {
			return msgDueCards{}
		}
		remaining := dailyReviewLimit - reviewedToday
		if remaining <= 0 {
			return msgDueCards{reviewedToday: reviewedToday, limitReached: true}
		}
		limit := remaining
		if limit > reviewSessionSize {
			limit = reviewSessionSize
		}
		cards, err := db.ListDueCards(database, limit)
		if err != nil {
			return msgDueCards{}
		}
		excluded, _ := config.LoadExcludedWords()
		var filtered []db.CardWithReview
		for _, c := range cards {
			if !excluded[strings.ToLower(c.Front)] {
				filtered = append(filtered, c)
			}
		}
		return msgDueCards{cards: filtered, reviewedToday: reviewedToday}
	}
}

func (m ReviewModel) loadNextBatchCmd() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListDueCards(database, reviewSessionSize)
		if err != nil {
			return msgDueCards{}
		}
		excluded, _ := config.LoadExcludedWords()
		var filtered []db.CardWithReview
		for _, c := range cards {
			if !excluded[strings.ToLower(c.Front)] {
				filtered = append(filtered, c)
			}
		}
		return msgDueCards{cards: filtered}
	}
}

func (m ReviewModel) loadChoicesCmd() tea.Cmd {
	card := m.cards[m.index]
	database := m.db
	return func() tea.Msg {
		distractors, _ := db.ListRandomCardsExcluding(database, 3, []int64{card.Card.ID})
		correct := card.Back
		var distractorValues []string
		for _, d := range distractors {
			distractorValues = append(distractorValues, d.Back)
		}
		choices := append([]string{correct}, distractorValues...)
		rand.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
		correctIndex := 0
		for i, c := range choices {
			if c == correct {
				correctIndex = i
				break
			}
		}
		return msgReviewChoicesLoaded{choices: choices, correctIndex: correctIndex}
	}
}

func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgDueCards:
		m.cards = msg.cards
		m.reviewed = 0
		m.index = 0
		if msg.reviewedToday > 0 {
			m.reviewedToday = msg.reviewedToday
		}
		if msg.limitReached {
			m.state = reviewStateLimitReached
			return m, nil
		}
		if len(m.cards) == 0 {
			m.state = reviewStateEmpty
			return m, nil
		}
		return m, m.loadChoicesCmd()

	case msgReviewChoicesLoaded:
		m.choices = msg.choices
		m.correctIndex = msg.correctIndex
		m.cursorIndex = 0
		m.state = reviewStateQuestion
		return m, nil

	case msgReviewResultReset:
		m.reviewed++
		m.reviewedToday++
		m.index++
		if m.index >= len(m.cards) {
			if !m.ignoreLimit && m.reviewedToday >= dailyReviewLimit {
				m.state = reviewStateLimitReached
			} else {
				m.state = reviewStateSummary
			}
			return m, nil
		}
		return m, m.loadChoicesCmd()

	case tea.KeyMsg:
		if m.quitting {
			switch msg.String() {
			case "y":
				return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: "Review: esc/q で中断"} }
			case "n", "esc":
				m.quitting = false
			}
			return m, nil
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m ReviewModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case reviewStateEmpty, reviewStateSummary:
		if msg.String() == "enter" || msg.String() == " " || msg.String() == "esc" {
			if m.onComplete != nil {
				return m, m.onComplete
			}
			reason := ""
			if m.state == reviewStateEmpty {
				reason = "Review skipped: no due cards"
			}
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: reason} }
		}

	case reviewStateLimitReached:
		switch msg.String() {
		case "y":
			m.state = reviewStateLoading
			return m, m.loadNextBatchCmd()
		case "n", "enter", "esc":
			return m, func() tea.Msg {
				return MsgGotoScreen{Target: screenHome, Reason: "Review: 上限到達のため終了"}
			}
		}

	case reviewStateQuestion:
		switch msg.String() {
		case "j", "down":
			m.cursorIndex = (m.cursorIndex + 1) % len(m.choices)
		case "k", "up":
			m.cursorIndex = (m.cursorIndex - 1 + len(m.choices)) % len(m.choices)
		case "enter":
			card := m.cards[m.index]
			m.lastCorrect = m.cursorIndex == m.correctIndex
			if m.lastCorrect {
				m.correctIDs = append(m.correctIDs, card.Card.ID)
			}
			m.state = reviewStateResult
			delay := 600 * time.Millisecond
			if !m.lastCorrect {
				delay = 1500 * time.Millisecond
			}
			tick := tea.Tick(delay, func(time.Time) tea.Msg { return msgReviewResultReset{} })
			if m.sessionMode {
				return m, tick
			}
			rating := 0
			if m.lastCorrect {
				rating = 4
			}
			return m, tea.Batch(m.rateCard(rating), tick)
		case "esc", "q":
			m.quitting = true
			return m, nil
		}
	}
	return m, nil
}

func (m ReviewModel) rateCard(rating int) tea.Cmd {
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

func (m ReviewModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Review Session"))
	b.WriteString("\n\n")

	if m.quitting {
		b.WriteString(labelStyle.Render("Review を中断しますか？"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%s  %s", keyStyle.Render("[y] 中断"), keyStyle.Render("[n] 続ける")))
		return b.String()
	}

	switch m.state {
	case reviewStateLoading:
		b.WriteString(subtitleStyle.Render("Loading cards..."))

	case reviewStateEmpty:
		b.WriteString(successStyle.Render("No cards due today!"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case reviewStateLimitReached:
		b.WriteString(successStyle.Render(fmt.Sprintf("Daily limit reached! (%d / %d reviewed today)", m.reviewedToday, dailyReviewLimit)))
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render("Continue anyway?"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%s  %s",
			keyStyle.Render("[y] Continue"),
			keyStyle.Render("[n/enter] Back to home"),
		))

	case reviewStateQuestion:
		card := m.cards[m.index]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.index+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(cardFrontStyle.Render(card.Front))
		b.WriteString("\n\n")
		for i, choice := range m.choices {
			if i == m.cursorIndex {
				b.WriteString(matchCursorStyle.Render("> " + choice))
			} else {
				b.WriteString("  " + labelStyle.Render(choice))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[j/k] move  [enter] select  [esc] quit"))

	case reviewStateResult:
		card := m.cards[m.index]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.index+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(card.Front))
		b.WriteString("\n\n")
		if m.lastCorrect {
			b.WriteString(successStyle.Render("✓ " + m.choices[m.correctIndex]))
		} else {
			b.WriteString(errorStyle.Render("✗ " + m.choices[m.cursorIndex]))
			b.WriteString(" → ")
			b.WriteString(successStyle.Render(m.choices[m.correctIndex]))
		}

	case reviewStateSummary:
		b.WriteString(successStyle.Render(fmt.Sprintf("Session complete! %d card(s) reviewed.", m.reviewed)))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Today total: %d / %d", m.reviewedToday, dailyReviewLimit)))
		b.WriteString("\n\n")
		if m.sessionMode {
			b.WriteString(helpStyle.Render("[enter] continue"))
		} else {
			b.WriteString(helpStyle.Render("[enter] back to home"))
		}
	}

	return b.String()
}
