package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/hcwong/arteta/internal/ui"
)

const (
	colorOchre      = ui.ColorOchre
	colorParchment  = ui.ColorParchment
	colorTerracotta = ui.ColorTerracotta
	colorAmber      = ui.ColorAmber
	colorSage       = ui.ColorSage
	colorDust       = ui.ColorDust
	colorAsh        = ui.ColorAsh
	colorRust       = ui.ColorRust
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
