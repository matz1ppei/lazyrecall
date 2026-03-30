package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/poc-anki-claude/ai"
	"github.com/ippei/poc-anki-claude/db"
	"github.com/ippei/poc-anki-claude/tui"
)

func main() {
	database, err := db.Open("anki.db")
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
