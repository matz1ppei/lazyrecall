package tui

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/ai"
	"github.com/ippei/poc-anki-claude/db"
	"github.com/ippei/poc-anki-claude/importer"
)

type homeState int

const (
	homeStateNormal homeState = iota
	homeStateImport
)

type msgStats struct {
	total int
	due   int
}

type msgImportDone struct {
	count int
	err   error
}

type HomeModel struct {
	db         *sql.DB
	ai         ai.Client
	state      homeState
	total      int
	due        int
	statsReady bool
	importInput textinput.Model
	importMsg   string
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
	return func() tea.Msg {
		cards, err := db.ListCards(h.db)
		if err != nil {
			return msgStats{}
		}
		due, err := db.ListDueCards(h.db)
		if err != nil {
			return msgStats{total: len(cards)}
		}
		return msgStats{total: len(cards), due: len(due)}
	}
}

func (h HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgStats:
		h.total = msg.total
		h.due = msg.due
		h.statsReady = true
		return h, nil

	case msgImportDone:
		if msg.err != nil {
			h.importMsg = errorStyle.Render(fmt.Sprintf("Import error: %v", msg.err))
		} else {
			h.importMsg = successStyle.Render(fmt.Sprintf("Imported %d cards.", msg.count))
		}
		h.state = homeStateNormal
		return h, h.loadStats()

	case tea.KeyMsg:
		if h.state == homeStateImport {
			return h.handleImportKey(msg)
		}
		return h.handleNormalKey(msg)
	}
	return h, nil
}

func (h HomeModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenReview} }
	case "a":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenAdd} }
	case "f":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenFetch} }
	case "i":
		h.state = homeStateImport
		h.importInput.SetValue("")
		h.importMsg = ""
		return h, h.importInput.Focus()
	case "q":
		return h, tea.Quit
	}
	return h, nil
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

	b.WriteString(titleStyle.Render("🃏  Anki Clone"))
	b.WriteString("\n\n")

	if h.statsReady {
		b.WriteString(labelStyle.Render(fmt.Sprintf("Total cards: %d   Due today: %d", h.total, h.due)))
	} else {
		b.WriteString(subtitleStyle.Render("Loading stats..."))
	}
	b.WriteString("\n\n")

	if h.state == homeStateImport {
		b.WriteString(inputLabelStyle.Render("CSV file path:"))
		b.WriteString("\n")
		b.WriteString(h.importInput.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[enter] import  [esc] cancel"))
	} else {
		menu := []string{
			keyStyle.Render("[r]") + menuItemStyle.Render(" Review"),
			keyStyle.Render("[a]") + menuItemStyle.Render(" Add card"),
			keyStyle.Render("[f]") + menuItemStyle.Render(" Fetch with AI"),
			keyStyle.Render("[i]") + menuItemStyle.Render(" Import CSV"),
			keyStyle.Render("[q]") + menuItemStyle.Render(" Quit"),
		}
		for _, item := range menu {
			b.WriteString("  " + item + "\n")
		}
	}

	if h.importMsg != "" {
		b.WriteString("\n" + h.importMsg)
	}

	return b.String()
}
