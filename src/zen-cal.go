package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Check for tooltip mode flag
	if CheckTooltipFlag() {
		return
	}

	// Run the interactive TUI
	p := tea.NewProgram(newCalendarPage())
	if _, err := p.Run(); err != nil {
		panic(err)
	}
}