package service

import (
	"strings"
	"testing"
	"time"

	"github.com/hcwong/arteta/internal/harness"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

// claudeCommand is a test helper that mirrors the Claude harness's launch
// command, used to verify pane-0 command strings in service tests.
func claudeCommand(sessionID string) string {
	return harness.Get("claude").LaunchCommand(sessionID)
}

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

// shellFallback is the marker the pane-0 command must carry so that, when
// Claude exits, the pane drops to an interactive shell instead of closing
// (which would tear down the tmux session and the iTerm tab). See plan.
const shellFallback = "exec ${SHELL"

// TestClaudeCommand verifies the launch-command contract through the Claude
// harness. Kept in the service package because service tests rely on the
// claudeCommand helper below, and this test documents that contract.
func TestClaudeCommand(t *testing.T) {
	got := claudeCommand("")
	if got == "claude" {
		t.Fatalf("claudeCommand(\"\") must not be bare %q — the pane would close on Claude exit", got)
	}
	if !strings.HasPrefix(got, "claude") {
		t.Errorf("claudeCommand(\"\") should start with claude: got %q", got)
	}
	if !strings.Contains(got, shellFallback) {
		t.Errorf("claudeCommand(\"\") missing shell fallback %q: got %q", shellFallback, got)
	}

	resumed := claudeCommand("abc-123")
	if !strings.Contains(resumed, "claude --resume abc-123") {
		t.Errorf("claudeCommand(sid) should resume: got %q", resumed)
	}
	if !strings.Contains(resumed, shellFallback) {
		t.Errorf("claudeCommand(sid) missing shell fallback %q: got %q", shellFallback, resumed)
	}
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
	if !strings.Contains(sess.Panes[0].Cmd, shellFallback) {
		t.Errorf("claude pane should keep tab alive on exit, missing %q: got %q", shellFallback, sess.Panes[0].Cmd)
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
	w.SessionID = "abc-123"
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
	if sess.Panes[0].Cmd != claudeCommand("") {
		t.Errorf("revive without session_id: got cmd %q, want %q", sess.Panes[0].Cmd, claudeCommand(""))
	}
}

func TestRestartAll_LiveSessionsGetRespawned(t *testing.T) {
	s, tx, _, st := newTestService(t)

	// Create two live workflows.
	if _, err := s.Create(CreateOpts{Name: "wf1", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create wf1: %v", err)
	}
	if _, err := s.Create(CreateOpts{Name: "wf2", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create wf2: %v", err)
	}

	// Give wf2 a known SessionID.
	w2, _ := st.LoadWorkflow("wf2")
	w2.SessionID = "sess-abc"
	_ = st.SaveWorkflow(w2)

	tx.Calls = nil
	n, err := s.RestartAll()
	if err != nil {
		t.Fatalf("RestartAll: %v", err)
	}
	if n != 2 {
		t.Errorf("RestartAll returned %d, want 2", n)
	}

	// Both sessions should have had RespawnPane called.
	respawn1 := containsCall(tx.Calls, "RespawnPane:arteta-wf1:0")
	respawn2 := containsCall(tx.Calls, "RespawnPane:arteta-wf2:0")
	if !respawn1 || !respawn2 {
		t.Errorf("RespawnPane calls: %v", tx.Calls)
	}

	// wf1 has no session ID → cmd should be the plain-claude command.
	sess1 := tx.Sessions()["arteta-wf1"]
	if sess1.Panes[0].Cmd != claudeCommand("") {
		t.Errorf("wf1 pane cmd: got %q, want %q", sess1.Panes[0].Cmd, claudeCommand(""))
	}

	// wf2 has a session ID → cmd should be "claude --resume sess-abc".
	sess2 := tx.Sessions()["arteta-wf2"]
	if !strings.Contains(sess2.Panes[0].Cmd, "claude --resume sess-abc") {
		t.Errorf("wf2 pane cmd: got %q, want claude --resume sess-abc", sess2.Panes[0].Cmd)
	}
}

func TestRestartAll_DormantSessionsSkipped(t *testing.T) {
	s, tx, _, _ := newTestService(t)

	// Create one workflow and kill the tmux session to make it dormant.
	if _, err := s.Create(CreateOpts{Name: "dormant", Cwd: "/r", Layout: workflow.LayoutSingle}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = tx.KillSession("arteta-dormant")

	tx.Calls = nil
	n, err := s.RestartAll()
	if err != nil {
		t.Fatalf("RestartAll: %v", err)
	}
	if n != 0 {
		t.Errorf("RestartAll returned %d, want 0", n)
	}
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "RespawnPane") {
			t.Errorf("RespawnPane should not be called for dormant session, got: %v", tx.Calls)
		}
	}
}

func TestRestartAll_NoWorkflows_ReturnsZero(t *testing.T) {
	s, _, _, _ := newTestService(t)
	n, err := s.RestartAll()
	if err != nil {
		t.Fatalf("RestartAll: %v", err)
	}
	if n != 0 {
		t.Errorf("RestartAll returned %d, want 0", n)
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

// cycleContent maps a desired screen-detected state to canned pane output that
// detect.FromPaneContent will classify as that state. StateUnknown yields
// neutral content with no detectable signal (so hook state wins).
func cycleContent(state workflow.State) string {
	switch state {
	case workflow.StateAwaitingInput:
		return "╭──────────────╮\nDo you want to proceed?\n╰──────────────╯"
	case workflow.StateIdle:
		return "ran the build\n> "
	default:
		return "working on it..."
	}
}

// setupCycleWF persists a live workflow with a tmux session, canned pane
// content for screen detection, and a hook status with the given timestamp.
func setupCycleWF(t *testing.T, st *store.Store, tx *tmux.Fake, name string, screen workflow.State, hookEvent string, ts time.Time) {
	t.Helper()
	session := workflow.TmuxSessionName(name)
	w := workflow.Workflow{
		Name:        name,
		Cwd:         "/repo",
		TmuxSession: session,
		Layout:      workflow.LayoutSingle,
		ITermTab:    &workflow.ITermTab{WindowID: "1", TabID: name},
	}
	if err := st.SaveWorkflow(w); err != nil {
		t.Fatalf("SaveWorkflow(%s): %v", name, err)
	}
	if err := tx.NewSession(tmux.NewSessionOpts{Name: session, Cwd: "/repo"}); err != nil {
		t.Fatalf("NewSession(%s): %v", session, err)
	}
	tx.SetPaneOutput(session, cycleContent(screen))
	if err := st.SaveStatus(name, workflow.Status{LastEvent: hookEvent, Timestamp: ts}); err != nil {
		t.Fatalf("SaveStatus(%s): %v", name, err)
	}
}

func ts(hour int) time.Time {
	return time.Date(2026, 5, 9, hour, 0, 0, 0, time.UTC)
}

// TestCycle_OrdersAndWraps verifies the candidate ordering (awaiting before
// idle, oldest-first within group), that running/dormant are excluded, that
// next advances + wraps, and that the cursor is persisted each step.
func TestCycle_OrdersAndWraps(t *testing.T) {
	s, tx, _, st := newTestService(t)
	// wfA awaiting @10, wfB awaiting @09 (older), wfC idle @08, wfD running.
	setupCycleWF(t, st, tx, "wfA", workflow.StateAwaitingInput, "Notification", ts(10))
	setupCycleWF(t, st, tx, "wfB", workflow.StateAwaitingInput, "Notification", ts(9))
	setupCycleWF(t, st, tx, "wfC", workflow.StateIdle, "Stop", ts(8))
	setupCycleWF(t, st, tx, "wfD", workflow.StateUnknown, "UserPromptSubmit", ts(7)) // running, excluded

	want := []string{"wfB", "wfA", "wfC", "wfB"} // expected sequence of next presses (wrap)
	for i, w := range want {
		got, _, err := s.Cycle(DirNext)
		if err != nil {
			t.Fatalf("Cycle press %d: %v", i, err)
		}
		if got != w {
			t.Fatalf("Cycle press %d = %q, want %q", i, got, w)
		}
		cursor, _ := st.LoadCycleCursor()
		if cursor != w {
			t.Errorf("cursor after press %d = %q, want %q", i, cursor, w)
		}
	}
}

// TestCycle_Prev steps backward and wraps.
func TestCycle_Prev(t *testing.T) {
	s, tx, _, st := newTestService(t)
	setupCycleWF(t, st, tx, "wfA", workflow.StateAwaitingInput, "Notification", ts(10))
	setupCycleWF(t, st, tx, "wfB", workflow.StateAwaitingInput, "Notification", ts(9))
	setupCycleWF(t, st, tx, "wfC", workflow.StateIdle, "Stop", ts(8))

	// Order: wfB, wfA, wfC. Cursor empty + prev -> last (wfC).
	want := []string{"wfC", "wfA", "wfB", "wfC"}
	for i, w := range want {
		got, _, err := s.Cycle(DirPrev)
		if err != nil {
			t.Fatalf("Cycle(prev) press %d: %v", i, err)
		}
		if got != w {
			t.Fatalf("Cycle(prev) press %d = %q, want %q", i, got, w)
		}
	}
}

// TestCycle_StaleCursorFallsBackToFirst: when the stored cursor is not a
// current candidate (e.g. that workflow is now running), next starts at the
// first candidate.
func TestCycle_StaleCursorFallsBackToFirst(t *testing.T) {
	s, tx, _, st := newTestService(t)
	setupCycleWF(t, st, tx, "wfA", workflow.StateAwaitingInput, "Notification", ts(10))
	setupCycleWF(t, st, tx, "wfB", workflow.StateIdle, "Stop", ts(9))
	if err := st.SaveCycleCursor("ghost"); err != nil {
		t.Fatalf("SaveCycleCursor: %v", err)
	}
	got, state, err := s.Cycle(DirNext)
	if err != nil {
		t.Fatalf("Cycle: %v", err)
	}
	if got != "wfA" {
		t.Errorf("Cycle with stale cursor = %q, want first candidate %q", got, "wfA")
	}
	if state != workflow.StateAwaitingInput {
		t.Errorf("returned state = %v, want awaiting_input", state)
	}
}

// TestCycle_NoCandidates returns empty without focusing anything.
func TestCycle_NoCandidates(t *testing.T) {
	s, tx, tm, st := newTestService(t)
	setupCycleWF(t, st, tx, "wfA", workflow.StateUnknown, "UserPromptSubmit", ts(10)) // running

	got, _, err := s.Cycle(DirNext)
	if err != nil {
		t.Fatalf("Cycle: %v", err)
	}
	if got != "" {
		t.Errorf("Cycle with no candidates = %q, want \"\"", got)
	}
	if containsCall(tm.Calls, "OpenTab") || containsCall(tm.Calls, "FocusTab") {
		t.Errorf("no tab op expected when nothing to cycle to, got: %v", tm.Calls)
	}
}

// TestFakeHarness_Create verifies that Service.Create respects a non-Claude
// harness ID: it writes the harness ID into the persisted workflow and launches
// the harness's own command string in pane 0.
func TestFakeHarness_Create(t *testing.T) {
	// Register a temporary fake harness.
	fakeH := &harness.Fake{
		FakeID: "test-fake-harness",
		FakeLaunchCmd: func(resumeID string) string {
			if resumeID != "" {
				return "fake --resume " + resumeID
			}
			return "fake-agent"
		},
	}
	harness.Register(fakeH)

	s, tx, _, st := newTestService(t)
	w, err := s.Create(CreateOpts{
		Name:    "fake-wf",
		Cwd:     "/r",
		Layout:  workflow.LayoutSingle,
		Harness: "test-fake-harness",
	})
	if err != nil {
		t.Fatalf("Create with fake harness: %v", err)
	}
	if w.Harness != "test-fake-harness" {
		t.Errorf("Workflow.Harness: got %q, want %q", w.Harness, "test-fake-harness")
	}

	// Pane 0 must use the fake harness's command, not "claude".
	sess := tx.Sessions()["arteta-fake-wf"]
	if sess == nil {
		t.Fatal("tmux session not created")
	}
	if sess.Panes[0].Cmd != "fake-agent" {
		t.Errorf("pane 0 cmd: got %q, want %q", sess.Panes[0].Cmd, "fake-agent")
	}

	// Persisted workflow must record the harness ID.
	loaded, err := st.LoadWorkflow("fake-wf")
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if loaded.Harness != "test-fake-harness" {
		t.Errorf("persisted Harness: got %q, want %q", loaded.Harness, "test-fake-harness")
	}
}

// TestOpen_WritesCycleCursor verifies the homepage open path shares the cursor.
func TestOpen_WritesCycleCursor(t *testing.T) {
	s, tx, _, st := newTestService(t)
	setupCycleWF(t, st, tx, "wfA", workflow.StateIdle, "Stop", ts(10))

	if err := s.Open("wfA"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	cursor, _ := st.LoadCycleCursor()
	if cursor != "wfA" {
		t.Errorf("cursor after Open = %q, want %q", cursor, "wfA")
	}
}
