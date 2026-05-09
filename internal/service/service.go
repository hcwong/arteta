// Package service is the choreography layer between the TUI/CLI and the
// store/tmux/terminal adapters. It owns the high-level operations
// (Create, Close, Open, Revive) so both the Bubble Tea TUI (via tea.Cmd)
// and the CLI subcommands can share a single implementation.
package service

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

// Service ties the adapters together for high-level workflow operations.
type Service struct {
	Store *store.Store
	Tmux  tmux.Client
	Term  terminal.Adapter
	Now   func() time.Time
	// SocketName is the tmux socket Arteta uses; defaults to tmux.DefaultSocket.
	SocketName string
}

// CreateOpts is the user input collected by the new-workflow modal.
type CreateOpts struct {
	Name      string
	Cwd       string
	Layout    workflow.Layout
	GitBranch string
}

// Create allocates a tmux session with the requested layout, opens an
// iTerm tab attached to it, persists the workflow, and returns it.
//
// On failure, Create attempts to roll back partial state: tmux session
// is killed if subsequent steps fail, and the iTerm tab (if opened) is
// closed. It does not, however, undo a successful persist if the only
// thing that fails is the iTerm tab close — at that point the user has
// a usable tmux session and can recover by re-opening manually.
func (s *Service) Create(opts CreateOpts) (workflow.Workflow, error) {
	if err := workflow.ValidateName(opts.Name); err != nil {
		return workflow.Workflow{}, err
	}
	if !opts.Layout.Valid() {
		return workflow.Workflow{}, fmt.Errorf("invalid layout %q", opts.Layout)
	}
	if existing, err := s.Store.LoadWorkflow(opts.Name); err == nil {
		return workflow.Workflow{}, fmt.Errorf("workflow %q already exists (cwd=%s)", opts.Name, existing.Cwd)
	}

	sessionName := workflow.TmuxSessionName(opts.Name)
	claudeCmd := "claude"
	env := map[string]string{"ARTETA_WORKFLOW": opts.Name}

	// Step 1: allocate tmux session with the requested layout.
	if err := tmux.BuildLayout(tmux.BuildOpts{
		Client:    s.Tmux,
		Name:      sessionName,
		Cwd:       opts.Cwd,
		Layout:    opts.Layout,
		ClaudeCmd: claudeCmd,
		Env:       env,
	}); err != nil {
		return workflow.Workflow{}, fmt.Errorf("build tmux layout: %w", err)
	}

	// Step 2: open iTerm tab attached to the session.
	attach := s.attachCmd(sessionName)
	tab, err := s.Term.OpenTab(terminal.OpenOpts{Title: opts.Name, Command: attach})
	if err != nil {
		_ = s.Tmux.KillSession(sessionName)
		return workflow.Workflow{}, fmt.Errorf("open iterm tab: %w", err)
	}

	// Step 3: persist workflow.
	w := workflow.Workflow{
		Name:        opts.Name,
		Cwd:         opts.Cwd,
		TmuxSession: sessionName,
		GitBranch:   opts.GitBranch,
		Layout:      opts.Layout,
		ITermTab:    &workflow.ITermTab{WindowID: tab.WindowID, TabID: tab.TabID},
		CreatedAt:   s.now(),
	}
	if err := s.Store.SaveWorkflow(w); err != nil {
		_ = s.Term.CloseTab(tab)
		_ = s.Tmux.KillSession(sessionName)
		return workflow.Workflow{}, fmt.Errorf("save workflow: %w", err)
	}
	return w, nil
}

// Open focuses the workflow's iTerm tab if it still exists, otherwise
// opens a fresh tab attached to the same tmux session and updates the
// stored TabHandle.
func (s *Service) Open(name string) error {
	w, err := s.Store.LoadWorkflow(name)
	if err != nil {
		return err
	}
	if w.ITermTab != nil {
		h := terminal.TabHandle{WindowID: w.ITermTab.WindowID, TabID: w.ITermTab.TabID}
		ok, _ := s.Term.TabExists(h)
		if ok {
			return s.Term.FocusTab(h)
		}
	}
	tab, err := s.Term.OpenTab(terminal.OpenOpts{
		Title:   w.Name,
		Command: s.attachCmd(w.TmuxSession),
	})
	if err != nil {
		return fmt.Errorf("open iterm tab: %w", err)
	}
	w.ITermTab = &workflow.ITermTab{WindowID: tab.WindowID, TabID: tab.TabID}
	return s.Store.SaveWorkflow(w)
}

// Close kills the tmux session, closes the iTerm tab (if any), and
// removes the workflow + status from the store.
func (s *Service) Close(name string) error {
	w, err := s.Store.LoadWorkflow(name)
	if err != nil {
		return err
	}
	if has, _ := s.Tmux.HasSession(w.TmuxSession); has {
		if err := s.Tmux.KillSession(w.TmuxSession); err != nil {
			return fmt.Errorf("kill tmux session: %w", err)
		}
	}
	if w.ITermTab != nil {
		_ = s.Term.CloseTab(terminal.TabHandle{WindowID: w.ITermTab.WindowID, TabID: w.ITermTab.TabID})
	}
	return s.Store.DeleteWorkflow(name)
}

// Revive restarts a dormant workflow's tmux session and reopens its iTerm
// tab. If a session_id is known, Claude is started with --resume.
func (s *Service) Revive(name string) error {
	w, err := s.Store.LoadWorkflow(name)
	if err != nil {
		return err
	}
	claudeCmd := "claude"
	if w.ClaudeSessionID != "" {
		claudeCmd = "claude --resume " + w.ClaudeSessionID
	}
	env := map[string]string{"ARTETA_WORKFLOW": w.Name}
	if err := tmux.BuildLayout(tmux.BuildOpts{
		Client:    s.Tmux,
		Name:      w.TmuxSession,
		Cwd:       w.Cwd,
		Layout:    w.Layout,
		ClaudeCmd: claudeCmd,
		Env:       env,
	}); err != nil {
		return fmt.Errorf("build tmux layout: %w", err)
	}
	tab, err := s.Term.OpenTab(terminal.OpenOpts{
		Title:   w.Name,
		Command: s.attachCmd(w.TmuxSession),
	})
	if err != nil {
		_ = s.Tmux.KillSession(w.TmuxSession)
		return fmt.Errorf("open iterm tab: %w", err)
	}
	w.ITermTab = &workflow.ITermTab{WindowID: tab.WindowID, TabID: tab.TabID}
	return s.Store.SaveWorkflow(w)
}

// attachCmd returns the shell command that the new iTerm tab will run to
// attach to the workflow's tmux session. iTerm execs this command directly
// (not via the user's login shell), so we resolve tmux to an absolute path
// up-front — otherwise iTerm's PATH may miss /usr/local/bin or
// /opt/homebrew/bin and the tab fails with "no such program tmux".
func (s *Service) attachCmd(sessionName string) string {
	socket := s.SocketName
	if socket == "" {
		socket = tmux.DefaultSocket
	}
	bin := "tmux"
	if p, err := exec.LookPath("tmux"); err == nil {
		bin = p
	}
	return bin + " -L " + socket + " attach -t " + sessionName
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
