package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hcwong/arteta/internal/workflow"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir())
}

func sampleWorkflow(name string) workflow.Workflow {
	return workflow.Workflow{
		Name:            name,
		Cwd:             "/Users/josh/repo",
		TmuxSession:     workflow.TmuxSessionName(name),
		ClaudeSessionID: "abc-123",
		GitBranch:       "feat/x",
		Layout:          workflow.LayoutQuad,
		ITermTab:        &workflow.ITermTab{WindowID: "w1", TabID: "t1"},
		CreatedAt:       time.Date(2026, 5, 9, 17, 0, 0, 0, time.UTC),
	}
}

func TestSaveAndLoadWorkflow_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	in := sampleWorkflow("auth-refactor")
	if err := s.SaveWorkflow(in); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	got, err := s.LoadWorkflow("auth-refactor")
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if got.Name != in.Name || got.Cwd != in.Cwd || got.TmuxSession != in.TmuxSession {
		t.Errorf("roundtrip mismatch:\n got=%+v\nwant=%+v", got, in)
	}
	if got.Layout != in.Layout {
		t.Errorf("layout: got %q, want %q", got.Layout, in.Layout)
	}
	if got.ITermTab == nil || got.ITermTab.WindowID != "w1" {
		t.Errorf("iterm tab not preserved: %+v", got.ITermTab)
	}
	if !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("created_at: got %v, want %v", got.CreatedAt, in.CreatedAt)
	}
}

func TestLoadWorkflow_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LoadWorkflow("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLoadAllWorkflows_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.LoadAllWorkflows()
	if err != nil {
		t.Fatalf("LoadAllWorkflows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 workflows, got %d", len(got))
	}
}

func TestLoadAllWorkflows_Multiple(t *testing.T) {
	s := newTestStore(t)
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if err := s.SaveWorkflow(sampleWorkflow(n)); err != nil {
			t.Fatalf("SaveWorkflow(%q): %v", n, err)
		}
	}
	got, err := s.LoadAllWorkflows()
	if err != nil {
		t.Fatalf("LoadAllWorkflows: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 workflows, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, w := range got {
		seen[w.Name] = true
	}
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if !seen[n] {
			t.Errorf("workflow %q missing from LoadAllWorkflows result", n)
		}
	}
}

func TestDeleteWorkflow(t *testing.T) {
	s := newTestStore(t)
	w := sampleWorkflow("doomed")
	if err := s.SaveWorkflow(w); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	// Delete should also remove any status file.
	if err := s.SaveStatus("doomed", workflow.Status{LastEvent: "Stop", Timestamp: time.Now()}); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}
	if err := s.DeleteWorkflow("doomed"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	if _, err := s.LoadWorkflow("doomed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete, LoadWorkflow expected ErrNotFound, got %v", err)
	}
	if _, err := s.LoadStatus("doomed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete, LoadStatus expected ErrNotFound, got %v", err)
	}
}

func TestDeleteWorkflow_NotFound(t *testing.T) {
	s := newTestStore(t)
	// Deleting a non-existent workflow should be a no-op (idempotent).
	if err := s.DeleteWorkflow("ghost"); err != nil {
		t.Errorf("DeleteWorkflow on missing workflow returned error: %v", err)
	}
}

func TestSaveAndLoadStatus_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	in := workflow.Status{
		LastEvent:   "Notification",
		LastMessage: "Should I keep the legacy fallback?",
		SessionID:   "abc-123",
		Timestamp:   time.Date(2026, 5, 9, 17, 42, 0, 0, time.UTC),
	}
	if err := s.SaveStatus("login-bug", in); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}
	got, err := s.LoadStatus("login-bug")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if got.LastEvent != in.LastEvent || got.LastMessage != in.LastMessage || got.SessionID != in.SessionID {
		t.Errorf("status roundtrip mismatch:\n got=%+v\nwant=%+v", got, in)
	}
}

func TestLoadStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LoadStatus("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveWorkflow_NoTempLeftover(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveWorkflow(sampleWorkflow("clean")); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	// After a successful save, no tmp file should remain.
	matches, err := filepath.Glob(filepath.Join(s.WorkflowsDir(), "*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) > 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestSaveWorkflow_CreatesDirs(t *testing.T) {
	root := t.TempDir()
	// Don't pre-create subdirs; SaveWorkflow should mkdir as needed.
	s := New(root)
	if err := s.SaveWorkflow(sampleWorkflow("first")); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "workflows", "first.json")); err != nil {
		t.Errorf("workflow file not created: %v", err)
	}
}

func TestStatus_DeriveState(t *testing.T) {
	cases := []struct {
		event string
		want  workflow.State
	}{
		{"Stop", workflow.StateIdle},
		{"Notification", workflow.StateAwaitingInput},
		{"UserPromptSubmit", workflow.StateRunning},
		{"", workflow.StateUnknown},
	}
	for _, tc := range cases {
		s := workflow.Status{LastEvent: tc.event}
		if got := s.State(); got != tc.want {
			t.Errorf("Status{LastEvent:%q}.State() = %v, want %v", tc.event, got, tc.want)
		}
	}
}
