package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/dict"
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
	homeStateConfigure
)

const dailyReviewLimit = 100

type msgStats struct {
	total           int
	due             int
	overdue         int
	reviewedToday   int
	practiceToday   db.PracticeTodayStats
	session         db.DailySession
	resumeAvailable bool
	notifyFatigue   bool
	notifyBenchmark bool
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

type msgAutoAddDone struct {
	saved int
	err   error
}

type msgNotificationReset struct{}

type HomeModel struct {
	db              *sql.DB
	ai              ai.Client
	cfg             config.Config
	state           homeState
	total           int
	due             int
	overdue         int
	reviewedToday   int
	practiceToday   db.PracticeTodayStats
	statsReady      bool
	autoAdding      bool
	session         db.DailySession
	resumeAvailable bool
	importInput     textinput.Model
	importMsg       string
	statusMsg       string // shown when navigating back from a session phase unexpectedly
	notifyFatigue   bool
	notifyBenchmark bool
	// configure state
	cfgLangInput         textinput.Model
	cfgCountInput        textinput.Model
	cfgProfileInput      textinput.Model
	cfgFeedbackLangInput textinput.Model
	cfgEnabled           bool
	cfgInlineErr         string
	cfgFocus             int // 0=enabled, 1=lang, 2=count, 3=profile, 4=feedback_lang
}

func NewHomeModel(database *sql.DB, aiClient ai.Client, cfg config.Config) HomeModel {
	ti := textinput.New()
	ti.Placeholder = "path/to/cards.csv"
	ti.CharLimit = 256

	langInput := textinput.New()
	langInput.Placeholder = "e.g. Spanish, French, Japanese"
	langInput.CharLimit = 64

	countInput := textinput.New()
	countInput.Placeholder = "20"
	countInput.CharLimit = 5

	profileInput := textinput.New()
	profileInput.Placeholder = "e.g. 東京在住でカミーノ・デ・サンティアゴに行く準備をしている"
	profileInput.CharLimit = 256

	feedbackLangInput := textinput.New()
	feedbackLangInput.Placeholder = "e.g. Japanese, English, Spanish"
	feedbackLangInput.CharLimit = 64

	return HomeModel{
		db:                   database,
		ai:                   aiClient,
		cfg:                  cfg,
		importInput:          ti,
		cfgLangInput:         langInput,
		cfgCountInput:        countInput,
		cfgProfileInput:      profileInput,
		cfgFeedbackLangInput: feedbackLangInput,
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
		practiceToday, _ := db.GetTodayPracticeStats(database)
		session, _ := db.GetTodaySession(database)
		resumeAvailable := false
		if snapshot, err := config.LoadDailySessionSnapshot(); err == nil {
			resumeAvailable = snapshot.Date == time.Now().Format("2006-01-02") && len(snapshot.CardIDs) > 0
		}

		totalSessions, _ := db.CountAllReviewSessions(database)
		clearedAt, _ := db.GetMilestoneInt(database, "fatigue_cleared_at_count")
		notifyFatigue := totalSessions >= 20 && (totalSessions-clearedAt) >= 20

		firstBenchmark, _ := db.FirstBenchmarkRunAt(database)
		benchmarkCleared, _ := db.GetMilestoneInt(database, "benchmark_cleared")
		notifyBenchmark := !firstBenchmark.IsZero() &&
			time.Since(firstBenchmark) >= 60*24*time.Hour &&
			benchmarkCleared == 0

		return msgStats{
			total: len(cards), due: due, overdue: overdue,
			reviewedToday: reviewedToday, practiceToday: practiceToday, session: session, resumeAvailable: resumeAvailable,
			notifyFatigue: notifyFatigue, notifyBenchmark: notifyBenchmark,
		}
	}
}

func (h HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgStats:
		h.total = msg.total
		h.due = msg.due
		h.overdue = msg.overdue
		h.reviewedToday = msg.reviewedToday
		h.practiceToday = msg.practiceToday
		h.session = msg.session
		h.resumeAvailable = msg.resumeAvailable
		h.notifyFatigue = msg.notifyFatigue
		h.notifyBenchmark = msg.notifyBenchmark
		h.statsReady = true
		// Redirect first-time users to the onboarding setup flow.
		if h.total == 0 {
			return h, func() tea.Msg { return MsgGotoScreen{Target: screenSetup} }
		}
		// Trigger auto-add only after stats are loaded so we know the DB is not empty.
		// session.AutoAddDone comes from loadStats, avoiding a separate DB query.
		if h.cfg.AutoAdd.Enabled && h.ai != nil && h.cfg.AutoAdd.Language != "" && !msg.session.AutoAddDone {
			h.autoAdding = true
			return h, h.autoAddCmd()
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

	case msgAutoAddDone:
		h.autoAdding = false
		if msg.err != nil {
			h.importMsg = errorStyle.Render(fmt.Sprintf("Auto-add error: %v", msg.err))
		} else if msg.saved > 0 {
			h.importMsg = successStyle.Render(fmt.Sprintf("Auto-added %d cards today.", msg.saved))
		}
		return h, h.loadStats()

	case msgNotificationReset:
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
		if h.state == homeStateConfigure {
			return h.handleConfigureKey(msg)
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
	case "l":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenList} }
	case "s":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenStats} }
	case "b":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenBenchmark} }
	case "t":
		h.state = homeStateTools
		h.importMsg = ""
		return h, nil
	case "c":
		h.state = homeStateConfigure
		h.cfgEnabled = h.cfg.AutoAdd.Enabled
		h.cfgLangInput.SetValue(h.cfg.AutoAdd.LangName)
		h.cfgCountInput.SetValue(fmt.Sprintf("%d", h.cfg.AutoAdd.Count))
		h.cfgProfileInput.SetValue(h.cfg.UserProfile)
		h.cfgFeedbackLangInput.SetValue(h.cfg.FeedbackLanguage)
		h.cfgFocus = 0
		h.cfgInlineErr = ""
		h.cfgLangInput.Blur()
		h.cfgCountInput.Blur()
		h.cfgProfileInput.Blur()
		h.cfgFeedbackLangInput.Blur()
		return h, nil
	case "1":
		if h.notifyFatigue {
			return h, h.resetFatigueNotificationCmd()
		}
	case "2":
		if h.notifyBenchmark {
			return h, h.resetBenchmarkNotificationCmd()
		}
	case "q":
		return h, tea.Quit
	}
	return h, nil
}

func (h HomeModel) resetFatigueNotificationCmd() tea.Cmd {
	database := h.db
	return func() tea.Msg {
		total, _ := db.CountAllReviewSessions(database)
		_ = db.SetMilestoneInt(database, "fatigue_cleared_at_count", total)
		return msgNotificationReset{}
	}
}

func (h HomeModel) resetBenchmarkNotificationCmd() tea.Cmd {
	database := h.db
	return func() tea.Msg {
		_ = db.SetMilestoneInt(database, "benchmark_cleared", 1)
		return msgNotificationReset{}
	}
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
	case "c":
		if h.ai == nil {
			h.state = homeStateNormal
			h.importMsg = errorStyle.Render("AI not configured. Compose requires an AI backend.")
			return h, nil
		}
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenCompose} }
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
	case "a":
		return h, func() tea.Msg { return MsgGotoScreen{Target: screenAdd} }
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

func (h HomeModel) handleConfigureKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		h.state = homeStateNormal
		h.cfgLangInput.Blur()
		h.cfgCountInput.Blur()
		h.cfgProfileInput.Blur()
		h.cfgFeedbackLangInput.Blur()
		return h, nil
	case "tab", "down":
		h.cfgFocus = (h.cfgFocus + 1) % 5
		h, cmd := h.syncCfgFocus()
		return h, cmd
	case "shift+tab", "up":
		h.cfgFocus = (h.cfgFocus + 4) % 5
		h, cmd := h.syncCfgFocus()
		return h, cmd
	case "enter":
		if h.cfgFocus == 0 {
			h.cfgEnabled = !h.cfgEnabled
			return h, nil
		}
		if h.cfgFocus < 4 {
			h.cfgFocus++
			h, cmd := h.syncCfgFocus()
			return h, cmd
		}
		// focus==4: save
		return h.saveConfig()
	}
	var cmd tea.Cmd
	if h.cfgFocus == 1 {
		h.cfgLangInput, cmd = h.cfgLangInput.Update(msg)
	} else if h.cfgFocus == 2 {
		h.cfgCountInput, cmd = h.cfgCountInput.Update(msg)
	} else if h.cfgFocus == 3 {
		h.cfgProfileInput, cmd = h.cfgProfileInput.Update(msg)
	} else if h.cfgFocus == 4 {
		h.cfgFeedbackLangInput, cmd = h.cfgFeedbackLangInput.Update(msg)
	}
	return h, cmd
}

// syncCfgFocus blurs all configure inputs, then focuses the active one.
// Returns the updated HomeModel (with correct focus state) and the focus Cmd.
func (h HomeModel) syncCfgFocus() (HomeModel, tea.Cmd) {
	h.cfgLangInput.Blur()
	h.cfgCountInput.Blur()
	h.cfgProfileInput.Blur()
	h.cfgFeedbackLangInput.Blur()
	switch h.cfgFocus {
	case 1:
		return h, h.cfgLangInput.Focus()
	case 2:
		return h, h.cfgCountInput.Focus()
	case 3:
		return h, h.cfgProfileInput.Focus()
	case 4:
		return h, h.cfgFeedbackLangInput.Focus()
	}
	return h, nil
}

func (h HomeModel) saveConfig() (tea.Model, tea.Cmd) {
	langName := strings.TrimSpace(h.cfgLangInput.Value())
	if langName == "" && h.cfgEnabled {
		h.cfgInlineErr = "Language is required when auto-add is enabled."
		return h, nil
	}
	var langCode string
	if langName != "" {
		code, name, ok := dict.DetectLang(langName)
		if !ok {
			h.cfgInlineErr = fmt.Sprintf("Unknown language: %q", langName)
			return h, nil
		}
		langCode = code
		langName = name
	}
	count := h.cfg.AutoAdd.Count
	if raw := strings.TrimSpace(h.cfgCountInput.Value()); raw != "" {
		n := 0
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				h.cfgInlineErr = "Count must be a positive integer."
				return h, nil
			}
			n = n*10 + int(ch-'0')
		}
		if n <= 0 {
			h.cfgInlineErr = "Count must be greater than 0."
			return h, nil
		}
		count = n
	}
	h.cfg.AutoAdd.Enabled = h.cfgEnabled
	h.cfg.AutoAdd.Language = langCode
	h.cfg.AutoAdd.LangName = langName
	h.cfg.AutoAdd.Count = count
	h.cfg.UserProfile = strings.TrimSpace(h.cfgProfileInput.Value())
	h.cfg.FeedbackLanguage = strings.TrimSpace(h.cfgFeedbackLangInput.Value())
	cfg := h.cfg
	_ = config.Save(cfg)
	// ライブクライアントにもプロフィールを即時反映（再起動不要）
	type profileSetter interface{ SetUserProfile(string) }
	if ps, ok := h.ai.(profileSetter); ok {
		ps.SetUserProfile(h.cfg.UserProfile)
	}
	h.state = homeStateNormal
	h.cfgInlineErr = ""
	h.importMsg = successStyle.Render("Settings saved.")
	return h, nil
}

func (h HomeModel) autoAddCmd() tea.Cmd {
	database := h.db
	aiClient := h.ai
	cfg := h.cfg
	return func() tea.Msg {
		return runAutoAdd(database, aiClient, cfg)
	}
}

func runAutoAdd(database *sql.DB, aiClient ai.Client, cfg config.Config) tea.Msg {
	existingFronts, err := db.GetAllFronts(database)
	if err != nil {
		return msgAutoAddDone{err: err}
	}
	excluded, _ := config.LoadExcludedWords()
	words, err := dict.GetWords(cfg.AutoAdd.Language, 0)
	if err != nil {
		return msgAutoAddDone{err: err}
	}
	var newFronts []string
	seen := make(map[string]bool)
	for _, w := range words {
		if len(newFronts) >= cfg.AutoAdd.Count {
			break
		}
		key := strings.ToLower(w)
		if excluded[key] || existingFronts[key] || seen[key] {
			continue
		}
		seen[key] = true
		newFronts = append(newFronts, w)
	}
	if len(newFronts) == 0 {
		_ = db.MarkAutoAddDone(database)
		return msgAutoAddDone{}
	}
	cards, err := aiClient.GenerateCardsForWords(context.Background(), cfg.AutoAdd.LangName, newFronts)
	if err != nil {
		return msgAutoAddDone{err: err}
	}
	saved, _, err := saveBatch(database, cards)
	if err != nil {
		return msgAutoAddDone{err: err}
	}
	_ = db.MarkAutoAddDone(database)
	return msgAutoAddDone{saved: saved}
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
		b.WriteString(labelStyle.Render(fmt.Sprintf("Total: %d cards", h.total)))
		b.WriteString("\n")
		if h.overdue > 0 {
			b.WriteString(labelStyle.Render(fmt.Sprintf("Due now: %d   Past-day overdue: %d", h.due, h.overdue)))
		} else {
			b.WriteString(labelStyle.Render(fmt.Sprintf("Due now: %d", h.due)))
		}
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("Today: %d / %d reviewed   Remaining: %d", h.reviewedToday, dailyReviewLimit, remaining)))
		if h.practiceToday.Runs > 0 {
			b.WriteString("\n")
			b.WriteString(labelStyle.Render(fmt.Sprintf("Standalone today: %d run(s), %d items, %d correct", h.practiceToday.Runs, h.practiceToday.Items, h.practiceToday.Correct)))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Today counts saved review updates. Standalone is tracked separately below."))
		b.WriteString("\n")
		check := func(done bool) string {
			if done {
				return "✓"
			}
			return " "
		}
		b.WriteString(labelStyle.Render(fmt.Sprintf("Session: Review [%s] Match [%s] Reverse [%s] Blank [%s]",
			check(h.session.ReviewDone), check(h.session.MatchDone), check(h.session.ReverseDone), check(h.session.BlankDone))))
		if h.resumeAvailable {
			b.WriteString("\n")
			b.WriteString(labelStyle.Render("Resume available: unfinished Daily Session detected"))
		}
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
			keyStyle.Render("[c]") + menuItemStyle.Render(" Compose"),
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
			keyStyle.Render("[a]") + menuItemStyle.Render(" Add card"),
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
	case homeStateConfigure:
		b.WriteString(subtitleStyle.Render("Configure"))
		b.WriteString("\n\n")
		enabledLabel := "Disabled"
		if h.cfgEnabled {
			enabledLabel = "Enabled"
		}
		focusMarker := func(i int) string {
			if h.cfgFocus == i {
				return "> "
			}
			return "  "
		}
		b.WriteString(focusMarker(0) + keyStyle.Render("[e]") + menuItemStyle.Render(fmt.Sprintf(" Auto-add: %s", enabledLabel)))
		b.WriteString("\n")
		b.WriteString(focusMarker(1) + inputLabelStyle.Render("Language: ") + h.cfgLangInput.View())
		b.WriteString("\n")
		b.WriteString(focusMarker(2) + inputLabelStyle.Render("Count:    ") + h.cfgCountInput.View())
		b.WriteString("\n")
		b.WriteString(focusMarker(3) + inputLabelStyle.Render("Profile:  ") + h.cfgProfileInput.View())
		b.WriteString("\n")
		b.WriteString(focusMarker(4) + inputLabelStyle.Render("Feedback: ") + h.cfgFeedbackLangInput.View())
		b.WriteString("\n")
		if h.cfgInlineErr != "" {
			b.WriteString("\n" + errorStyle.Render(h.cfgInlineErr))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[tab] next  [e] toggle  [enter] save  [esc] cancel"))
	default:
		if h.notifyFatigue {
			b.WriteString(warningStyle.Render("📊 [分析] 疲労カーブ分析のタイミングです（20セッション到達）"))
			b.WriteString("\n")
		}
		if h.notifyBenchmark {
			b.WriteString(warningStyle.Render("📊 [分析] Benchmark比較のタイミングです（初回から60日経過）"))
			b.WriteString("\n")
		}
		if h.notifyFatigue || h.notifyBenchmark {
			b.WriteString(helpStyle.Render("[1] 疲労カーブ通知をリセット  [2] Benchmark通知をリセット"))
			b.WriteString("\n\n")
		}
		menu := []string{
			keyStyle.Render("[d]") + menuItemStyle.Render(func() string {
				if h.resumeAvailable {
					return " Resume Daily Session"
				}
				return " Daily Session"
			}()),
			keyStyle.Render("[p]") + menuItemStyle.Render(" Practice"),
			keyStyle.Render("[b]") + menuItemStyle.Render(" Benchmark"),
			keyStyle.Render("[l]") + menuItemStyle.Render(" List cards"),
			keyStyle.Render("[s]") + menuItemStyle.Render(" Stats"),
			keyStyle.Render("[t]") + menuItemStyle.Render(" Tools"),
			keyStyle.Render("[c]") + menuItemStyle.Render(" Configure"),
			keyStyle.Render("[q]") + menuItemStyle.Render(" Quit"),
		}
		for _, item := range menu {
			b.WriteString("  " + item + "\n")
		}
		if h.autoAdding {
			b.WriteString("\n" + subtitleStyle.Render("Auto-adding cards..."))
		}
		b.WriteString("\n\n")
	}

	if h.importMsg != "" {
		b.WriteString("\n" + h.importMsg)
	}
	if h.statusMsg != "" {
		b.WriteString("\n" + subtitleStyle.Render("← "+h.statusMsg))
	}

	return b.String()
}
