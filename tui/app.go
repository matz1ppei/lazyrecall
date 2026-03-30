package tui

import (
	"database/sql"

	"github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/ai"
)

type screen int

const (
	screenHome screen = iota
	screenAdd
	screenReview
	screenFetch
)

// MsgGotoScreen is sent by sub-models to request a screen transition.
type MsgGotoScreen struct {
	Target screen
}

type App struct {
	screen screen
	home   HomeModel
	add    AddModel
	review ReviewModel
	fetch  FetchModel
	db     *sql.DB
	ai     ai.Client
}

func New(db *sql.DB, aiClient ai.Client) *App {
	return &App{
		screen: screenHome,
		home:   NewHomeModel(db, aiClient),
		add:    NewAddModel(db, aiClient),
		review: NewReviewModel(db),
		fetch:  NewFetchModel(db, aiClient),
		db:     db,
		ai:     aiClient,
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
			a.home = NewHomeModel(a.db, a.ai)
			return a, a.home.Init()
		case screenAdd:
			a.add = NewAddModel(a.db, a.ai)
			return a, a.add.Init()
		case screenReview:
			a.review = NewReviewModel(a.db)
			return a, a.review.Init()
		case screenFetch:
			a.fetch = NewFetchModel(a.db, a.ai)
			return a, a.fetch.Init()
		}
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
	case screenFetch:
		var updated FetchModel
		m, c := a.fetch.Update(msg)
		updated = m.(FetchModel)
		a.fetch = updated
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
	case screenFetch:
		return a.fetch.View()
	}
	return ""
}
