package sidebar

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/hcwong/arteta/internal/reconcile"
	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/ui"
	"github.com/hcwong/arteta/internal/workflow"
)

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(ui.ColorTerracotta)
	dimStyle     = lipgloss.NewStyle().Foreground(ui.ColorDust)
	dotRunning   = lipgloss.NewStyle().Foreground(ui.ColorAmber)
	dotAwaiting  = lipgloss.NewStyle().Foreground(ui.ColorTerracotta).Bold(true)
	dotIdle      = lipgloss.NewStyle().Foreground(ui.ColorSage)
	dotDormant   = lipgloss.NewStyle().Foreground(ui.ColorAsh)
	currentStyle = lipgloss.NewStyle().Foreground(ui.ColorParchment).Bold(true)
)

type item struct {
	Name    string
	State   workflow.State
	Dormant bool
}

type Model struct {
	store           *store.Store
	svc             *service.Service
	tmuxClient      tmux.Client
	currentWorkflow string
	items           []item
	cursor          int
	events          <-chan fsnotify.Event
	width           int
	height          int
}

func New(st *store.Store, svc *service.Service, tmuxClient tmux.Client, currentWorkflow string) Model {
	return Model{
		store:           st,
		svc:             svc,
		tmuxClient:      tmuxClient,
		currentWorkflow: currentWorkflow,
	}
}

type itemsLoadedMsg struct{ items []item }
type fsReadyMsg struct{ events <-chan fsnotify.Event }
type statusChangedMsg struct{}
type refreshTickMsg struct{}
type errMsg struct{ err error }

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadItemsCmd(m.store, m.tmuxClient),
		startWatchCmd(m.store),
		refreshTickCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case itemsLoadedMsg:
		oldName := ""
		if m.cursor >= 0 && m.cursor < len(m.items) {
			oldName = m.items[m.cursor].Name
		}
		m.items = msg.items
		if oldName != "" {
			for i, it := range m.items {
				if it.Name == oldName {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}

	case fsReadyMsg:
		m.events = msg.events
		return m, waitEventCmd(m.events)

	case statusChangedMsg:
		return m, tea.Batch(
			loadItemsCmd(m.store, m.tmuxClient),
			waitEventCmd(m.events),
		)

	case refreshTickMsg:
		return m, tea.Batch(
			loadItemsCmd(m.store, m.tmuxClient),
			refreshTickCmd(),
		)

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				name := m.items[m.cursor].Name
				return m, openCmd(m.svc, name)
			}
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.items) == 0 {
		return dimStyle.Render("(no workflows)")
	}

	maxNameW := m.width - 8 // dot(2) + cursor(2) + state(3) + space(1)
	if maxNameW < 4 {
		maxNameW = 4
	}

	var b strings.Builder
	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("▶ ")
		}
		isCurrent := it.Name == m.currentWorkflow

		dot, stateAbbr := badge(it)
		name := truncate(it.Name, maxNameW)
		if isCurrent {
			name = currentStyle.Render(name)
		}

		b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, dot, name, dimStyle.Render(stateAbbr)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func badge(it item) (dot string, abbr string) {
	if it.Dormant {
		return dotDormant.Render("○"), "dor"
	}
	switch it.State {
	case workflow.StateRunning:
		return dotRunning.Render("●"), "run"
	case workflow.StateAwaitingInput:
		return dotAwaiting.Render("◐"), "inp"
	case workflow.StateIdle:
		return dotIdle.Render("○"), "idl"
	}
	return dimStyle.Render("·"), " — "
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func loadItemsCmd(st *store.Store, tc tmux.Client) tea.Cmd {
	return func() tea.Msg {
		ws, err := st.LoadAllWorkflows()
		if err != nil {
			return errMsg{err}
		}
		r, err := reconcile.Reconcile(ws, tc)
		if err != nil {
			return errMsg{err}
		}
		liveSet := map[string]bool{}
		for _, w := range r.Live {
			liveSet[w.Name] = true
		}

		items := make([]item, 0, len(ws))
		for _, w := range ws {
			st, _ := st.LoadStatus(w.Name)
			items = append(items, item{
				Name:    w.Name,
				State:   st.State(),
				Dormant: !liveSet[w.Name],
			})
		}
		return itemsLoadedMsg{items: items}
	}
}

func startWatchCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return errMsg{err}
		}
		_ = watcher.Add(st.SessionsDir())
		_ = watcher.Add(st.WorkflowsDir())
		return fsReadyMsg{events: watcher.Events}
	}
}

func waitEventCmd(events <-chan fsnotify.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		base := filepath.Base(ev.Name)
		if len(base) > 0 && base[0] == '.' {
			return statusChangedMsg{}
		}
		return statusChangedMsg{}
	}
}

func openCmd(svc *service.Service, name string) tea.Cmd {
	return func() tea.Msg {
		_ = svc.Open(name)
		return nil
	}
}

func refreshTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

func Run(st *store.Store, svc *service.Service, tmuxClient tmux.Client, currentWorkflow string) error {
	p := tea.NewProgram(New(st, svc, tmuxClient, currentWorkflow))
	_, err := p.Run()
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
