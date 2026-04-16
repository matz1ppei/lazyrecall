package tui

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
)

type matchState int

const (
	matchStateLoading  matchState = iota
	matchStatePlaying             // game in progress
	matchStateWrong               // error flash active
	matchStateComplete            // all pairs matched
	matchStateEmpty               // fewer than 2 cards in DB
)

const (
	matchPairCount  = 4
	matchQueueSize  = 20
	matchWrongFlash = 1200 * time.Millisecond
)

type matchItem struct {
	cardID  int64
	text    string
	matched bool
}

type matchSelection struct {
	col   int // 0=left, 1=right
	index int
}

type wrongCard struct {
	cardID int64
	front  string
	back   string
}

type msgMatchCards []db.Card
type msgMatchWrongReset struct{}
type msgReviewAdded struct {
	count int
	err   error
}

// MatchModel is the model for the Match Madness mini-game.
type MatchModel struct {
	db                   *sql.DB
	state                matchState
	leftItems            []matchItem
	rightItems           []matchItem
	queue                []db.Card
	selected             *matchSelection
	activeCol            int // 0=left, 1=right
	leftCursor           int
	rightCursor          int
	mistakes             int
	totalMatched         int
	totalPairs           int
	startTime            time.Time
	elapsed              time.Duration
	wrongCards           []wrongCard
	wrongCardIDs         map[int64]bool
	reviewPromptAnswered bool
	reviewAdded          int
	reviewErr            string
	preloadedCards       []db.Card
	sessionMode          bool
	quitting             bool
	onComplete           tea.Cmd
}

func NewMatchModel(database *sql.DB) MatchModel {
	return MatchModel{
		db:           database,
		state:        matchStateLoading,
		wrongCardIDs: make(map[int64]bool),
	}
}

func NewMatchModelWithCards(database *sql.DB, cards []db.Card, onComplete tea.Cmd) MatchModel {
	return MatchModel{
		db:             database,
		state:          matchStateLoading,
		wrongCardIDs:   make(map[int64]bool),
		preloadedCards: cards,
		sessionMode:    true,
		onComplete:     onComplete,
	}
}

func (m MatchModel) Init() tea.Cmd {
	if m.preloadedCards != nil {
		cards := m.preloadedCards
		return func() tea.Msg { return msgMatchCards(cards) }
	}
	database := m.db
	return func() tea.Msg {
		cards, err := db.ListRandomCards(database, matchQueueSize)
		if err != nil {
			return msgMatchCards(nil)
		}
		excluded, _ := config.LoadExcludedWords()
		var filtered []db.Card
		for _, c := range cards {
			if !excluded[strings.ToLower(c.Front)] {
				filtered = append(filtered, c)
			}
		}
		return msgMatchCards(filtered)
	}
}

func (m MatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgMatchCards:
		if len(msg) < 2 {
			m.state = matchStateEmpty
			return m, nil
		}
		cards := []db.Card(msg)
		m.totalPairs = len(cards)

		visible := cards
		if len(visible) > matchPairCount {
			visible = cards[:matchPairCount]
			m.queue = cards[matchPairCount:]
		}
		m.leftItems = make([]matchItem, len(visible))
		m.rightItems = make([]matchItem, len(visible))
		for i, c := range visible {
			m.leftItems[i] = matchItem{cardID: c.ID, text: c.Front}
			m.rightItems[i] = matchItem{cardID: c.ID, text: c.Back}
		}
		rand.Shuffle(len(m.rightItems), func(i, j int) {
			m.rightItems[i], m.rightItems[j] = m.rightItems[j], m.rightItems[i]
		})
		m.state = matchStatePlaying
		m.startTime = time.Now()
		return m, nil

	case msgMatchWrongReset:
		m.selected = nil
		m.state = matchStatePlaying
		return m, nil

	case msgReviewAdded:
		if msg.err != nil {
			m.reviewErr = msg.err.Error()
		} else {
			m.reviewAdded = msg.count
		}
		return m, nil

	case tea.KeyMsg:
		if m.quitting {
			switch msg.String() {
			case "y":
				return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: "Match Madness: esc で中断"} }
			case "n", "esc":
				m.quitting = false
			}
			return m, nil
		}
		switch m.state {
		case matchStateComplete, matchStateEmpty:
			return m.handleEndKey(msg)
		case matchStatePlaying, matchStateWrong:
			return m.handlePlayKey(msg)
		}
	}
	return m, nil
}

func (m MatchModel) handleEndKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Wrong cards review prompt (standalone mode only)
	if !m.sessionMode && len(m.wrongCards) > 0 && !m.reviewPromptAnswered {
		switch msg.String() {
		case "y":
			m.reviewPromptAnswered = true
			cardIDs := make([]int64, len(m.wrongCards))
			for i, wc := range m.wrongCards {
				cardIDs[i] = wc.cardID
			}
			database := m.db
			count := len(cardIDs)
			return m, func() tea.Msg {
				err := db.SetDueToday(database, cardIDs)
				return msgReviewAdded{count: count, err: err}
			}
		case "n", "esc":
			m.reviewPromptAnswered = true
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "enter", "q":
		if m.onComplete != nil {
			return m, m.onComplete
		}
		reason := ""
		if m.state == matchStateEmpty {
			reason = "Match skipped: fewer than 2 cards available"
		}
		return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: reason} }
	case "r":
		if m.sessionMode {
			return m, nil
		}
		fresh := NewMatchModel(m.db)
		return fresh, fresh.Init()
	}
	return m, nil
}

func (m MatchModel) handlePlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		m.quitting = true
		return m, nil
	}
	if m.state == matchStateWrong {
		return m, nil
	}
	switch key {
	case "up":
		m = m.moveCursorDir(m.activeCol, -1)
	case "down":
		m = m.moveCursorDir(m.activeCol, 1)
	case "left":
		m.activeCol = 0
	case "right":
		m.activeCol = 1
	case "enter":
		cursor := m.leftCursor
		if m.activeCol == 1 {
			cursor = m.rightCursor
		}
		return m.handleKeySelect(m.activeCol, cursor)
	}
	return m, nil
}

func (m MatchModel) moveCursorDir(col, dir int) MatchModel {
	items := m.leftItems
	if col == 1 {
		items = m.rightItems
	}
	n := len(items)
	if n == 0 {
		return m
	}
	current := m.leftCursor
	if col == 1 {
		current = m.rightCursor
	}
	for i := 1; i <= n; i++ {
		next := ((current+dir*i)%n + n) % n
		if !items[next].matched {
			if col == 0 {
				m.leftCursor = next
			} else {
				m.rightCursor = next
			}
			return m
		}
	}
	return m
}

func (m MatchModel) handleKeySelect(col, idx int) (tea.Model, tea.Cmd) {
	if col == 0 && m.leftItems[idx].matched {
		return m, nil
	}
	if col == 1 && m.rightItems[idx].matched {
		return m, nil
	}

	if m.selected == nil {
		m.selected = &matchSelection{col: col, index: idx}
		return m, nil
	}

	prev := m.selected

	if prev.col == col && prev.index == idx {
		m.selected = nil
		return m, nil
	}
	if prev.col == col {
		m.selected = &matchSelection{col: col, index: idx}
		return m, nil
	}

	var leftIdx, rightIdx int
	if col == 0 {
		leftIdx = idx
		rightIdx = prev.index
	} else {
		leftIdx = prev.index
		rightIdx = idx
	}

	if m.leftItems[leftIdx].cardID == m.rightItems[rightIdx].cardID {
		return m.handleCorrectMatch(leftIdx, rightIdx)
	}

	// Wrong match — record the card the user failed on
	cid := m.leftItems[leftIdx].cardID
	if !m.wrongCardIDs[cid] {
		m.wrongCardIDs[cid] = true
		back := ""
		for _, ri := range m.rightItems {
			if ri.cardID == cid {
				back = ri.text
				break
			}
		}
		m.wrongCards = append(m.wrongCards, wrongCard{
			cardID: cid,
			front:  m.leftItems[leftIdx].text,
			back:   back,
		})
	}

	m.mistakes++
	m.selected = nil
	m.state = matchStateWrong
	return m, tea.Tick(matchWrongFlash, func(_ time.Time) tea.Msg {
		return msgMatchWrongReset{}
	})
}

func (m MatchModel) handleCorrectMatch(leftIdx, rightIdx int) (MatchModel, tea.Cmd) {
	m.totalMatched++
	m.selected = nil

	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.leftItems[leftIdx] = matchItem{cardID: next.ID, text: next.Front}
		m.rightItems[rightIdx] = matchItem{cardID: next.ID, text: next.Back}

		candidates := []int{}
		for i, item := range m.rightItems {
			if !item.matched && i != rightIdx {
				candidates = append(candidates, i)
			}
		}
		if len(candidates) > 0 {
			swapIdx := candidates[rand.Intn(len(candidates))]
			m.rightItems[rightIdx], m.rightItems[swapIdx] = m.rightItems[swapIdx], m.rightItems[rightIdx]
		}
	} else {
		m.leftItems[leftIdx].matched = true
		m.rightItems[rightIdx].matched = true

		allDone := true
		for _, item := range m.leftItems {
			if !item.matched {
				allDone = false
				break
			}
		}
		if allDone {
			m.elapsed = time.Since(m.startTime)
			m.state = matchStateComplete
		}
	}
	return m, nil
}

func (m MatchModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Match Madness"))
	b.WriteString("\n\n")

	if m.quitting {
		b.WriteString(labelStyle.Render("Match Madness を中断しますか？"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%s  %s", keyStyle.Render("[y] 中断"), keyStyle.Render("[n] 続ける")))
		return b.String()
	}

	switch m.state {
	case matchStateLoading:
		b.WriteString(subtitleStyle.Render("Loading..."))
		return b.String()

	case matchStateEmpty:
		b.WriteString(errorStyle.Render("Not enough cards (need at least 2)"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))
		return b.String()

	case matchStateComplete:
		return m.viewComplete()
	}

	// Playing or wrong flash
	n := len(m.leftItems)
	leftWidth := 20
	rightWidth := 42
	for i := 0; i < n; i++ {
		b.WriteString("  " + m.renderItem(0, i, leftWidth) + "    " + m.renderItem(1, i, rightWidth) + "\n")
	}

	b.WriteString("\n")
	if m.state == matchStateWrong {
		b.WriteString(errorStyle.Render("✗ Wrong!"))
		b.WriteString("  ")
	}
	elapsed := time.Since(m.startTime)
	b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d matched", m.totalMatched, m.totalPairs)))
	b.WriteString("   ")
	b.WriteString(helpStyle.Render(fmt.Sprintf("Time: %02d:%02d  Mistakes: %d",
		int(elapsed.Minutes()), int(elapsed.Seconds())%60, m.mistakes)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[↑/↓] move  [←/→] switch column  [enter] select  [esc] quit"))
	return b.String()
}

func (m MatchModel) viewComplete() string {
	var b strings.Builder

	secs := m.elapsed.Seconds()
	b.WriteString(successStyle.Render(fmt.Sprintf("Complete!  %d pairs  Time: %.1fs  Mistakes: %d",
		m.totalMatched, secs, m.mistakes)))
	b.WriteString("\n\n")

	if len(m.wrongCards) == 0 {
		b.WriteString(successStyle.Render("Perfect! No mistakes."))
		b.WriteString("\n\n")
		if m.sessionMode {
			b.WriteString(helpStyle.Render("[enter] continue"))
		} else {
			b.WriteString(helpStyle.Render("[r] play again  [enter/esc] back to home"))
		}
		return b.String()
	}

	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Missed pairs (%d):", len(m.wrongCards))))
	b.WriteString("\n")
	for _, wc := range m.wrongCards {
		b.WriteString(labelStyle.Render(fmt.Sprintf("  %s  →  %s", wc.front, wc.back)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if !m.sessionMode && !m.reviewPromptAnswered {
		b.WriteString(labelStyle.Render("Add missed cards to today's review?"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%s  %s",
			keyStyle.Render("[y] Yes"),
			keyStyle.Render("[n] Skip"),
		))
	} else {
		if !m.sessionMode {
			if m.reviewErr != "" {
				b.WriteString(errorStyle.Render("Error: " + m.reviewErr))
			} else if m.reviewAdded > 0 {
				b.WriteString(successStyle.Render(fmt.Sprintf("✓ Added %d card(s) to today's review.", m.reviewAdded)))
			} else {
				b.WriteString(labelStyle.Render("Skipped."))
			}
			b.WriteString("\n\n")
			b.WriteString(helpStyle.Render("[r] play again  [enter/esc] back to home"))
		} else {
			b.WriteString(helpStyle.Render("[enter] continue"))
		}
	}

	return b.String()
}

func (m MatchModel) renderItem(col, idx int, width int) string {
	var item matchItem
	if col == 0 {
		item = m.leftItems[idx]
	} else {
		item = m.rightItems[idx]
	}

	isCursor := (col == 0 && m.activeCol == 0 && m.leftCursor == idx) ||
		(col == 1 && m.activeCol == 1 && m.rightCursor == idx)
	isSelected := m.selected != nil && m.selected.col == col && m.selected.index == idx

	prefix := "  "
	if isCursor {
		prefix = "> "
	}
	text := truncateText(item.text, width-4)
	cell := fmt.Sprintf("%s%-*s", prefix, width-2, text)

	switch {
	case item.matched:
		return matchMatchedStyle.Render(cell)
	case m.state == matchStateWrong && isSelected:
		return matchWrongStyle.Render(cell)
	case isSelected:
		return matchSelectedStyle.Render(cell)
	case isCursor:
		return matchCursorStyle.Render(cell)
	default:
		return labelStyle.Render(cell)
	}
}

func truncateText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
