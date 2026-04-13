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
	"github.com/ippei/lazyrecall/dict"
)

type fetchLangState int

const (
	fetchLangIdle        fetchLangState = iota
	fetchLangCount
	fetchLangDownloading
	fetchLangCardGen
	fetchLangSaved
	fetchLangError
	fetchLangNoAI
)

type msgDictReady struct {
	words []string
	lang  string
	err   error
}

type FetchLangModel struct {
	db             *sql.DB
	ai             ai.Client
	state          fetchLangState
	langInput      textinput.Model
	countInput     textinput.Model
	spinner        spinner.Model
	progress       progress.Model
	inlineErr      string
	errMsg         string
	savedCount     int
	skippedCount   int
	langCode       string
	langName       string
	requestedCount int
	pendingWords     []ai.WordPair
	totalBatches     int
	completedBatches int
	nextBatchIdx     int
	nextBatchNum     int
}

func NewFetchLangModel(database *sql.DB, aiClient ai.Client) FetchLangModel {
	li := textinput.New()
	li.Placeholder = "e.g. Spanish, French, Japanese"
	li.CharLimit = 64
	li.Focus()

	ci := textinput.New()
	ci.Placeholder = "20"
	ci.CharLimit = 5

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = subtitleStyle

	pr := progress.New(progress.WithDefaultGradient())

	m := FetchLangModel{
		db:         database,
		ai:         aiClient,
		langInput:  li,
		countInput: ci,
		spinner:    sp,
		progress:   pr,
	}
	if aiClient == nil {
		m.state = fetchLangNoAI
	} else {
		m.state = fetchLangIdle
	}
	return m
}

func (m FetchLangModel) Init() tea.Cmd {
	if m.state == fetchLangNoAI {
		return nil
	}
	return textinput.Blink
}

func (m FetchLangModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd

	case spinner.TickMsg:
		if m.state == fetchLangDownloading || m.state == fetchLangCardGen {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case msgDictReady:
		if msg.err != nil {
			m.state = fetchLangError
			m.errMsg = msg.err.Error()
			return m, nil
		}
		existingFronts, err := db.GetAllFronts(m.db)
		if err != nil {
			m.state = fetchLangError
			m.errMsg = err.Error()
			return m, nil
		}
		seen := make(map[string]bool)
		var newWords []ai.WordPair
		dbSkipped := 0
		for _, w := range msg.words {
			if len(newWords) >= m.requestedCount {
				break
			}
			key := strings.ToLower(w)
			if existingFronts[key] {
				dbSkipped++
				continue
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			newWords = append(newWords, ai.WordPair{Front: w})
		}
		m.pendingWords = newWords
		m.skippedCount = dbSkipped
		return m.startCardGen()

	case msgCardGenResult:
		if m.state == fetchLangError {
			return m, nil
		}
		db.RecordBatchStat(m.db, msg.topic, msg.batchNum, msg.totalBatches, msg.wordStart, msg.wordEnd, msg.elapsed, msg.err)
		if msg.err != nil {
			m.state = fetchLangError
			m.errMsg = fmt.Sprintf("card gen batch %d/%d: %v", msg.batchNum, msg.totalBatches, msg.err)
			return m, nil
		}
		saved, _, err := saveBatch(m.db, msg.cards)
		m.savedCount += saved
		if err != nil {
			m.state = fetchLangError
			m.errMsg = err.Error()
			return m, nil
		}
		m.completedBatches++
		pct := float64(m.completedBatches) / float64(msg.totalBatches)
		progressCmd := m.progress.SetPercent(pct)

		if m.completedBatches >= msg.totalBatches {
			m.state = fetchLangSaved
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
		langName := m.langName
		totalBatches := msg.totalBatches
		fronts := make([]string, len(words))
		for i, w := range words {
			fronts[i] = w.Front
		}

		return m, tea.Batch(
			progressCmd,
			m.spinner.Tick,
			func() tea.Msg {
				t := time.Now()
				cards, err := aiClient.GenerateCardsForWords(context.Background(), langName, fronts)
				return msgCardGenResult{
					cards: cards, batchNum: batchNum, totalBatches: totalBatches,
					wordStart: wordStart, wordEnd: wordEnd,
					topic: langName, elapsed: time.Since(t), err: err,
				}
			},
		)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	switch m.state {
	case fetchLangIdle:
		var cmd tea.Cmd
		m.langInput, cmd = m.langInput.Update(msg)
		return m, cmd
	case fetchLangCount:
		var cmd tea.Cmd
		m.countInput, cmd = m.countInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m FetchLangModel) startCardGen() (tea.Model, tea.Cmd) {
	if len(m.pendingWords) == 0 {
		m.state = fetchLangSaved
		return m, nil
	}

	totalBatches := (len(m.pendingWords) + cardGenBatchSize - 1) / cardGenBatchSize
	m.state = fetchLangCardGen
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
		langName := m.langName
		tb := totalBatches
		fronts := make([]string, len(words))
		for j, w := range words {
			fronts[j] = w.Front
		}

		cmds = append(cmds, func() tea.Msg {
			t := time.Now()
			cards, err := aiClient.GenerateCardsForWords(context.Background(), langName, fronts)
			return msgCardGenResult{
				cards: cards, batchNum: batchNum, totalBatches: tb,
				wordStart: wordStart, wordEnd: wordEnd,
				topic: langName, elapsed: time.Since(t), err: err,
			}
		})
	}

	return m, tea.Batch(cmds...)
}

func (m FetchLangModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case fetchLangNoAI:
		if msg.String() == "enter" || msg.String() == "esc" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchLangIdle:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		case "enter":
			input := strings.TrimSpace(m.langInput.Value())
			if input == "" {
				return m, nil
			}
			code, name, ok := dict.DetectLang(input)
			if !ok {
				m.inlineErr = "Unknown language. Try: Spanish, French, Japanese, Korean, German..."
				return m, nil
			}
			m.langCode = code
			m.langName = name
			m.inlineErr = ""
			m.langInput.Blur()
			m.state = fetchLangCount
			return m, m.countInput.Focus()
		default:
			m.inlineErr = ""
			var cmd tea.Cmd
			m.langInput, cmd = m.langInput.Update(msg)
			return m, cmd
		}

	case fetchLangCount:
		switch msg.String() {
		case "esc":
			m.state = fetchLangIdle
			m.countInput.SetValue("")
			return m, m.langInput.Focus()
		case "enter":
			m.requestedCount = m.parseCount()
			m.savedCount = 0
			m.skippedCount = 0
			m.pendingWords = nil
			m.countInput.Blur()
			m.state = fetchLangDownloading
			langCode := m.langCode
			langName := m.langName
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					words, err := dict.GetWords(langCode, 0)
					return msgDictReady{words: words, lang: langName, err: err}
				},
			)
		default:
			var cmd tea.Cmd
			m.countInput, cmd = m.countInput.Update(msg)
			return m, cmd
		}

	case fetchLangSaved:
		if msg.String() == "enter" || msg.String() == " " {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case fetchLangError:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.state = fetchLangIdle
			m.langInput.SetValue("")
			m.countInput.SetValue("")
			m.pendingWords = nil
			return m, m.langInput.Focus()
		}
	}
	return m, nil
}

func (m FetchLangModel) parseCount() int {
	v := strings.TrimSpace(m.countInput.Value())
	if v == "" {
		return 20
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func (m FetchLangModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Fetch: Language"))
	b.WriteString("\n\n")

	switch m.state {
	case fetchLangNoAI:
		b.WriteString(errorStyle.Render("AI not configured. Set AI_BACKEND and credentials."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchLangIdle:
		b.WriteString(inputLabelStyle.Render("Language:"))
		b.WriteString("\n")
		b.WriteString(m.langInput.View())
		if m.inlineErr != "" {
			b.WriteString("\n")
			b.WriteString(errorStyle.Render(m.inlineErr))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] next  [esc] cancel"))

	case fetchLangCount:
		b.WriteString(subtitleStyle.Render("Language: " + m.langName))
		b.WriteString("\n\n")
		b.WriteString(inputLabelStyle.Render("How many cards? (default: 20)"))
		b.WriteString("\n")
		b.WriteString(m.countInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] start  [esc] back"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("* Picks up from where you left off (skips cards already in deck)"))

	case fetchLangDownloading:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Downloading %s frequency dictionary...", m.langName))

	case fetchLangCardGen:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Generating cards... %d/%d batches done", m.completedBatches, m.totalBatches))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("New words: %d  (skipped %d already in deck)", len(m.pendingWords), m.skippedCount)))
		b.WriteString("\n\n")
		b.WriteString(m.progress.View())
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Saved: %d", m.savedCount)))

	case fetchLangSaved:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d card(s) saved!", m.savedCount)))
		if m.skippedCount > 0 {
			b.WriteString("\n")
			b.WriteString(labelStyle.Render(fmt.Sprintf("%d already in deck (skipped)", m.skippedCount)))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))

	case fetchLangError:
		b.WriteString(errorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] try again"))
	}

	return b.String()
}
