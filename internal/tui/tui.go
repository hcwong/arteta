// Package tui implements Arteta's Bubble Tea homepage.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/workflow"
)

// Mode is the TUI's modal state.
type Mode int

const (
	ModeList Mode = iota
	ModeCreate
	ModeConfirmDelete
	ModeHelp
)

// Model is the root Bubble Tea model.
type Model struct {
	Store      *store.Store
	Service    *service.Service
	DefaultCwd string

	items   []DisplayItem
	cursor  int
	mode    Mode
	create  CreateForm
	err     error
	width   int
	height  int
	events  <-chan fsnotify.Event
	pending string
}

func New(s *store.Store, svc *service.Service, defaultCwd string) Model {
	return Model{
		Store:      s,
		Service:    svc,
		DefaultCwd: defaultCwd,
		mode:       ModeList,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadWorkflowsCmd(m.Store, m.Service),
		startWatchCmd(m.Store.SessionsDir()),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case workflowsLoadedMsg:
		m.items = msg.items
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}
		return m, nil

	case fsnotifyReadyMsg:
		m.events = msg.events
		return m, waitForStatusCmd(m.events)

	case statusChangedMsg:
		// Re-load to pick up the change, then wait for the next event.
		return m, tea.Batch(
			loadWorkflowsCmd(m.Store, m.Service),
			waitForStatusCmd(m.events),
		)

	case createDoneMsg:
		m.mode = ModeList
		return m, loadWorkflowsCmd(m.Store, m.Service)

	case closeDoneMsg:
		m.mode = ModeList
		return m, loadWorkflowsCmd(m.Store, m.Service)

	case reviveDoneMsg:
		return m, loadWorkflowsCmd(m.Store, m.Service)

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		// Esc clears errors.
		if m.err != nil && msg.String() != "ctrl+c" {
			m.err = nil
		}
		switch m.mode {
		case ModeList:
			return m.updateList(msg)
		case ModeCreate:
			return m.updateCreate(msg)
		case ModeConfirmDelete:
			return m.updateConfirm(msg)
		case ModeHelp:
			m.mode = ModeList
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(0, len(m.items)-1)
	case "n":
		m.create = newCreateForm(m.DefaultCwd)
		m.mode = ModeCreate
	case "?":
		m.mode = ModeHelp
	case "r":
		return m, loadWorkflowsCmd(m.Store, m.Service)
	case "enter":
		if it := m.selected(); it != nil {
			if it.Dormant {
				return m, reviveCmd(m.Service, it.Workflow.Name)
			}
			return m, openCmd(m.Service, it.Workflow.Name)
		}
	case "D":
		if it := m.selected(); it != nil {
			m.pending = it.Workflow.Name
			m.mode = ModeConfirmDelete
		}
	}
	return m, nil
}

func (m Model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd, submitted, cancelled := m.create.Update(msg)
	m.create = form
	if cancelled {
		m.mode = ModeList
		return m, nil
	}
	if submitted {
		opts := service.CreateOpts{
			Name:   strings.TrimSpace(m.create.NameInput.Value()),
			Cwd:    strings.TrimSpace(m.create.CwdInput.Value()),
			Layout: m.create.Layout,
		}
		return m, createCmd(m.Service, opts)
	}
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		name := m.pending
		m.pending = ""
		m.mode = ModeList
		return m, closeCmd(m.Service, name)
	case "n", "N", "esc", "q":
		m.pending = ""
		m.mode = ModeList
	}
	return m, nil
}

func (m Model) selected() *DisplayItem {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.cursor]
}

func (m Model) View() string {
	switch m.mode {
	case ModeCreate:
		return center(m.create.View(), m.width, m.height)
	case ModeConfirmDelete:
		return center(m.viewConfirm(), m.width, m.height)
	case ModeHelp:
		return center(m.viewHelp(), m.width, m.height)
	}
	return m.viewList()
}

func (m Model) viewList() string {
	body := m.renderListBody()
	footer := m.renderListFooter()

	// No size yet (e.g. tests or first frame): render plainly so substring
	// assertions still work and the user gets *something* on screen.
	if m.width == 0 || m.height == 0 {
		return body + "\n\n" + footer
	}

	const topPad = 2
	contentW := contentWidth(m.width)
	bodyBox := lipgloss.NewStyle().Width(contentW).Render(body)
	footerBox := lipgloss.NewStyle().Width(contentW).Render(footer)

	bodyCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bodyBox)
	footerCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footerBox)

	spacer := m.height - topPad - lipgloss.Height(bodyCentered) - lipgloss.Height(footerCentered)
	if spacer < 1 {
		spacer = 1
	}

	return strings.Repeat("\n", topPad) + bodyCentered + strings.Repeat("\n", spacer) + footerCentered
}

func (m Model) renderListBody() string {
	var b strings.Builder
	header := titleStyle.Render(fmt.Sprintf("Arteta — %d workflow%s", len(m.items), plural(len(m.items))))
	b.WriteString(header)
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("No workflows yet. Press n to create one."))
		b.WriteString("\n")
	}

	for i, it := range m.items {
		row := formatRow(it, i == m.cursor, contentWidth(m.width))
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderListFooter() string {
	footer := helpStyle.Render("j/k move  ⏎ open  n new  D close  r refresh  ? help  q quit")
	if m.err != nil {
		footer = errorStyle.Render("error: "+m.err.Error()) + "\n" + footer
	}
	return footer
}

// contentWidth caps the homepage content to a comfortable line length so wide
// terminals don't stretch rows across the screen. Keeps a small horizontal
// margin around the content.
func contentWidth(termW int) int {
	if termW <= 0 {
		return 80
	}
	w := termW - 4
	if w > 120 {
		w = 120
	}
	if w < 40 {
		w = termW
	}
	return w
}

func formatRow(it DisplayItem, selected bool, width int) string {
	cursor := "  "
	if selected {
		cursor = cursorStyle.Render("▶ ")
	}
	dot, stateText := stateBadge(it)
	name := it.Workflow.Name
	if selected {
		name = selectedStyle.Render(name)
	}
	msg := it.Status.LastMessage
	if it.Dormant {
		msg = "[dormant — ⏎ to revive]"
	}
	maxMsg := width - 60
	if maxMsg < 20 {
		maxMsg = 20
	}
	msg = truncate(msg, maxMsg)

	return fmt.Sprintf("%s%s %s %s   %s",
		cursor,
		dot,
		padRight(name, 24),
		padRight(stateText, 16),
		dimStyle.Render(msg),
	)
}

func stateBadge(it DisplayItem) (dot string, text string) {
	if it.Dormant {
		return stateDormantCl.Render("○"), stateDormantCl.Render("dormant")
	}
	switch it.Status.State() {
	case workflow.StateRunning:
		return stateRunning.Render("●"), stateRunning.Render("running")
	case workflow.StateAwaitingInput:
		return stateAwaiting.Render("◐"), stateAwaiting.Render("awaiting input")
	case workflow.StateIdle:
		return stateIdle.Render("○"), stateIdle.Render("idle")
	}
	return dimStyle.Render("·"), dimStyle.Render("—")
}

func (m Model) viewConfirm() string {
	body := fmt.Sprintf("Close workflow %q?\n\nThis kills its tmux session and iTerm tab.\n\n[y] yes  [n] no", m.pending)
	return modalStyle.Render(body)
}

func (m Model) viewHelp() string {
	body := strings.Join([]string{
		titleStyle.Render("Arteta keybindings"),
		"",
		"  j / k / ↓ ↑   move selection",
		"  g / G         jump to top / bottom",
		"  ⏎             open selected workflow (or revive if dormant)",
		"  n             new workflow",
		"  D             close workflow (with confirm)",
		"  r             refresh",
		"  ?             this help",
		"  q             quit Arteta (workflows keep running)",
		"",
		helpStyle.Render("Press any key to dismiss."),
	}, "\n")
	return modalStyle.Render(body)
}

// Run starts the Bubble Tea program. Blocks until the user quits.
func Run(s *store.Store, svc *service.Service, defaultCwd string) error {
	p := tea.NewProgram(New(s, svc, defaultCwd), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func padRight(s string, n int) string {
	if lipgloss.Width(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-lipgloss.Width(s))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
