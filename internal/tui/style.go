package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
	dormantStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Bold(true)
	modalStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2)
	stateRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")) // yellow
	stateAwaiting  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
	stateIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")) // green
	stateDormantCl = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
)
