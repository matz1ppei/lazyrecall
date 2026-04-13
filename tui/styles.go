package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	inputLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	cardFrontStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")).
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			MarginBottom(1)

	cardBackStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			MarginBottom(1)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Italic(true)

	ratingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("247"))

	matchCursorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86"))

	matchSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214"))

	matchMatchedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Strikethrough(true)

	matchWrongStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))
)
