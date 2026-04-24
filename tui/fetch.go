package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
)

const (
	wordListBatchSize = 10
	cardGenBatchSize  = 25
	cardGenParallel   = 3
	maxExcludeWords   = 50
)

type fetchState int

const (
	fetchStateIdle fetchState = iota
	fetchStateCount
	fetchStateLoading  // single batch (count <= cardGenBatchSize)
	fetchStateWordList // phase 1: building word list via LLM
	fetchStateCardGen  // phase 2: generating cards for new words
	fetchStatePreview
	fetchStateSaved
	fetchStateError
	fetchStateNoAI
)

type msgFetchResult struct {
	cards []ai.GeneratedCard
	err   error
}

type msgWordListResult struct {
	pairs        []ai.WordPair
	batchNum     int
	totalBatches int
	rankStart    int
	rankEnd      int
	nextStart    int
	topic        string
	totalCount   int
	elapsed      time.Duration
	err          error
}

type msgCardGenResult struct {
	cards        []ai.GeneratedCard
	batchNum     int
	totalBatches int
	wordStart    int
	wordEnd      int
	topic        string
	elapsed      time.Duration
	err          error
}

type msgFetchSaved struct {
	saved   int
	skipped int
	err     error
}

type FetchModel struct {
	db           *sql.DB
	ai           ai.Client
	state        fetchState
	topicInput   textinput.Model
	countInput   textinput.Model
	spinner      spinner.Model
	progress     progress.Model
	cards        []ai.GeneratedCard
	previewIndex int
	errMsg       string
	savedCount   int
	skippedCount int
	// word list phase
	pendingWords  []ai.WordPair
	dbExclude     []string
	wordListDone  int
	wordListTotal int
	// card gen phase
	totalBatches     int
	completedBatches int
	nextBatchIdx     int
	nextBatchNum     int
}

func NewFetchModel(database *sql.DB, aiClient ai.Client) FetchModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. Python design patterns, French idioms"
	ti.CharLimit = 256
	ti.Focus()

	ci := textinput.New()
	ci.Placeholder = "5"
	ci.CharLimit = 5

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = subtitleStyle

	pr := progress.New(progress.WithDefaultGradient())

	m := FetchModel{
		db:         database,
		ai:         aiClient,
		topicInput: ti,
		countInput: ci,
		spinner:    sp,
		progress:   pr,
	}
	if aiClient == nil {
		m.state = fetchStateNoAI
	} else {
		m.state = fetchStateIdle
	}
	return m
}

func (m FetchModel) Init() tea.Cmd {
	if m.state == fetchStateNoAI {
		return nil
	}
	return textinput.Blink
}

func (m FetchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd

	case spinner.TickMsg:
		if m.state == fetchStateLoading || m.state == fetchStateWordList || m.state == fetchStateCardGen {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case msgFetchResult:
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = msg.err.Error()
		} else {
			m.state = fetchStatePreview
			m.cards = msg.cards
			m.previewIndex = 0
		}
		return m, nil

	case msgWordListResult:
		db.RecordBatchStat(m.db, msg.topic, msg.batchNum, msg.totalBatches, msg.rankStart, msg.rankEnd, msg.elapsed, msg.err)
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = fmt.Sprintf("word list batch %d/%d: %v", msg.batchNum, msg.totalBatches, msg.err)
			return m, nil
		}
		m.pendingWords = append(m.pendingWords, msg.pairs...)
		m.wordListDone = msg.batchNum

		if msg.nextStart > msg.totalCount {
			return m.startCardGen(msg.topic)
		}
		nextEnd := msg.nextStart + wordListBatchSize - 1
		if nextEnd > msg.totalCount {
			nextEnd = msg.totalCount
		}
		aiClient := m.ai
		topic := msg.topic
		totalCount := msg.totalCount
		totalBatches := msg.totalBatches
		batchNum := msg.batchNum + 1
		nextStart := msg.nextStart
		exclude := m.buildExcludeList()
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg {
				t := time.Now()
				pairs, err := aiClient.GenerateWordList(context.Background(), topic, nextStart, nextEnd, exclude)
				return msgWordListResult{
					pairs: pairs, batchNum: batchNum, totalBatches: totalBatches,
					rankStart: nextStart, rankEnd: nextEnd, nextStart: nextEnd + 1,
					topic: topic, totalCount: totalCount, elapsed: time.Since(t), err: err,
				}
			},
		)

	case msgCardGenResult:
		if m.state == fetchStateError {
			return m, nil
		}
		db.RecordBatchStat(m.db, msg.topic, msg.batchNum, msg.totalBatches, msg.wordStart, msg.wordEnd, msg.elapsed, msg.err)
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = fmt.Sprintf("card gen batch %d/%d: %v", msg.batchNum, msg.totalBatches, msg.err)
			return m, nil
		}
		saved, _, err := saveBatch(m.db, msg.cards)
		m.savedCount += saved
		if err != nil {
			m.state = fetchStateError
			m.errMsg = err.Error()
			return m, nil
		}
		m.completedBatches++
		pct := float64(m.completedBatches) / float64(msg.totalBatches)
		progressCmd := m.progress.SetPercent(pct)

		if m.completedBatches >= msg.totalBatches {
			m.state = fetchStateSaved
			return m, progressCmd
		}

		if m.nextBatchIdx >= len(m.pendingWords) {
			return m, tea.Batch(progressCmd, m.spinner.Tick)
		}

		start := m.nextBatchIdx
		end := start + cardGenBatchSize
		if end > len(m.pendingWords) {
			end = len(m.pendingWords)
		}
		words := m.pendingWords[start:end]
		m.nextBatchIdx = end
		batchNum := m.nextBatchNum
		m.nextBatchNum++
		wordStart := start
		wordEnd := end - 1
		aiClient := m.ai
		topic := msg.topic
		totalBatches := msg.totalBatches

		return m, tea.Batch(
			progressCmd,
			m.spinner.Tick,
			func() tea.Msg {
				t := time.Now()
				cards, err := aiClient.GenerateCardsFromWords(context.Background(), words)
				return msgCardGenResult{
					cards: cards, batchNum: batchNum, totalBatches: totalBatches,
					wordStart: wordStart, wordEnd: wordEnd,
					topic: topic, elapsed: time.Since(t), err: err,
				}
			},
		)

	case msgFetchSaved:
		if msg.err != nil {
			m.state = fetchStateError
			m.errMsg = msg.err.Error()
		} else {
			m.state = fetchStateSaved
			m.savedCount = msg.saved
			m.skippedCount = msg.skipped
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	switch m.state {
	case fetchStateIdle:
		var cmd tea.Cmd
		m.topicInput, cmd = m.topicInput.Update(msg)
		return m, cmd
	case fetchStateCount:
		var cmd tea.Cmd
		m.countInput, cmd = m.countInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m FetchModel) startCardGen(topic string) (tea.Model, tea.Cmd) {
	existingFronts, err := db.GetAllFronts(m.db)
	if err != nil {
		m.state = fetchStateError
		m.errMsg = err.Error()
		return m, nil
	}
	seen := make(map[string]bool)
	var newWords []ai.WordPair
	for _, p := range m.pendingWords {
		key := strings.ToLower(p.Front)
		if !existingFronts[key] && !seen[key] {
			seen[key] = true
			newWords = append(newWords, p)
		}
	}
	m.skippedCount = len(m.pendingWords) - len(newWords)
	m.pendingWords = newWords

	if len(m.pendingWords) == 0 {
		m.state = fetchStateSaved
		return m, nil
	}

	totalBatches := (len(m.pendingWords) + cardGenBatchSize - 1) / cardGenBatchSize
	m.state = fetchStateCardGen
	m.completedBatches = 0
	m.totalBatches = totalBatches
	m.nextBatchIdx = 0
	m.nextBatchNum = 1

	var cmds []tea.Cmd
	cmds = append(cmds, m.spinner.Tick, m.progress.SetPercent(0))

	for i := 0; i < cardGenParallel && m.nextBatchIdx < len(m.pendingWords); i++ {
		start := m.nextBatchIdx
		end := start + cardGenBatchSize
		if end > len(m.pendingWords) {
			end = len(m.pendingWords)
		}
		words := m.pendingWords[start:end]
		m.nextBatchIdx = end
		batchNum := m.nextBatchNum
		m.nextBatchNum++
		wordStart := start
		wordEnd := end - 1
		aiClient := m.ai
		topicVal := topic
		tb := totalBatches

		cmds = append(cmds, func() tea.Msg {
			t := time.Now()
			cards, err := aiClient.GenerateCardsFromWords(context.Background(), words)
			return msgCardGenResult{
				cards: cards, batchNum: batchNum, totalBatches: tb,
				wordStart: wordStart, wordEnd: wordEnd,
				topic: topicVal, elapsed: time.Since(t), err: err,
			}
		})
	}

	return m, tea.Batch(cmds...)
}

func (m FetchModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case fetchStateNoAI:
		if msg.String() == "enter" || msg.String() == "esc" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchStateIdle:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			if strings.TrimSpace(m.topicInput.Value()) == "" {
				return m, nil
			}
			m.topicInput.Blur()
			m.state = fetchStateCount
			return m, m.countInput.Focus()
		default:
			var cmd tea.Cmd
			m.topicInput, cmd = m.topicInput.Update(msg)
			return m, cmd
		}

	case fetchStateCount:
		switch msg.String() {
		case "esc":
			m.state = fetchStateIdle
			m.countInput.SetValue("")
			return m, m.topicInput.Focus()
		case "enter":
			count := m.parseCount()
			topic := strings.TrimSpace(m.topicInput.Value())
			m.countInput.Blur()
			m.savedCount = 0
			m.skippedCount = 0
			m.pendingWords = nil

			if count <= cardGenBatchSize {
				m.state = fetchStateLoading
				aiClient := m.ai
				return m, tea.Batch(
					m.spinner.Tick,
					func() tea.Msg {
						cards, err := aiClient.GenerateCards(context.Background(), topic, 1, count)
						return msgFetchResult{cards: cards, err: err}
					},
				)
			}
			totalWordBatches := (count + wordListBatchSize - 1) / wordListBatchSize
			m.state = fetchStateWordList
			m.pendingWords = nil
			m.wordListDone = 0
			m.wordListTotal = totalWordBatches
			m.dbExclude, _ = db.GetRecentFronts(m.db, maxExcludeWords)
			firstEnd := wordListBatchSize
			if firstEnd > count {
				firstEnd = count
			}
			aiClient := m.ai
			topicVal := topic
			countVal := count
			initialExclude := m.buildExcludeList()
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					t := time.Now()
					pairs, err := aiClient.GenerateWordList(context.Background(), topicVal, 1, firstEnd, initialExclude)
					return msgWordListResult{
						pairs: pairs, batchNum: 1, totalBatches: totalWordBatches,
						rankStart: 1, rankEnd: firstEnd, nextStart: firstEnd + 1,
						topic: topicVal, totalCount: countVal, elapsed: time.Since(t), err: err,
					}
				},
			)
		default:
			var cmd tea.Cmd
			m.countInput, cmd = m.countInput.Update(msg)
			return m, cmd
		}

	case fetchStatePreview:
		switch msg.String() {
		case "y", "enter":
			return m, m.saveCards()
		case "n", "esc":
			m.state = fetchStateIdle
			m.topicInput.SetValue("")
			m.countInput.SetValue("")
			m.cards = nil
			return m, m.topicInput.Focus()
		case "left", "h":
			if m.previewIndex > 0 {
				m.previewIndex--
			}
		case "right", "l":
			if m.previewIndex < len(m.cards)-1 {
				m.previewIndex++
			}
		}

	case fetchStateSaved:
		if msg.String() == "enter" || msg.String() == " " {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchStateError:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.state = fetchStateIdle
			m.topicInput.SetValue("")
			m.countInput.SetValue("")
			m.pendingWords = nil
			return m, m.topicInput.Focus()
		}
	}
	return m, nil
}

func (m FetchModel) parseCount() int {
	v := strings.TrimSpace(m.countInput.Value())
	if v == "" {
		return 5
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// buildExcludeList returns up to maxExcludeWords exclusion words.
// Recent pending words take priority; remaining slots are filled from DB fronts.
func (m FetchModel) buildExcludeList() []string {
	pendingStart := 0
	if len(m.pendingWords) > maxExcludeWords {
		pendingStart = len(m.pendingWords) - maxExcludeWords
	}
	recent := m.pendingWords[pendingStart:]

	remaining := maxExcludeWords - len(recent)
	dbStart := 0
	if len(m.dbExclude) > remaining {
		dbStart = len(m.dbExclude) - remaining
	}
	dbSlice := m.dbExclude[dbStart:]

	out := make([]string, 0, len(recent)+len(dbSlice))
	for _, p := range recent {
		out = append(out, p.Front)
	}
	return append(out, dbSlice...)
}

func saveBatch(database *sql.DB, cards []ai.GeneratedCard) (saved, skipped int, err error) {
	for _, c := range cards {
		dups, e := db.FindCardsByFront(database, c.Front)
		if e != nil {
			return saved, skipped, e
		}
		if len(dups) > 0 {
			skipped++
			continue
		}
		_, e = db.CreateCardWithReview(database, c.Front, c.Back, c.Hint, c.Example, c.ExampleTranslation, c.ExampleWord)
		if e != nil {
			return saved, skipped, e
		}
		saved++
	}
	return saved, skipped, nil
}

func (m FetchModel) saveCards() tea.Cmd {
	cards := m.cards
	database := m.db
	return func() tea.Msg {
		saved, skipped, err := saveBatch(database, cards)
		return msgFetchSaved{saved: saved, skipped: skipped, err: err}
	}
}

func (m FetchModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Fetch: Topic (AI)"))
	b.WriteString("\n\n")

	switch m.state {
	case fetchStateNoAI:
		b.WriteString(errorStyle.Render("AI not configured. Set AI_BACKEND and credentials."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchStateIdle:
		b.WriteString(inputLabelStyle.Render("Topic:"))
		b.WriteString("\n")
		b.WriteString(m.topicInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] next  [esc] cancel"))

	case fetchStateCount:
		b.WriteString(subtitleStyle.Render("Topic: " + m.topicInput.Value()))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("How many cards? (default: 5)"))
		b.WriteString("\n")
		b.WriteString(m.countInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] generate  [esc] back"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("* Over 25: 2-step generation (word list → cards), no preview"))

	case fetchStateLoading:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Generating %s cards...", m.countInput.Value()))

	case fetchStateWordList:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Building word list... %d/%d", m.wordListDone, m.wordListTotal))

	case fetchStateCardGen:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Generating cards... %d/%d batches done", m.completedBatches, m.totalBatches))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("New words: %d  (skipped %d duplicates)", len(m.pendingWords), m.skippedCount)))
		b.WriteString("\n\n")
		b.WriteString(m.progress.View())
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Saved: %d", m.savedCount)))

	case fetchStatePreview:
		total := len(m.cards)
		idx := m.previewIndex
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Preview: %d/%d cards", idx+1, total)))
		b.WriteString("\n\n")
		card := m.cards[idx]
		b.WriteString(inputLabelStyle.Render("Front:   ") + card.Front + "\n")
		b.WriteString(inputLabelStyle.Render("Back:    ") + card.Back + "\n")
		if card.Hint != "" {
			b.WriteString(inputLabelStyle.Render("Hint:    ") + card.Hint + "\n")
		}
		if card.Example != "" {
			b.WriteString(inputLabelStyle.Render("Example: ") + card.Example + "\n")
		}
		b.WriteString("\n")
		nav := ""
		if total > 1 {
			nav = fmt.Sprintf("  %s/%s browse  ", keyStyle.Render("[←]"), keyStyle.Render("[→]"))
		}
		b.WriteString(fmt.Sprintf("%s save all  %s discard%s",
			keyStyle.Render("[y/enter]"),
			keyStyle.Render("[n/esc]"),
			nav,
		))

	case fetchStateSaved:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d card(s) saved!", m.savedCount)))
		if m.skippedCount > 0 {
			b.WriteString("\n")
			b.WriteString(labelStyle.Render(fmt.Sprintf("%d skipped (duplicate front)", m.skippedCount)))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchStateError:
		b.WriteString(errorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] try again"))
	}

	return b.String()
}
