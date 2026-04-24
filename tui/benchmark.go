package tui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"golang.org/x/text/unicode/norm"
)

// levenshtein returns the edit distance between two strings (rune-based).
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1]
			} else {
				d := prev[j]
				if curr[j-1] < d {
					d = curr[j-1]
				}
				if prev[j-1] < d {
					d = prev[j-1]
				}
				curr[j] = 1 + d
			}
		}
		prev = curr
	}
	return prev[lb]
}

// benchmarkMaxCards is the maximum number of cards captured in a snapshot.
// Oldest cards (by ID) are preferred so the set is stable across runs.
const benchmarkMaxCards = 100

// normalizeAnswer strips leading/trailing spaces, lowercases, and removes
// combining diacritical marks so accented characters (é, ü, ñ …) match
// their unaccented equivalents when the user cannot type them.
func normalizeAnswer(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	// NFD decomposition separates base letters from combining marks
	s = norm.NFD.String(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue // drop combining diacritical mark
		}
		b.WriteRune(r)
	}
	return b.String()
}

type benchmarkState int

const (
	benchmarkStateLoading  benchmarkState = iota
	benchmarkStateReady                   // cards loaded, confirm before starting
	benchmarkStatePlaying                 // showing Back, waiting for typed Front
	benchmarkStateJudging                 // wrong answer: show diff, wait for y/n
	benchmarkStateComplete                // all cards done, score shown
	benchmarkStateHistory                 // past run list
	benchmarkStateEmpty                   // no cards available
)

type msgBenchmarkReady struct {
	cards []db.Card
	err   error
}

type msgBenchmarkSaved struct{ err error }

type msgBenchmarkHistory struct {
	runs []db.BenchmarkRun
	err  error
}

type BenchmarkModel struct {
	db        *sql.DB
	state     benchmarkState
	cards     []db.Card
	current   int
	correct   int
	input     textinput.Model
	refreshed bool // true when card set was just updated via [u]
	// judging state
	lastTyped   string
	lastCorrect string
	// history
	runs []db.BenchmarkRun
	err  string
}

func NewBenchmarkModel(database *sql.DB) BenchmarkModel {
	ti := textinput.New()
	ti.Placeholder = "Type the word..."
	ti.CharLimit = 256
	return BenchmarkModel{
		db:    database,
		state: benchmarkStateLoading,
		input: ti,
	}
}

func (m BenchmarkModel) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		ids, err := db.GetBenchmarkCardIDs(database)
		if err != nil {
			return msgBenchmarkReady{err: err}
		}

		var cards []db.Card
		if len(ids) == 0 {
			// First run: create snapshot from the oldest non-excluded cards (capped at benchmarkMaxCards)
			all, err := db.ListCards(database) // ordered by id ASC
			if err != nil {
				return msgBenchmarkReady{err: err}
			}
			excluded, _ := config.LoadExcludedWords()
			for _, c := range all {
				if !excluded[strings.ToLower(c.Front)] {
					cards = append(cards, c)
					if len(cards) == benchmarkMaxCards {
						break
					}
				}
			}
			if len(cards) == 0 {
				return msgBenchmarkReady{}
			}
			snapshotIDs := make([]int64, len(cards))
			for i, c := range cards {
				snapshotIDs[i] = c.ID
			}
			if err := db.SetBenchmarkCards(database, snapshotIDs); err != nil {
				return msgBenchmarkReady{err: err}
			}
		} else {
			cards, err = db.ListBenchmarkCards(database)
			if err != nil {
				return msgBenchmarkReady{err: err}
			}
		}
		return msgBenchmarkReady{cards: cards}
	}
}

func (m BenchmarkModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgBenchmarkReady:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		if len(msg.cards) == 0 {
			m.state = benchmarkStateEmpty
			return m, nil
		}
		wasRefresh := m.state == benchmarkStateLoading && len(m.cards) > 0
		m.cards = msg.cards
		m.current = 0
		m.correct = 0
		m.refreshed = wasRefresh
		m.state = benchmarkStateReady
		return m, nil

	case msgBenchmarkSaved:
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return m, nil

	case msgBenchmarkHistory:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.runs = msg.runs
		m.state = benchmarkStateHistory
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == benchmarkStatePlaying {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m BenchmarkModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case benchmarkStateEmpty:
		if msg.String() == "esc" || msg.String() == "enter" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case benchmarkStateReady:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			m.state = benchmarkStatePlaying
			m.input.Reset()
			return m, m.input.Focus()
		case "h":
			database := m.db
			return m, func() tea.Msg {
				runs, err := db.ListBenchmarkRuns(database)
				return msgBenchmarkHistory{runs: runs, err: err}
			}
		case "u":
			m.state = benchmarkStateLoading
			return m, m.refreshSnapshotCmd()
		}

	case benchmarkStatePlaying:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			typed := strings.TrimSpace(m.input.Value())
			if typed == "" {
				return m, nil
			}
			card := m.cards[m.current]
			expected := card.Front
			normTyped := normalizeAnswer(typed)
			normExpected := normalizeAnswer(expected)
			if normTyped == normExpected {
				// Correct
				m.correct++
				m.current++
				if m.current >= len(m.cards) {
					m.state = benchmarkStateComplete
					return m, m.saveAndComplete()
				}
				m.input.Reset()
				return m, m.input.Focus()
			}
			// Only offer typo judgment when edit distance == 1
			if levenshtein(normTyped, normExpected) == 1 {
				m.lastTyped = typed
				m.lastCorrect = expected
				m.state = benchmarkStateJudging
				return m, nil
			}
			// Distance > 1: count as wrong immediately
			m.current++
			if m.current >= len(m.cards) {
				m.state = benchmarkStateComplete
				return m, m.saveAndComplete()
			}
			m.input.Reset()
			return m, m.input.Focus()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case benchmarkStateJudging:
		switch msg.String() {
		case "y":
			// Counted as correct (typo)
			m.correct++
			m.current++
			if m.current >= len(m.cards) {
				m.state = benchmarkStateComplete
				return m, m.saveAndComplete()
			}
			m.state = benchmarkStatePlaying
			m.input.Reset()
			return m, m.input.Focus()
		case "n":
			// Wrong
			m.current++
			if m.current >= len(m.cards) {
				m.state = benchmarkStateComplete
				return m, m.saveAndComplete()
			}
			m.state = benchmarkStatePlaying
			m.input.Reset()
			return m, m.input.Focus()
		}

	case benchmarkStateComplete:
		switch msg.String() {
		case "esc", "enter":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "r":
			fresh := NewBenchmarkModel(m.db)
			return fresh, fresh.Init()
		case "h":
			database := m.db
			return m, func() tea.Msg {
				runs, err := db.ListBenchmarkRuns(database)
				return msgBenchmarkHistory{runs: runs, err: err}
			}
		case "u":
			m.state = benchmarkStateLoading
			return m, m.refreshSnapshotCmd()
		}

	case benchmarkStateHistory:
		if msg.String() == "esc" || msg.String() == "enter" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}
	}
	return m, nil
}

func (m BenchmarkModel) refreshSnapshotCmd() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		all, err := db.ListCards(database)
		if err != nil {
			return msgBenchmarkReady{err: err}
		}
		excluded, _ := config.LoadExcludedWords()
		var cards []db.Card
		for _, c := range all {
			if !excluded[strings.ToLower(c.Front)] {
				cards = append(cards, c)
				if len(cards) == benchmarkMaxCards {
					break
				}
			}
		}
		if len(cards) == 0 {
			return msgBenchmarkReady{}
		}
		ids := make([]int64, len(cards))
		for i, c := range cards {
			ids[i] = c.ID
		}
		if err := db.SetBenchmarkCards(database, ids); err != nil {
			return msgBenchmarkReady{err: err}
		}
		return msgBenchmarkReady{cards: cards}
	}
}

func (m BenchmarkModel) saveAndComplete() tea.Cmd {
	database := m.db
	total := len(m.cards)
	correct := m.correct
	return func() tea.Msg {
		err := db.InsertBenchmarkRun(database, time.Now(), total, correct)
		return msgBenchmarkSaved{err: err}
	}
}

func (m BenchmarkModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Benchmark"))
	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: " + m.err))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))
		return b.String()
	}

	switch m.state {
	case benchmarkStateLoading:
		b.WriteString(subtitleStyle.Render("Loading..."))

	case benchmarkStateReady:
		if m.refreshed {
			b.WriteString(successStyle.Render(fmt.Sprintf("Card set updated: %d cards", len(m.cards))))
		} else {
			b.WriteString(labelStyle.Render(fmt.Sprintf("%d cards loaded", len(m.cards))))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] start  [h] history  [u] update card set  [esc] back"))

	case benchmarkStateEmpty:
		b.WriteString(errorStyle.Render("No cards available for benchmark."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[esc] back"))

	case benchmarkStatePlaying:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("Meaning:"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(card.Back))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] submit  [esc] quit"))

	case benchmarkStateJudging:
		card := m.cards[m.current]
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d / %d", m.current+1, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("Meaning:"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(card.Back))
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("✗ Wrong"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(fmt.Sprintf("  Your answer : %s", m.lastTyped)))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("  Correct     : %s", m.lastCorrect)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Typo? [y] yes (mark correct)  [n] no (mark wrong)"))

	case benchmarkStateComplete:
		total := len(m.cards)
		pct := 0.0
		if total > 0 {
			pct = float64(m.correct) / float64(total) * 100
		}
		b.WriteString(successStyle.Render(fmt.Sprintf("%d / %d correct (%.0f%%)", m.correct, total, pct)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back  [r] retry  [h] history  [u] update card set"))

	case benchmarkStateHistory:
		b.WriteString(subtitleStyle.Render("Benchmark History"))
		b.WriteString("\n\n")
		if len(m.runs) == 0 {
			b.WriteString(labelStyle.Render("No results yet."))
		} else {
			for _, r := range m.runs {
				pct := 0.0
				if r.Total > 0 {
					pct = float64(r.Correct) / float64(r.Total) * 100
				}
				line := fmt.Sprintf("%-18s  %d / %d  (%.0f%%)",
					r.RunAt.Local().Format("2006-01-02 15:04"), r.Correct, r.Total, pct)
				b.WriteString(labelStyle.Render(line))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[esc] back"))
	}

	return b.String()
}
