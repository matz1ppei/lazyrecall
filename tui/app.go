package tui

import (
	"database/sql"

	"github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
)

type screen int

const (
	screenHome screen = iota
	screenAdd
	screenReview
	screenReverseReview
	screenFetch
	screenFetchLang
	screenList
	screenStats
	screenMatch
	screenBlank
	screenSession
	screenCompose
	screenSetup // first-run onboarding, appended last to preserve iota order
)

// MsgGotoScreen is sent by sub-models to request a screen transition.
// Reason is an optional human-readable explanation shown on the home screen
// to help diagnose unexpected transitions (e.g. "no cards with translations").
type MsgGotoScreen struct {
	Target screen
	Reason string
}

type App struct {
	screen        screen
	home          HomeModel
	add           AddModel
	review        ReviewModel
	reverseReview ReverseInputModel
	fetch         FetchModel
	fetchLang     FetchLangModel
	list          ListModel
	stats         StatsModel
	match         MatchModel
	blank         BlankModel
	session       SessionModel
	compose       ComposeModel
	setup         SetupModel
	db            *sql.DB
	ai            ai.Client
	cfg           config.Config
	termWidth     int
}

func New(db *sql.DB, aiClient ai.Client, cfg config.Config) *App {
	return &App{
		screen:    screenHome,
		home:      NewHomeModel(db, aiClient, cfg),
		add:       NewAddModel(db, aiClient),
		review:    NewReviewModel(db),
		fetch:     NewFetchModel(db, aiClient),
		fetchLang: NewFetchLangModel(db, aiClient),
		list:      NewListModel(db, aiClient),
		stats:     NewStatsModel(db),
		db:        db,
		ai:        aiClient,
		cfg:       cfg,
	}
}

func (a *App) Init() tea.Cmd {
	return a.home.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MsgGotoScreen:
		a.screen = msg.Target
		switch msg.Target {
		case screenHome:
			if cfg, err := config.Load(); err == nil {
				a.cfg = cfg
			}
			a.home = NewHomeModel(a.db, a.ai, a.cfg)
			if msg.Reason != "" {
				a.home.statusMsg = msg.Reason
			}
			return a, a.home.Init()
		case screenAdd:
			a.add = NewAddModel(a.db, a.ai)
			return a, a.add.Init()
		case screenReview:
			a.review = NewReviewModel(a.db)
			return a, a.review.Init()
		case screenReverseReview:
			a.reverseReview = NewReverseInputModel(a.db)
			return a, a.reverseReview.Init()
		case screenFetch:
			a.fetch = NewFetchModel(a.db, a.ai)
			return a, a.fetch.Init()
		case screenFetchLang:
			a.fetchLang = NewFetchLangModel(a.db, a.ai)
			return a, a.fetchLang.Init()
		case screenList:
			a.list = NewListModel(a.db, a.ai)
			return a, a.list.Init()
		case screenStats:
			a.stats = NewStatsModel(a.db)
			return a, a.stats.Init()
		case screenMatch:
			a.match = NewMatchModel(a.db)
			return a, a.match.Init()
		case screenBlank:
			a.blank = NewBlankModel(a.db)
			return a, a.blank.Init()
		case screenCompose:
			if cfg, err := config.Load(); err == nil {
				a.cfg = cfg
			}
			a.compose = NewComposeModel(a.db, a.ai, a.termWidth, a.cfg.FeedbackLanguage)
			return a, a.compose.Init()
		case screenSession:
			a.session = NewSessionModel(a.db, a.ai)
			return a, a.session.Init()
		case screenSetup:
			a.setup = NewSetupModel(a.db, a.ai, a.cfg)
			return a, a.setup.Init()
		}
	case tea.WindowSizeMsg:
		a.termWidth = msg.Width
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenHome:
		var updated HomeModel
		m, c := a.home.Update(msg)
		updated = m.(HomeModel)
		a.home = updated
		cmd = c
	case screenAdd:
		var updated AddModel
		m, c := a.add.Update(msg)
		updated = m.(AddModel)
		a.add = updated
		cmd = c
	case screenReview:
		var updated ReviewModel
		m, c := a.review.Update(msg)
		updated = m.(ReviewModel)
		a.review = updated
		cmd = c
	case screenReverseReview:
		var updated ReverseInputModel
		m, c := a.reverseReview.Update(msg)
		updated = m.(ReverseInputModel)
		a.reverseReview = updated
		cmd = c
	case screenFetch:
		var updated FetchModel
		m, c := a.fetch.Update(msg)
		updated = m.(FetchModel)
		a.fetch = updated
		cmd = c
	case screenFetchLang:
		var updated FetchLangModel
		m, c := a.fetchLang.Update(msg)
		updated = m.(FetchLangModel)
		a.fetchLang = updated
		cmd = c
	case screenList:
		var updated ListModel
		m, c := a.list.Update(msg)
		updated = m.(ListModel)
		a.list = updated
		cmd = c
	case screenStats:
		var updated StatsModel
		m, c := a.stats.Update(msg)
		updated = m.(StatsModel)
		a.stats = updated
		cmd = c
	case screenMatch:
		var updated MatchModel
		m, c := a.match.Update(msg)
		updated = m.(MatchModel)
		a.match = updated
		cmd = c
	case screenBlank:
		var updated BlankModel
		m, c := a.blank.Update(msg)
		updated = m.(BlankModel)
		a.blank = updated
		cmd = c
	case screenSession:
		var updated SessionModel
		m, c := a.session.Update(msg)
		updated = m.(SessionModel)
		a.session = updated
		cmd = c
	case screenCompose:
		var updated ComposeModel
		m, c := a.compose.Update(msg)
		updated = m.(ComposeModel)
		a.compose = updated
		cmd = c
	case screenSetup:
		var updated SetupModel
		m, c := a.setup.Update(msg)
		updated = m.(SetupModel)
		a.setup = updated
		cmd = c
	}
	return a, cmd
}

func (a *App) View() string {
	switch a.screen {
	case screenHome:
		return a.home.View()
	case screenAdd:
		return a.add.View()
	case screenReview:
		return a.review.View()
	case screenReverseReview:
		return a.reverseReview.View()
	case screenFetch:
		return a.fetch.View()
	case screenFetchLang:
		return a.fetchLang.View()
	case screenList:
		return a.list.View()
	case screenStats:
		return a.stats.View()
	case screenMatch:
		return a.match.View()
	case screenBlank:
		return a.blank.View()
	case screenSession:
		return a.session.View()
	case screenCompose:
		return a.compose.View()
	case screenSetup:
		return a.setup.View()
	}
	return ""
}
