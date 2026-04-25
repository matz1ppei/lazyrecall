package tui

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

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

type listSortMode int

const (
	listSortNewest listSortMode = iota
	listSortDue
	listSortFront
	listSortExcluded
)

type ListModel struct {
	db                *sql.DB
	ai                ai.Client
	state             listState
	cards             []db.CardWithReview
	excluded          map[string]bool
	cursor            int
	offset            int
	errMsg            string
	editInputs        [6]textinput.Model // front, back, hint, example, example translation, example word
	editFocus         int
	editLoading       bool
	editOriginalFront string
	editExcluded      bool
	filterInput       textinput.Model
	filterActive      bool
	filterExcluded    bool
	filterDueOnly     bool
	sortMode          listSortMode
}

const listPageSize = 15

func NewListModel(database *sql.DB, aiClient ai.Client) ListModel {
	filterInput := textinput.New()
	filterInput.Placeholder = "Search front/back"
	filterInput.CharLimit = 128
	filterInput.Width = 24
	return ListModel{
		db:          database,
		ai:          aiClient,
		state:       listStateLoading,
		filterInput: filterInput,
		sortMode:    listSortNewest,
	}
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
		m.clampListPosition()
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
		return m, tea.Batch(m.reloadCmd(), loadExcludedCmd())

	case msgExcludedWords:
		m.excluded = msg.excluded
		m.clampListPosition()
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
				m.editInputs[4].SetValue(msg.translation)
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
	labels := []string{"Front", "Back", "Hint", "Example", "Example Translation", "Example Word"}
	values := []string{card.Front, card.Back, card.Hint, card.Example, card.Card.ExampleTranslation, card.Card.ExampleWord}
	for i := range m.editInputs {
		ti := textinput.New()
		ti.Placeholder = labels[i]
		ti.CharLimit = 512
		ti.SetValue(values[i])
		m.editInputs[i] = ti
	}
	m.editFocus = 0
	m.editInputs[0].Focus()
	m.editOriginalFront = card.Front
	m.editExcluded = m.excluded[strings.ToLower(card.Front)]
}

func (m ListModel) filteredCardIndices() []int {
	indices := make([]int, 0, len(m.cards))
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	now := time.Now()
	for i, card := range m.cards {
		if query != "" {
			front := strings.ToLower(card.Front)
			back := strings.ToLower(card.Back)
			if !strings.Contains(front, query) && !strings.Contains(back, query) {
				continue
			}
		}
		if m.filterExcluded && !m.excluded[strings.ToLower(card.Front)] {
			continue
		}
		if m.filterDueOnly && !isDueNow(card.Review.DueDate, now) {
			continue
		}
		indices = append(indices, i)
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left := m.cards[indices[i]]
		right := m.cards[indices[j]]
		switch m.sortMode {
		case listSortFront:
			if strings.ToLower(left.Front) != strings.ToLower(right.Front) {
				return strings.ToLower(left.Front) < strings.ToLower(right.Front)
			}
		case listSortNewest:
			if !left.Card.CreatedAt.Equal(right.Card.CreatedAt) {
				return left.Card.CreatedAt.After(right.Card.CreatedAt)
			}
		case listSortExcluded:
			leftExcluded := m.excluded[strings.ToLower(left.Front)]
			rightExcluded := m.excluded[strings.ToLower(right.Front)]
			if leftExcluded != rightExcluded {
				return leftExcluded
			}
			if strings.ToLower(left.Front) != strings.ToLower(right.Front) {
				return strings.ToLower(left.Front) < strings.ToLower(right.Front)
			}
		default:
			if left.Review.DueDate != right.Review.DueDate {
				return left.Review.DueDate < right.Review.DueDate
			}
		}
		return left.Card.ID < right.Card.ID
	})
	return indices
}

func (m *ListModel) cycleSortMode() {
	m.sortMode = (m.sortMode + 1) % 4
	m.cursor = 0
	m.offset = 0
	m.clampListPosition()
}

func (m ListModel) sortModeLabel() string {
	switch m.sortMode {
	case listSortFront:
		return "front"
	case listSortNewest:
		return "new"
	case listSortExcluded:
		return "excluded"
	default:
		return "due"
	}
}

func (m *ListModel) clampListPosition() {
	total := len(m.filteredCardIndices())
	if total == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor >= total {
		m.cursor = total - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.offset > m.cursor {
		m.offset = m.cursor
	}
	maxOffset := total - listPageSize
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.cursor >= m.offset+listPageSize {
		m.offset = m.cursor - listPageSize + 1
	}
}

func (m ListModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case listStateEmpty:
		if msg.String() == "esc" || msg.String() == "q" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case listStateNormal:
		filtered := m.filteredCardIndices()
		total := len(filtered)
		if m.filterActive {
			switch msg.String() {
			case "esc":
				m.filterActive = false
				m.filterInput.Blur()
				return m, nil
			case "enter":
				m.filterActive = false
				m.filterInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.clampListPosition()
				return m, cmd
			}
		}
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "/", "ctrl+f":
			m.filterActive = true
			m.errMsg = ""
			return m, m.filterInput.Focus()
		case "o":
			m.filterExcluded = !m.filterExcluded
			m.clampListPosition()
		case "u":
			m.filterDueOnly = !m.filterDueOnly
			m.clampListPosition()
		case "c":
			m.filterInput.SetValue("")
			m.filterExcluded = false
			m.filterDueOnly = false
			m.filterActive = false
			m.filterInput.Blur()
			m.clampListPosition()
		case "s":
			m.cycleSortMode()
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
			if total == 0 {
				return m, nil
			}
			m.initEditInputs(m.cards[filtered[m.cursor]])
			m.state = listStateEdit
			m.errMsg = ""
			return m, textinput.Blink
		case "d":
			if total == 0 {
				return m, nil
			}
			m.state = listStateConfirmDelete
			m.errMsg = ""
		case "x":
			if total == 0 {
				return m, nil
			}
			m.state = listStateConfirmExclude
			m.errMsg = ""
		}

	case listStateConfirmExclude:
		switch msg.String() {
		case "y", "enter":
			filtered := m.filteredCardIndices()
			if len(filtered) == 0 {
				m.state = listStateNormal
				return m, nil
			}
			word := m.cards[filtered[m.cursor]].Front
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
			filtered := m.filteredCardIndices()
			if len(filtered) == 0 {
				m.state = listStateNormal
				return m, nil
			}
			card := m.cards[filtered[m.cursor]]
			database := m.db
			if m.cursor > 0 && m.cursor >= len(filtered)-1 {
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
			m.editFocus = (m.editFocus + 1) % len(m.editInputs)
			return m, m.editInputs[m.editFocus].Focus()
		case "shift+tab", "up":
			m.editInputs[m.editFocus].Blur()
			m.editFocus = (m.editFocus + len(m.editInputs) - 1) % len(m.editInputs)
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
						example, translation, _, err := aiClient.GenerateExample(context.Background(), front, back)
						return msgEditGenerated{text: example, translation: translation, err: err}
					}
					text, err := aiClient.GenerateHint(context.Background(), front, back)
					return msgEditGenerated{text: text, err: err}
				}
			}
		case "ctrl+s", "enter":
			if m.editFocus < len(m.editInputs)-1 {
				// Tab to next field on enter (except last)
				m.editInputs[m.editFocus].Blur()
				m.editFocus++
				return m, m.editInputs[m.editFocus].Focus()
			}
			// Save on enter at last field
			filtered := m.filteredCardIndices()
			if len(filtered) == 0 {
				m.state = listStateNormal
				return m, nil
			}
			card := m.cards[filtered[m.cursor]]
			front := strings.TrimSpace(m.editInputs[0].Value())
			back := strings.TrimSpace(m.editInputs[1].Value())
			hint := strings.TrimSpace(m.editInputs[2].Value())
			example := strings.TrimSpace(m.editInputs[3].Value())
			exampleTranslation := strings.TrimSpace(m.editInputs[4].Value())
			exampleWord := strings.TrimSpace(m.editInputs[5].Value())
			if front == "" || back == "" {
				m.errMsg = "Front and Back cannot be empty"
				return m, nil
			}
			database := m.db
			id := card.Card.ID
			originalFront := m.editOriginalFront
			editExcluded := m.editExcluded
			m.state = listStateLoading
			return m, func() tea.Msg {
				err := db.UpdateCard(database, id, front, back, hint, example, exampleTranslation, exampleWord)
				if err != nil {
					return msgUpdateDone{err: err}
				}
				if err := config.SetExcludedWord(originalFront, false); err != nil {
					return msgUpdateDone{err: err}
				}
				if editExcluded {
					if err := config.SetExcludedWord(front, true); err != nil {
						return msgUpdateDone{err: err}
					}
				}
				return msgUpdateDone{err: err}
			}
		case "ctrl+x":
			m.editExcluded = !m.editExcluded
			return m, nil
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

func parseDueTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func isDueNow(s string, now time.Time) bool {
	if s == "" {
		return false
	}
	due := parseDueTime(s)
	if due.IsZero() {
		return false
	}
	return !due.After(now)
}

func formatDueLabel(s string) string {
	if s == "" {
		return ""
	}
	due := parseDueTime(s)
	if due.IsZero() {
		return s
	}
	now := time.Now()
	if due.Format("2006-01-02") == now.Format("2006-01-02") {
		return due.Format("15:04")
	}
	return due.Format("01-02 15:04")
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
		filtered := m.filteredCardIndices()
		if len(filtered) == 0 {
			b.WriteString(subtitleStyle.Render("No cards match the current filter."))
			b.WriteString("\n\n")
			b.WriteString(helpStyle.Render("[esc] back"))
			break
		}
		card := m.cards[filtered[m.cursor]]
		b.WriteString(labelStyle.Render(fmt.Sprintf("Editing card #%d", card.Card.ID)))
		b.WriteString("\n\n")

		fieldNames := []string{"Front", "Back", "Hint", "Example", "Example Translation", "Example Word"}
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
		excludeLabel := "Off"
		if m.editExcluded {
			excludeLabel = "On"
		}
		b.WriteString(labelStyle.Render("Excluded: " + excludeLabel))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[tab/↑↓] move  [enter] next/save  [ctrl+s] save  [ctrl+x] toggle exclude  [esc] cancel"))
		if m.errMsg != "" {
			b.WriteString("\n" + errorStyle.Render(m.errMsg))
		}

	case listStateNormal, listStateConfirmDelete, listStateConfirmExclude:
		filtered := m.filteredCardIndices()
		total := len(filtered)
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d cards shown / %d total", total, len(m.cards))))
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Search: %q  Excluded only: %t  Due only: %t  Sort: %s", m.filterInput.Value(), m.filterExcluded, m.filterDueOnly, m.sortModeLabel())))
		if m.filterActive {
			b.WriteString("  " + helpStyle.Render("(typing)"))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[/] search  [o] excluded only  [u] due only  [s] sort  [c] clear filters"))
		b.WriteString("\n\n")

		b.WriteString(subtitleStyle.Render(fmt.Sprintf("  %-4s %-3s %-12s %-20s %-45s %s", "ID", " x", "Front", "Back", "Example", "Due")))
		b.WriteString("\n")

		end := m.offset + listPageSize
		if end > total {
			end = total
		}
		for i := m.offset; i < end; i++ {
			c := m.cards[filtered[i]]
			mark := "   "
			if m.excluded[strings.ToLower(c.Front)] {
				mark = "[x]"
			}
			line := fmt.Sprintf("%-4d %s %-12s %-20s %-45s %s",
				c.Card.ID,
				mark,
				truncate(c.Front, 12),
				truncate(c.Back, 20),
				truncate(c.Example, 45),
				formatDueLabel(c.Review.DueDate),
			)
			if i == m.cursor {
				b.WriteString(inputLabelStyle.Render("> " + line))
			} else {
				b.WriteString(menuItemStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		if total == 0 {
			b.WriteString(subtitleStyle.Render("No cards match the current filters."))
			b.WriteString("\n")
		}

		if total > listPageSize {
			b.WriteString(helpStyle.Render(fmt.Sprintf("(%d-%d / %d)", m.offset+1, end, total)))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		switch m.state {
		case listStateConfirmDelete:
			card := m.cards[filtered[m.cursor]]
			b.WriteString(errorStyle.Render(fmt.Sprintf("Delete \"%s\"? ", truncate(card.Front, 30))))
			b.WriteString(helpStyle.Render("[y/enter] yes  [n/esc] no"))
		case listStateConfirmExclude:
			card := m.cards[filtered[m.cursor]]
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
