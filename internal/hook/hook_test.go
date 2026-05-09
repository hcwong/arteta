package hook

import (
	"strings"
	"testing"
	"time"

	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/workflow"
)

func newTestHandler(t *testing.T, env map[string]string) (*Handler, *store.Store) {
	t.Helper()
	s := store.New(t.TempDir())
	now := time.Date(2026, 5, 9, 17, 0, 0, 0, time.UTC)
	h := &Handler{
		Store:  s,
		Now:    func() time.Time { return now },
		Lookup: func(k string) string { return env[k] },
	}
	return h, s
}

func TestHandle_NoArtetaWorkflow_NoOps(t *testing.T) {
	h, s := newTestHandler(t, map[string]string{})
	wrote, err := h.Handle(workflow.EventStop, strings.NewReader(`{"session_id":"x"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if wrote {
		t.Errorf("expected wrote=false when ARTETA_WORKFLOW unset, got true")
	}
	// And no status file should exist for any name.
	if _, err := s.LoadStatus("anything"); err == nil {
		t.Errorf("expected no status file written, got one")
	}
}

func TestHandle_Stop_WritesIdleStatus(t *testing.T) {
	h, s := newTestHandler(t, map[string]string{"ARTETA_WORKFLOW": "auth"})
	payload := `{"session_id":"sid-1","cwd":"/x","hook_event_name":"Stop"}`
	wrote, err := h.Handle(workflow.EventStop, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	st, err := s.LoadStatus("auth")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.LastEvent != "Stop" {
		t.Errorf("LastEvent: got %q, want %q", st.LastEvent, "Stop")
	}
	if st.SessionID != "sid-1" {
		t.Errorf("SessionID: got %q, want %q", st.SessionID, "sid-1")
	}
	if st.LastMessage != "" {
		t.Errorf("LastMessage on Stop: got %q, want empty", st.LastMessage)
	}
	if st.State() != workflow.StateIdle {
		t.Errorf("State: got %v, want %v", st.State(), workflow.StateIdle)
	}
}

func TestHandle_Notification_CapturesMessage(t *testing.T) {
	h, s := newTestHandler(t, map[string]string{"ARTETA_WORKFLOW": "login"})
	payload := `{
		"session_id":"sid-2",
		"message":"Should I keep the legacy fallback?",
		"hook_event_name":"Notification"
	}`
	wrote, err := h.Handle(workflow.EventNotification, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	st, err := s.LoadStatus("login")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.LastMessage != "Should I keep the legacy fallback?" {
		t.Errorf("LastMessage: got %q", st.LastMessage)
	}
	if st.State() != workflow.StateAwaitingInput {
		t.Errorf("State: got %v, want %v", st.State(), workflow.StateAwaitingInput)
	}
}

func TestHandle_UserPromptSubmit_DoesNotEchoPrompt(t *testing.T) {
	h, s := newTestHandler(t, map[string]string{"ARTETA_WORKFLOW": "wf"})
	payload := `{"session_id":"sid-3","prompt":"please refactor this file"}`
	wrote, err := h.Handle(workflow.EventUserPromptSubmit, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	st, err := s.LoadStatus("wf")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.LastMessage != "" {
		t.Errorf("LastMessage on UserPromptSubmit should be empty (Arteta surfaces Claude's voice, not user's), got %q", st.LastMessage)
	}
	if st.State() != workflow.StateRunning {
		t.Errorf("State: got %v, want %v", st.State(), workflow.StateRunning)
	}
}

func TestHandle_BadJSON_Errors(t *testing.T) {
	h, _ := newTestHandler(t, map[string]string{"ARTETA_WORKFLOW": "wf"})
	_, err := h.Handle(workflow.EventStop, strings.NewReader("{not json"))
	if err == nil {
		t.Error("expected error on malformed JSON, got nil")
	}
}

func TestHandle_TruncatesLongMessage(t *testing.T) {
	h, s := newTestHandler(t, map[string]string{"ARTETA_WORKFLOW": "wf"})
	long := strings.Repeat("x", 1000)
	payload := `{"session_id":"sid","message":"` + long + `"}`
	if _, err := h.Handle(workflow.EventNotification, strings.NewReader(payload)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	st, err := s.LoadStatus("wf")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if len(st.LastMessage) > MaxMessageLen {
		t.Errorf("LastMessage length %d exceeds MaxMessageLen %d", len(st.LastMessage), MaxMessageLen)
	}
}
