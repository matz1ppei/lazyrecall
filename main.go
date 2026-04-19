package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/slack"
	"github.com/ippei/lazyrecall/tui"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) > 1 && os.Args[1] == "notify" {
		runNotify()
		return
	}

	database, err := db.Open("lazyrecall.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	aiClient, err := ai.NewClient(cfg.UserProfile)
	if err != nil {
		log.Fatalf("init ai: %v", err)
	}

	app := tui.New(database, aiClient, cfg)
	if _, err := tea.NewProgram(app).Run(); err != nil {
		log.Fatalf("run tui: %v", err)
	}
}

func runNotify() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("notify: load config: %v", err)
	}
	if cfg.Notify.WebhookURL == "" {
		log.Println("notify: webhook URL not configured in ~/.config/lazyrecall/config.json")
		return
	}

	database, err := db.Open("lazyrecall.db")
	if err != nil {
		log.Fatalf("notify: open db: %v", err)
	}
	defer database.Close()

	session, err := db.GetTodaySession(database)
	if err != nil {
		log.Fatalf("notify: get session: %v", err)
	}
	if session.ReviewDone {
		return // 今日の Review 完了済み → 通知しない
	}

	if err := slack.Send(cfg.Notify.WebhookURL, "📚 今日の lazyrecall、まだやってないですよ！"); err != nil {
		log.Fatalf("notify: send: %v", err)
	}
}
