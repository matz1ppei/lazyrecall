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
	"github.com/ippei/lazyrecall/importer"
)

type homeState int

const (
	homeStateNormal homeState = iota
	homeStatePractice
	homeStateTools
	homeStateImport
	homeStateDedup
	homeStateGenerating
)

const dailyReviewLimit = 100

type msgStats struct {
	total         int
	due           int
	overdue       int
	reviewedToday int
	session       db.DailySession
}

type msgImportDone struct {
	count int
	err   error
}

type msgDedupDone struct {
	deleted int
	err     error
}

type msgBatchDone struct {
	generated int
	errors    int
}

type HomeModel struct {
	db            *sql.DB
	ai            ai.Client
	state         homeState
	total         int
	due           int
	overdue       int
	reviewedToday int
	statsReady    bool
	session       db.DailySession
	importInput   textinput.Model
	importMsg     string
}

func NewHomeModel(database *sql.DB, aiClient ai.Client) HomeModel {
	ti := textinput.New()
	ti.Placeholder = "path/to/cards.csv"
	ti.CharLimit = 256
	return HomeModel{
		db: database,
		ai: aiClient,
		importInput: ti,
	}
}

func (h HomeModel) Init() tea.Cmd {
	return h.loadStats()
}

func (h HomeModel) loadStats() tea.Cmd {
	database := h.db
	return func() tea.Msg {
		cards, err := db.ListCards(database)
		if err != nil {
			return msgStats{}
		}
		due, err := db.CountDueCards(database)
		if err != nil {
			return msgStats{total: len(cards)}
		}
		overdue, err := db.CountOverdueCards(database)
		if err != nil {
			return msgStats{total: len(cards), due: due}
		}
		reviewedToday, err := db.CountReviewedToday(database)
		if err != nil {
			return msgStats{total: len(cards), due: due, overdue: overdue}
		}
		session, _ := db.GetTodaySession(database)
		return msgStats{total: len(cards), due: due, overdue: overdue, reviewedToday: reviewedToday, session: session}
	}
}

func (h HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgStats:
		h.total = msg.total
		h.due = msg.due
		h.overdue = msg.overdue
		h.reviewedToday = msg.reviewedToday
		h.session = msg.session
		h.statsReady = true
		// Redirect first-time users to the onboarding setup flow.
		if h.total == 0 {
			return h, func() tea.Msg { return MsgGotoScreen{Target: screenSetup} }
		}
		return h, nil

	case msgImportDone:
		if msg.err != nil {
			h.importMsg = errorStyle.Render(fmt.Sprintf("Import error: %v", msg.err))
		} else {
			h.importMsg = successStyle.Render(fmt.Sprintf("Imported %d cards.", msg.count))
		}
		h.state = homeStateNormal
		return h, h.loadStats()

	case msgDedupDone:
		if msg.err != nil {
			h.importMsg = errorStyle.Render(fmt.Sprintf("Dedup error: %v", msg.err))
		} else if msg.deleted == 0 {
			h.importMsg = successStyle.Render("No duplicates found.")
		} else {
			h.importMsg = successStyle.Render(fmt.Sprintf("Deleted %d duplicate(s).", msg.deleted))
		}
		h.state = homeStateNormal
		return h, h.loadStats()

	case msgBatchDone:
		h.state = homeStateNormal
		if msg.errors > 0 {
			h.importMsg = successStyle.Render(fmt.Sprintf("Generated %d translations. %d errors.", msg.generated, msg.errors))
		} else {
			h.importMsg = successStyle.Render(fmt.Sprintf("Generated %d translations.", msg.generated))
		}
		return h, h.loadStats()

	case tea.KeyMsg:
		if h.state == homeStateImport {
			return h.handleImportKey(msg)
		}
		if h.state == homeStateDedup {
			return h.handleDedupKey(msg)
		}
		if h.state == homeStateGenerating {
			return h, nil
		}
		if h.state == homeStateTools {
			return h.handleToolsKey(msg)
		}
		if h.state == homeStatePractice {
			return h.handlePracticeKey(msg)
		}
		return h.handleNormalKey(msg)
	}
	return h, nil
}

func (h HomeModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "d":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenSession} }
	case "p":
		h.state = homeStatePractice
		return h, nil
	case "a":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenAdd} }
	case "l":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenList} }
	case "s":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenStats} }
	case "t":
		h.state = homeStateTools
		h.importMsg = ""
		return h, nil
	case "q":
		return h, tea.Quit
	}
	return h, nil
}

func (h HomeModel) handlePracticeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		h.state = homeStateNormal
		return h, nil
	case "r":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenReview} }
	case "v":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenReverseReview} }
	case "m":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenMatch} }
	case "b":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenBlank} }
	}
	return h, nil
}

func (h HomeModel) handleToolsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		h.state = homeStateNormal
		return h, nil
	case "f":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenFetchLang} }
	case "t":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenFetch} }
	case "i":
		h.state = homeStateImport
		h.importInput.SetValue("")
		h.importMsg = ""
		return h, h.importInput.Focus()
	case "g":
		if h.ai == nil {
			h.importMsg = errorStyle.Render("AI not configured.")
			h.state = homeStateNormal
			return h, nil
		}
		h.state = homeStateGenerating
		h.importMsg = ""
		return h, h.batchGenerateTranslations()
	case "x":
		h.state = homeStateDedup
		h.importMsg = ""
		return h, nil
	}
	return h, nil
}

func (h HomeModel) handleDedupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		dbRef := h.db
		h.state = homeStateNormal
		return h, func() tea.Msg {
			deleted, err := db.DeduplicateCards(dbRef)
			return msgDedupDone{deleted: deleted, err: err}
		}
	case "n", "esc":
		h.state = homeStateNormal
	}
	return h, nil
}

func (h HomeModel) batchGenerateTranslations() tea.Cmd {
	database := h.db
	aiClient := h.ai
	return func() tea.Msg {
		cards, err := db.ListCardsNeedingTranslation(database)
		if err != nil || len(cards) == 0 {
			return msgBatchDone{}
		}
		generated := 0
		errors := 0
		for _, card := range cards {
			translation, err := aiClient.GenerateExampleTranslation(
				context.Background(), card.Front, card.Back, card.Example,
			)
			if err != nil {
				errors++
				continue
			}
			if err := db.UpdateCardTranslation(database, card.ID, translation); err != nil {
				errors++
				continue
			}
			generated++
		}
		return msgBatchDone{generated: generated, errors: errors}
	}
}

func (h HomeModel) handleImportKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path := strings.TrimSpace(h.importInput.Value())
		if path == "" {
			h.state = homeStateNormal
			return h, nil
		}
		dbRef := h.db
		return h, func() tea.Msg {
			count, err := importer.ImportCSV(dbRef, path)
			return msgImportDone{count: count, err: err}
		}
	case "esc":
		h.state = homeStateNormal
		h.importInput.Blur()
		return h, nil
	}
	var cmd tea.Cmd
	h.importInput, cmd = h.importInput.Update(msg)
	return h, cmd
}

func (h HomeModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("🃏  LazyRecall"))
	b.WriteString("\n\n")

	if h.statsReady {
		remaining := dailyReviewLimit - h.reviewedToday
		if remaining < 0 {
			remaining = 0
		}
		if remaining > h.due {
			remaining = h.due
		}
		dueToday := h.due - h.overdue
		b.WriteString(labelStyle.Render(fmt.Sprintf("Total: %d cards", h.total)))
		b.WriteString("\n")
		if h.overdue > 0 {
			b.WriteString(labelStyle.Render(fmt.Sprintf("Due today: %d   Overdue: %d", dueToday, h.overdue)))
		} else {
			b.WriteString(labelStyle.Render(fmt.Sprintf("Due today: %d", dueToday)))
		}
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Today: %d / %d reviewed   Remaining: %d", h.reviewedToday, dailyReviewLimit, remaining)))
		b.WriteString("\n")
		check := func(done bool) string {
			if done {
				return "✓"
			}
			return " "
		}
		b.WriteString(labelStyle.Render(fmt.Sprintf("Session: Review [%s] Match [%s] Reverse [%s] Blank [%s]",
			check(h.session.ReviewDone), check(h.session.MatchDone), check(h.session.ReverseDone), check(h.session.BlankDone))))
	} else {
		b.WriteString(subtitleStyle.Render("Loading stats..."))
	}
	b.WriteString("\n\n")

	switch h.state {
	case homeStateImport:
		b.WriteString(inputLabelStyle.Render("CSV file path:"))
		b.WriteString("\n")
		b.WriteString(h.importInput.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] import  [esc] cancel"))
	case homeStateDedup:
		b.WriteString(errorStyle.Render("Remove duplicate cards? (keeps oldest per front)"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[y/enter] yes  [n/esc] cancel"))
	case homeStatePractice:
		b.WriteString(subtitleStyle.Render("Practice"))
		b.WriteString("\n\n")
		menu := []string{
			keyStyle.Render("[r]") + menuItemStyle.Render(" Review"),
			keyStyle.Render("[v]") + menuItemStyle.Render(" Reverse Review"),
			keyStyle.Render("[m]") + menuItemStyle.Render(" Match Madness"),
			keyStyle.Render("[b]") + menuItemStyle.Render(" Blank fill"),
		}
		for _, item := range menu {
			b.WriteString("  " + item + "\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[esc] back"))
		b.WriteString("\n")
	case homeStateTools:
		b.WriteString(subtitleStyle.Render("Tools"))
		b.WriteString("\n\n")
		menu := []string{
			keyStyle.Render("[f]") + menuItemStyle.Render(" Fetch: Language (dict)"),
			keyStyle.Render("[t]") + menuItemStyle.Render(" Fetch: Topic (AI)"),
			keyStyle.Render("[i]") + menuItemStyle.Render(" Import CSV"),
			keyStyle.Render("[g]") + menuItemStyle.Render(" Generate translations"),
			keyStyle.Render("[x]") + menuItemStyle.Render(" Deduplicate"),
		}
		for _, item := range menu {
			b.WriteString("  " + item + "\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[esc] back"))
	case homeStateGenerating:
		b.WriteString(subtitleStyle.Render("Generating translations... Please wait."))
	default:
		menu := []string{
			keyStyle.Render("[d]") + menuItemStyle.Render(" Daily Session"),
			keyStyle.Render("[p]") + menuItemStyle.Render(" Practice"),
			keyStyle.Render("[a]") + menuItemStyle.Render(" Add card"),
			keyStyle.Render("[l]") + menuItemStyle.Render(" List cards"),
			keyStyle.Render("[s]") + menuItemStyle.Render(" Stats"),
			keyStyle.Render("[t]") + menuItemStyle.Render(" Tools"),
			keyStyle.Render("[q]") + menuItemStyle.Render(" Quit"),
		}
		for _, item := range menu {
			b.WriteString("  " + item + "\n")
		}
		b.WriteString("\n\n")
	}

	if h.importMsg != "" {
		b.WriteString("\n" + h.importMsg)
	}

	return b.String()
}
