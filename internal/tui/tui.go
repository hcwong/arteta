// Package tui implements Arteta's Bubble Tea homepage.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
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
	ModeConfirmRestart
	ModeFilePicker
)

// Model is the root Bubble Tea model.
type Model struct {
	Store      *store.Store
	Service    *service.Service
	DefaultCwd string

	items       []DisplayItem
	cursor      int
	mode        Mode
	create      CreateForm
	err         error
	width       int
	height      int
	events      <-chan fsnotify.Event
	pending     string
	preview     map[string]string // last-good capture per workflow name
	previewErrs map[string]int    // consecutive capture failures per name
	picker      filepicker.Model
}

// previewErrThreshold is how many consecutive capture failures we tolerate
// before surfacing the error in the persistent footer. Most failures are
// transient (race with KillSession, brief tmux server hiccup) and the
// last-good frame keeps the UI usable.
const previewErrThreshold = 5

func New(s *store.Store, svc *service.Service, defaultCwd string) Model {
	return Model{
		Store:       s,
		Service:     svc,
		DefaultCwd:  defaultCwd,
		mode:        ModeList,
		preview:     map[string]string{},
		previewErrs: map[string]int{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadWorkflowsCmd(m.Store, m.Service),
		startWatchCmd(m.Store.SessionsDir()),
		previewTickCmd(),
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

	case restartAllDoneMsg:
		return m, loadWorkflowsCmd(m.Store, m.Service)

	case previewTickMsg:
		return m, tea.Batch(m.captureSelectedCmd(), previewTickCmd())

	case previewMsg:
		if msg.err != nil {
			m.previewErrs[msg.name]++
			if m.previewErrs[msg.name] >= previewErrThreshold && m.err == nil {
				m.err = fmt.Errorf("preview capture for %q: %w", msg.name, msg.err)
			}
			return m, nil
		}
		m.preview[msg.name] = msg.content
		m.previewErrs[msg.name] = 0
		return m, nil

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
		case ModeConfirmRestart:
			return m.updateConfirmRestart(msg)
		case ModeHelp:
			m.mode = ModeList
			return m, nil
		case ModeFilePicker:
			return m.updateFilePicker(msg)
		}
	}

	// Route non-key msgs to the filepicker when active (e.g. the async readDirMsg).
	if m.mode == ModeFilePicker {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	prevCursor := m.cursor
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
	case "R":
		hasLive := false
		for _, it := range m.items {
			if !it.Dormant {
				hasLive = true
				break
			}
		}
		if hasLive {
			m.mode = ModeConfirmRestart
		}
	}
	if m.cursor != prevCursor {
		return m, m.captureSelectedCmd()
	}
	return m, nil
}

// captureSelectedCmd dispatches a CapturePane for the currently selected
// workflow, or nil if there's nothing capturable (empty list, dormant row,
// or workflow has no tmux session yet).
func (m Model) captureSelectedCmd() tea.Cmd {
	it := m.selected()
	if it == nil || it.Dormant {
		return nil
	}
	session := it.Workflow.TmuxSession
	if session == "" {
		return nil
	}
	return capturePaneCmd(m.Service, it.Workflow.Name, session)
}

func (m Model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd, submitted, cancelled := m.create.Update(msg)
	m.create = form
	if cancelled {
		m.mode = ModeList
		return m, nil
	}
	if m.create.OpenPicker {
		m.create.OpenPicker = false
		startDir := strings.TrimSpace(m.create.CwdInput.Value())
		if info, err := os.Stat(startDir); err != nil || !info.IsDir() {
			startDir = m.DefaultCwd
		}
		fp := filepicker.New()
		fp.CurrentDirectory = startDir
		fp.DirAllowed = true
		fp.FileAllowed = false
		fp.ShowHidden = false
		fp.AutoHeight = true
		m.picker = fp
		m.mode = ModeFilePicker
		return m, m.picker.Init()
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

func (m Model) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			// Cancel — must intercept before picker.Update since esc also navigates up.
			m.mode = ModeCreate
			m.create.Focus = 1
			m.create.CwdInput.Focus()
			return m, nil
		case " ":
			// Confirm current directory and return to form.
			m.create.CwdInput.SetValue(m.picker.CurrentDirectory)
			m.create.Focus = 1
			m.create.CwdInput.Focus()
			m.mode = ModeCreate
			return m, nil
		}
	}

	// All other keys (j/k navigate, l/enter open, h back) go to the picker.
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) updateConfirmRestart(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = ModeList
		return m, restartAllCmd(m.Service)
	case "n", "N", "esc", "q":
		m.mode = ModeList
	}
	return m, nil
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
	case ModeConfirmRestart:
		return center(m.viewConfirmRestart(), m.width, m.height)
	case ModeHelp:
		return center(m.viewHelp(), m.width, m.height)
	case ModeFilePicker:
		return center(m.viewFilePicker(), m.width, m.height)
	}
	return m.viewList()
}

func (m Model) viewFilePicker() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Choose directory"))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render(m.picker.CurrentDirectory))
	b.WriteString("\n\n")
	b.WriteString(m.picker.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[j/k] navigate   [l/⏎] open   [h] back   [space] select   [Esc] cancel"))
	return modalStyle.Render(b.String())
}

// previewWidthThreshold is the minimum terminal width at which we render
// the side-pane preview. Below this, the homepage falls back to a single
// centered column. Picked so list (~60 cols) + preview (~70 cols readable)
// + gutter fit comfortably.
const previewWidthThreshold = 140

// listColWidth is the fixed width of the workflow list when the side pane
// is showing. Rows never need more than this; flexing it would only steal
// space from the preview.
const listColWidth = 60

const topPad = 2

// sidePad reserves horizontal whitespace on each side of the homepage so
// content doesn't crash into the terminal edge. Applies in two-column mode
// and caps the preview width.
const sidePad = 4

func (m Model) viewList() string {
	body := m.renderListBody()
	footer := m.renderListFooter()

	// No size yet (e.g. tests or first frame): render plainly so substring
	// assertions still work and the user gets *something* on screen.
	if m.width == 0 || m.height == 0 {
		return body + "\n\n" + footer
	}

	if m.width < previewWidthThreshold {
		return m.viewListSingleCol(body, footer)
	}
	return m.viewListTwoCol(body, footer)
}

func (m Model) viewListSingleCol(body, footer string) string {
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

func (m Model) viewListTwoCol(body, footer string) string {
	footerBox := lipgloss.NewStyle().Width(m.width).Render(footer)
	footerCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footerBox)

	availableH := m.height - topPad - lipgloss.Height(footerCentered) - 1 // 1 for spacer
	if availableH < 5 {
		availableH = 5
	}

	listBox := lipgloss.NewStyle().Width(listColWidth).Render(body)
	previewW := m.width - listColWidth - 2 - 2*sidePad // 2 for gutter, sidePad on each edge
	previewBox := m.renderPreview(previewW, availableH)

	joined := lipgloss.JoinHorizontal(lipgloss.Top, listBox, "  ", previewBox)
	joinedCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, joined)

	spacer := m.height - topPad - lipgloss.Height(joinedCentered) - lipgloss.Height(footerCentered)
	if spacer < 1 {
		spacer = 1
	}

	return strings.Repeat("\n", topPad) + joinedCentered + strings.Repeat("\n", spacer) + footerCentered
}

// renderPreview produces the bordered side-pane content for the currently
// selected workflow, sized to fit (width × height) including the border.
func (m Model) renderPreview(width, height int) string {
	style := previewStyle.Width(width).Height(height)
	innerW := width - 4  // border (2) + horizontal padding (2)
	innerH := height - 2 // border top/bottom

	it := m.selected()
	if it == nil {
		return style.Render(centerText(dimStyle.Render("(no workflow selected)"), innerW, innerH))
	}
	if it.Dormant {
		msg := dormantStyle.Render("dormant — press ⏎ to revive")
		return style.Render(centerText(msg, innerW, innerH))
	}

	_, stateText := stateBadge(*it)
	title := titleStyle.Render(it.Workflow.Name) + " " + stateText
	body := m.preview[it.Workflow.Name]
	if body == "" {
		body = dimStyle.Render("(loading…)")
	}
	body = clipPreviewBody(body, innerW, innerH-2) // -2 for title + blank line

	return style.Render(title + "\n\n" + body)
}

// clipPreviewBody trims a captured pane string to the last `maxRows` rows
// and ANSI-truncates each row to `maxCols` printable runes.
func clipPreviewBody(s string, maxCols, maxRows int) string {
	if maxRows <= 0 || maxCols <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxRows {
		lines = lines[len(lines)-maxRows:]
	}
	for i, ln := range lines {
		lines[i] = truncateANSI(ln, maxCols)
	}
	return strings.Join(lines, "\n")
}

// centerText vertically and horizontally pads a (potentially multi-line)
// string into a width×height box.
func centerText(s string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, s)
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
	footer := helpStyle.Render("j/k move  ⏎ open  n new  D close  r refresh  R restart all  ? help  q quit")
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

func (m Model) viewConfirmRestart() string {
	liveCount := 0
	for _, it := range m.items {
		if !it.Dormant {
			liveCount++
		}
	}
	body := fmt.Sprintf(
		"Restart Claude in %d live workflow%s?\n\nThis kills and respawns pane 0 with --resume.\nOther panes are preserved.\n\n[y] yes  [n] no",
		liveCount, plural(liveCount),
	)
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
		"  R             restart all live workflows (Claude pane only)",
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
