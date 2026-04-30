package main

import "github.com/charmbracelet/lipgloss"

var (
	// Dracula Theme Palette
	ColorPrimary   = lipgloss.Color("#bd93f9") // Purple
	ColorSecondary = lipgloss.Color("#8be9fd") // Cyan
	ColorSuccess   = lipgloss.Color("#50fa7b") // Green
	ColorError     = lipgloss.Color("#ff5555") // Red
	ColorText      = lipgloss.Color("#f8f8f2") // White
	ColorSubtle    = lipgloss.Color("#6272a4") // Grey

	// Typography Styles
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	StyleHighlight = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	StyleSuccess   = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleError     = lipgloss.NewStyle().Foreground(ColorError)
	StyleSubtle    = lipgloss.NewStyle().Foreground(ColorSubtle)

	// Layout Boxes
	StyleMainBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			Margin(1, 1)

	StyleLogBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorSubtle).
			Padding(0, 1).
			MarginTop(1)
)
