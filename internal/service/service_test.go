package service

import (
	"strings"
	"testing"
	"time"

	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

func newTestService(t *testing.T) (*Service, *tmux.Fake, *terminal.Fake, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir())
	tx := tmux.NewFake()
	tm := terminal.NewFake()
	now := time.Date(2026, 5, 9, 17, 0, 0, 0, time.UTC)
	s := &Service{
		Store: st,
		Tmux:  tx,
		Term:  tm,
		Now:   func() time.Time { return now },
	}
	return s, tx, tm, st
}

func TestCreate_Quad_BuildsAllPanesAndPersists(t *testing.T) {
	s, tx, tm, st := newTestService(t)
	w, err := s.Create(CreateOpts{
		Name:   "auth-refactor",
		Cwd:    "/Users/josh/repo",
		Layout: workflow.LayoutQuad,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.Name != "auth-refactor" || w.TmuxSession != "arteta-auth-refactor" {
		t.Errorf("workflow fields wrong: %+v", w)
	}

	sess := tx.Sessions()["arteta-auth-refactor"]
	if sess == nil {
		t.Fatal("tmux session not created")
	}
	if len(sess.Panes) != 4 {
		t.Errorf("expected 4 panes, got %d", len(sess.Panes))
	}
	if sess.Panes[0].Env["ARTETA_WORKFLOW"] != "auth-refactor" {
		t.Errorf("ARTETA_WORKFLOW env not set on claude pane: %+v", sess.Panes[0].Env)
	}

	tabs := tm.Tabs()
	if len(tabs) != 1 || tabs[0].Title != "auth-refactor" {
		t.Errorf("iterm tabs: %+v", tabs)
	}
	if !strings.Contains(tabs[0].Command, "tmux -L arteta attach -t arteta-auth-refactor") {
		t.Errorf("tab command: %q", tabs[0].Command)
	}

	loaded, err := st.LoadWorkflow("auth-refactor")
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if loaded.ITermTab == nil {
		t.Error("iterm tab handle not persisted")
	}
}

func TestCreate_DuplicateName_Errors(t *testing.T) {
	s, _, _, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "x", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := s.Create(CreateOpts{Name: "x", Cwd: "/r", Layout: workflow.LayoutSingle}); err == nil {
		t.Error("second Create with same name returned nil, want error")
	}
}

func TestCreate_InvalidName_Errors(t *testing.T) {
	s, _, _, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "bad name", Cwd: "/r", Layout: workflow.LayoutSingle}); err == nil {
		t.Error("Create with invalid name returned nil, want error")
	}
}

func TestCreate_InvalidLayout_Errors(t *testing.T) {
	s, _, _, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "x", Cwd: "/r", Layout: workflow.Layout("bogus")}); err == nil {
		t.Error("Create with invalid layout returned nil, want error")
	}
}

func TestClose_KillsTmuxAndCleansUp(t *testing.T) {
	s, tx, tm, st := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "doomed", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Close("doomed"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if has, _ := tx.HasSession("arteta-doomed"); has {
		t.Error("tmux session still exists after Close")
	}
	if len(tm.Tabs()) != 0 {
		t.Errorf("iterm tabs after Close: %+v", tm.Tabs())
	}
	if _, err := st.LoadWorkflow("doomed"); err == nil {
		t.Error("workflow file still exists after Close")
	}
}

func TestClose_TmuxAlreadyDead_Succeeds(t *testing.T) {
	s, tx, _, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "x", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Simulate the tmux session having died externally.
	_ = tx.KillSession("arteta-x")
	// Close should still cleanly remove iterm tab + state without erroring on the
	// missing tmux session.
	if err := s.Close("x"); err != nil {
		t.Errorf("Close with missing tmux session: %v", err)
	}
}

func TestOpen_FocusesExistingTab(t *testing.T) {
	s, _, tm, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "wf", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tm.Calls = nil
	if err := s.Open("wf"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// We should see a TabExists check followed by a FocusTab — no new OpenTab.
	for _, c := range tm.Calls {
		if strings.HasPrefix(c, "OpenTab") {
			t.Errorf("Open of existing tab should not OpenTab: calls=%v", tm.Calls)
		}
	}
	if !containsCall(tm.Calls, "FocusTab") {
		t.Errorf("Open should FocusTab: calls=%v", tm.Calls)
	}
}

func TestOpen_RecreatesTabIfClosed(t *testing.T) {
	s, _, tm, st := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "wf", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// User closed the iTerm tab manually.
	w, _ := st.LoadWorkflow("wf")
	_ = tm.CloseTab(terminal.TabHandle{WindowID: w.ITermTab.WindowID, TabID: w.ITermTab.TabID})
	tm.Calls = nil
	if err := s.Open("wf"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !containsCall(tm.Calls, "OpenTab") {
		t.Errorf("Open after manual close should OpenTab: calls=%v", tm.Calls)
	}
	// Stored handle should have been refreshed.
	w2, _ := st.LoadWorkflow("wf")
	if w2.ITermTab.TabID == w.ITermTab.TabID {
		t.Error("expected new tab ID after recreate, got same ID")
	}
}

func TestRevive_ResumesClaudeSession(t *testing.T) {
	s, tx, _, st := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "wf", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Simulate tmux session dying + we have a known session_id.
	_ = tx.KillSession("arteta-wf")
	w, _ := st.LoadWorkflow("wf")
	w.ClaudeSessionID = "abc-123"
	_ = st.SaveWorkflow(w)

	if err := s.Revive("wf"); err != nil {
		t.Fatalf("Revive: %v", err)
	}
	sess := tx.Sessions()["arteta-wf"]
	if sess == nil {
		t.Fatal("tmux session not recreated")
	}
	if !strings.Contains(sess.Panes[0].Cmd, "claude --resume abc-123") {
		t.Errorf("claude command on revive: got %q, want resume", sess.Panes[0].Cmd)
	}
}

func TestRevive_NoSessionID_LaunchesFreshClaude(t *testing.T) {
	s, tx, _, _ := newTestService(t)
	if _, err := s.Create(CreateOpts{Name: "wf", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = tx.KillSession("arteta-wf")
	if err := s.Revive("wf"); err != nil {
		t.Fatalf("Revive: %v", err)
	}
	sess := tx.Sessions()["arteta-wf"]
	if sess.Panes[0].Cmd != "claude" {
		t.Errorf("revive without session_id: got cmd %q, want %q", sess.Panes[0].Cmd, "claude")
	}
}

func containsCall(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
