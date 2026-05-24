package tui

import "github.com/charmbracelet/lipgloss"

// Earth tone palette — warm ochre, terracotta, sage, parchment, amber.
const (
	colorOchre      = lipgloss.Color("#C9A84C") // titles, modal borders
	colorParchment  = lipgloss.Color("#E8D5B7") // selected text
	colorTerracotta = lipgloss.Color("#C4663A") // cursor, awaiting input
	colorAmber      = lipgloss.Color("#D4A017") // running
	colorSage       = lipgloss.Color("#7A9E5E") // idle
	colorDust       = lipgloss.Color("#8B7355") // dim text, neutral borders
	colorAsh        = lipgloss.Color("#6B5C4A") // dormant
	colorRust       = lipgloss.Color("#B94040") // errors
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorOchre)
	helpStyle     = lipgloss.NewStyle().Faint(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(colorTerracotta)
	dimStyle      = lipgloss.NewStyle().Foreground(colorDust)
	errorStyle    = lipgloss.NewStyle().Foreground(colorRust).Bold(true)
	dormantStyle  = lipgloss.NewStyle().Foreground(colorAsh).Italic(true)
	selectedStyle = lipgloss.NewStyle().Foreground(colorParchment).Bold(true)
	modalStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOchre).
			Padding(1, 2)
	previewStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDust).
			Padding(0, 1)
	stateRunning   = lipgloss.NewStyle().Foreground(colorAmber)
	stateAwaiting  = lipgloss.NewStyle().Foreground(colorTerracotta).Bold(true)
	stateIdle      = lipgloss.NewStyle().Foreground(colorSage)
	stateDormantCl = lipgloss.NewStyle().Foreground(colorAsh)
)
