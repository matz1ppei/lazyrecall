package tui

// setup.go implements the first-run onboarding flow.
// When no cards exist, the user is guided to pick a language and
// import 20 starter words automatically — reusing the fetchlang batch logic
// but skipping the count-input step (always 20) for simplicity.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/dict"
)

const setupRequestedCount = 20

type setupState int

const (
	setupStatePrompt  setupState = iota // "Would you like to import starter cards? [y/n]"
	setupStateLang                      // language text input
	setupStateLoading                   // dict + AI batch generation in progress
	setupStateDone                      // success — shows count, waits for Enter
	setupStateError                     // error — shows message, waits for Enter
)

// SetupModel handles the first-run experience: language selection → 20-word auto-import.
type SetupModel struct {
	db               *sql.DB
	ai               ai.Client
	cfg              config.Config
	state            setupState
	langInput        textinput.Model
	spinner          spinner.Model
	progress         progress.Model
	langCode         string
	langName         string
	savedCount       int
	errMsg           string
	inlineErr        string
	pendingWords     []ai.WordPair
	totalBatches     int
	completedBatches int
	nextBatchIdx     int
	nextBatchNum     int
}

func NewSetupModel(database *sql.DB, aiClient ai.Client, cfg config.Config) SetupModel {
	li := textinput.New()
	li.Placeholder = "e.g. Spanish, French, Japanese"
	li.CharLimit = 64
	li.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = subtitleStyle

	pr := progress.New(progress.WithDefaultGradient())

	return SetupModel{
		db:        database,
		ai:        aiClient,
		cfg:       cfg,
		state:     setupStatePrompt,
		langInput: li,
		spinner:   sp,
		progress:  pr,
	}
}

func (m SetupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd

	case spinner.TickMsg:
		if m.state == setupStateLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case msgDictReady:
		if msg.err != nil {
			m.state = setupStateError
			m.errMsg = msg.err.Error()
			return m, nil
		}
		existingFronts, err := db.GetAllFronts(m.db)
		if err != nil {
			m.state = setupStateError
			m.errMsg = err.Error()
			return m, nil
		}
		seen := make(map[string]bool)
		var newWords []ai.WordPair
		for _, w := range msg.words {
			if len(newWords) >= setupRequestedCount {
				break
			}
			key := strings.ToLower(w)
			if existingFronts[key] || seen[key] {
				continue
			}
			seen[key] = true
			newWords = append(newWords, ai.WordPair{Front: w})
		}
		m.pendingWords = newWords
		return m.startCardGen()

	case msgCardGenResult:
		if m.state == setupStateError {
			return m, nil
		}
		if msg.err != nil {
			m.state = setupStateError
			m.errMsg = fmt.Sprintf("card gen batch %d/%d: %v", msg.batchNum, msg.totalBatches, msg.err)
			return m, nil
		}
		saved, _, err := saveBatch(m.db, msg.cards)
		m.savedCount += saved
		if err != nil {
			m.state = setupStateError
			m.errMsg = err.Error()
			return m, nil
		}
		m.completedBatches++
		pct := float64(m.completedBatches) / float64(msg.totalBatches)
		progressCmd := m.progress.SetPercent(pct)

		if m.completedBatches >= msg.totalBatches {
			m.state = setupStateDone
			m.saveConfig()
			return m, progressCmd
		}

		// Fire next batch if there are remaining words.
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
					wordStart: start, wordEnd: end - 1,
					topic: langName, elapsed: time.Since(t), err: err,
				}
			},
		)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.state == setupStateLang {
		var cmd tea.Cmd
		m.langInput, cmd = m.langInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m SetupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case setupStatePrompt:
		switch msg.String() {
		case "y", "Y":
			m.state = setupStateLang
			return m, m.langInput.Focus()
		case "n", "N", "esc":
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}

	case setupStateLang:
		switch msg.String() {
		case "esc":
			m.state = setupStatePrompt
			m.langInput.Blur()
			return m, nil
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
			m.state = setupStateLoading
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
			m.inlineErr = ""
			var cmd tea.Cmd
			m.langInput, cmd = m.langInput.Update(msg)
			return m, cmd
		}

	case setupStateDone, setupStateError:
		if msg.String() == "enter" || msg.String() == "esc" {
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome} }
		}
	}
	return m, nil
}

// saveConfig persists auto-add settings based on the setup language.
// Enables auto-add so the chosen language is used from the next startup.
func (m *SetupModel) saveConfig() {
	m.cfg.AutoAdd.Enabled = true
	m.cfg.AutoAdd.Language = m.langCode
	m.cfg.AutoAdd.LangName = m.langName
	m.cfg.AutoAdd.Count = setupRequestedCount
	_ = config.Save(m.cfg)
}

func (m SetupModel) startCardGen() (tea.Model, tea.Cmd) {
	if len(m.pendingWords) == 0 {
		m.state = setupStateDone
		return m, nil
	}

	totalBatches := (len(m.pendingWords) + cardGenBatchSize - 1) / cardGenBatchSize
	m.state = setupStateLoading
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

func (m SetupModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Welcome to LazyRecall!"))
	b.WriteString("\n\n")

	switch m.state {
	case setupStatePrompt:
		b.WriteString(subtitleStyle.Render("No cards found. Would you like to import 20 starter words?"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[y] Yes, let's go!  [n/esc] Skip for now"))

	case setupStateLang:
		b.WriteString(inputLabelStyle.Render("Language:"))
		b.WriteString("\n")
		b.WriteString(m.langInput.View())
		if m.inlineErr != "" {
			b.WriteString("\n")
			b.WriteString(errorStyle.Render(m.inlineErr))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] start  [esc] back"))

	case setupStateLoading:
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Importing %s words... %d/%d batches done", m.langName, m.completedBatches, m.totalBatches))
		b.WriteString("\n\n")
		b.WriteString(m.progress.View())
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Saved: %d", m.savedCount)))

	case setupStateDone:
		b.WriteString(successStyle.Render(fmt.Sprintf("%d cards imported!", m.savedCount)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] start learning"))

	case setupStateError:
		b.WriteString(errorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[enter] back to home"))
	}

	return b.String()
}
