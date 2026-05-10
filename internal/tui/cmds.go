package tui

import (
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/hcwong/arteta/internal/reconcile"
	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/workflow"
)

// Messages flowing into Update.

type workflowsLoadedMsg struct{ items []DisplayItem }
type statusChangedMsg struct{ workflowName string }
type fsnotifyReadyMsg struct{ events <-chan fsnotify.Event }
type errMsg struct{ err error }
type createDoneMsg struct{ name string }
type closeDoneMsg struct{ name string }
type reviveDoneMsg struct{ name string }
type previewMsg struct {
	name    string
	content string
	err     error
}
type previewTickMsg struct{}
type restartAllDoneMsg struct{ count int }

func (e errMsg) Error() string { return e.err.Error() }

// loadWorkflowsCmd reads all workflows + their statuses, runs reconcile,
// and returns a workflowsLoadedMsg.
func loadWorkflowsCmd(s *store.Store, svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		ws, err := s.LoadAllWorkflows()
		if err != nil {
			return errMsg{err}
		}
		r, err := reconcile.Reconcile(ws, svc.Tmux)
		if err != nil {
			return errMsg{err}
		}
		liveByName := map[string]bool{}
		for _, w := range r.Live {
			liveByName[w.Name] = true
		}
		items := make([]DisplayItem, 0, len(ws))
		for _, w := range ws {
			st, _ := s.LoadStatus(w.Name)
			items = append(items, DisplayItem{
				Workflow: w,
				Status:   st,
				Dormant:  !liveByName[w.Name],
			})
		}
		return workflowsLoadedMsg{items: items}
	}
}

// startWatchCmd kicks off the fsnotify watcher for the sessions dir and
// returns the events channel via a fsnotifyReadyMsg.
func startWatchCmd(sessionsDir string) tea.Cmd {
	return func() tea.Msg {
		if err := ensureDir(sessionsDir); err != nil {
			return errMsg{err}
		}
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return errMsg{err}
		}
		if err := watcher.Add(sessionsDir); err != nil {
			return errMsg{err}
		}
		return fsnotifyReadyMsg{events: watcher.Events}
	}
}

// waitForStatusCmd blocks on the next fsnotify event and translates it to
// a statusChangedMsg. Re-issued by Update after each event.
func waitForStatusCmd(events <-chan fsnotify.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		// Filter out tmpfile churn from the atomic rename pattern.
		base := filepath.Base(ev.Name)
		if len(base) > 0 && base[0] == '.' {
			return statusChangedMsg{workflowName: ""}
		}
		// Strip ".json".
		name := base
		if ext := filepath.Ext(name); ext == ".json" {
			name = name[:len(name)-len(ext)]
		}
		return statusChangedMsg{workflowName: name}
	}
}

func createCmd(svc *service.Service, opts service.CreateOpts) tea.Cmd {
	return func() tea.Msg {
		if _, err := svc.Create(opts); err != nil {
			return errMsg{err}
		}
		return createDoneMsg{name: opts.Name}
	}
}

func closeCmd(svc *service.Service, name string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Close(name); err != nil {
			return errMsg{err}
		}
		return closeDoneMsg{name: name}
	}
}

func openCmd(svc *service.Service, name string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Open(name); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func reviveCmd(svc *service.Service, name string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Revive(name); err != nil {
			return errMsg{err}
		}
		return reviveDoneMsg{name: name}
	}
}

func restartAllCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		n, err := svc.RestartAll()
		if err != nil {
			return errMsg{err}
		}
		return restartAllDoneMsg{count: n}
	}
}

// capturePaneCmd snapshots pane 0 of a workflow's tmux session. Errors
// ride on previewMsg.err so the model can decide whether to surface them
// (instead of polluting the persistent m.err channel on every transient
// race with KillSession).
func capturePaneCmd(svc *service.Service, name, sessionName string) tea.Cmd {
	return func() tea.Msg {
		out, err := svc.Tmux.CapturePane(sessionName, 0)
		return previewMsg{name: name, content: out, err: err}
	}
}

// captureUncachedCmd fires a CapturePane for every live workflow whose preview
// cache is currently empty. Called after loading workflows so that navigating
// to any workflow for the first time shows real content instead of "(loading…)".
func captureUncachedCmd(svc *service.Service, items []DisplayItem, cached map[string]string) tea.Cmd {
	var cmds []tea.Cmd
	for _, it := range items {
		if it.Dormant || it.Workflow.TmuxSession == "" {
			continue
		}
		if cached[it.Workflow.Name] == "" {
			cmds = append(cmds, capturePaneCmd(svc, it.Workflow.Name, it.Workflow.TmuxSession))
		}
	}
	return tea.Batch(cmds...)
}

// previewTickCmd schedules the next preview refresh tick.
func previewTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return previewTickMsg{}
	})
}

// DisplayItem is the per-row data the homepage renders.
type DisplayItem struct {
	Workflow workflow.Workflow
	Status   workflow.Status
	Dormant  bool
}

func ensureDir(p string) error {
	return mkdirAll(p)
}

// indirected so tests can stub it; real impl uses os.MkdirAll.
var mkdirAll = func(p string) error {
	return defaultMkdirAll(p)
}
