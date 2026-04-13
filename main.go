package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/tui"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	database, err := db.Open("lazyrecall.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	aiClient, err := ai.NewClient()
	if err != nil {
		log.Fatalf("init ai: %v", err)
	}

	app := tui.New(database, aiClient)
	if _, err := tea.NewProgram(app).Run(); err != nil {
		log.Fatalf("run tui: %v", err)
	}
}
