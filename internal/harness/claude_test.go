package harness

import (
	"strings"
	"testing"

	"github.com/hcwong/arteta/internal/workflow"
)

const shellFallback = "exec ${SHELL"

func TestClaudeHarness_LaunchCommand(t *testing.T) {
	h := claudeHarness{}

	got := h.LaunchCommand("")
	if !strings.HasPrefix(got, "claude") {
		t.Errorf("LaunchCommand(\"\") should start with claude, got %q", got)
	}
	if got == "claude" {
		t.Errorf("LaunchCommand(\"\") must not be bare \"claude\" — pane closes on Claude exit")
	}
	if !strings.Contains(got, shellFallback) {
		t.Errorf("LaunchCommand(\"\") missing shell fallback %q, got %q", shellFallback, got)
	}

	resumed := h.LaunchCommand("abc-123")
	if !strings.Contains(resumed, "claude --resume abc-123") {
		t.Errorf("LaunchCommand(sid) should include --resume, got %q", resumed)
	}
	if !strings.Contains(resumed, shellFallback) {
		t.Errorf("LaunchCommand(sid) missing shell fallback, got %q", resumed)
	}
}

func TestClaudeHarness_DetectState_Box(t *testing.T) {
	h := claudeHarness{}
	content := "╭──────────────╮\n│ Allow? (y/n) │\n╰──────────────╯"
	state, ok := h.DetectState(content)
	if !ok {
		t.Fatal("expected ok=true for box pattern")
	}
	if state != workflow.StateAwaitingInput {
		t.Errorf("box pattern: got %v, want StateAwaitingInput", state)
	}
}

func TestClaudeHarness_DetectState_Idle(t *testing.T) {
	h := claudeHarness{}
	state, ok := h.DetectState("output\n> ")
	if !ok {
		t.Fatal("expected ok=true for idle prompt")
	}
	if state != workflow.StateIdle {
		t.Errorf("idle prompt: got %v, want StateIdle", state)
	}
}

func TestClaudeHarness_DetectState_Running_NoSignal(t *testing.T) {
	h := claudeHarness{}
	_, ok := h.DetectState("⠋ Thinking...\nsome output\n")
	if ok {
		t.Error("expected ok=false for running output (no override)")
	}
}

func TestClaudeHarness_HookConfig(t *testing.T) {
	h := claudeHarness{}
	hc := h.HookConfig()
	if hc == nil {
		t.Fatal("HookConfig() returned nil for Claude")
	}
	if !strings.Contains(hc.SettingsPath, ".claude") {
		t.Errorf("SettingsPath should contain .claude: %q", hc.SettingsPath)
	}

	wantStates := map[string]workflow.State{
		"stop":               workflow.StateIdle,
		"notification":       workflow.StateAwaitingInput,
		"user-prompt-submit": workflow.StateRunning,
	}
	if len(hc.Events) != len(wantStates) {
		t.Fatalf("Events count: got %d, want %d", len(hc.Events), len(wantStates))
	}
	for _, ev := range hc.Events {
		want, ok := wantStates[ev.Subcommand]
		if !ok {
			t.Errorf("unexpected subcommand %q", ev.Subcommand)
			continue
		}
		if ev.State != want {
			t.Errorf("event %q state: got %v, want %v", ev.Subcommand, ev.State, want)
		}
		if ev.ParsePayload == nil {
			t.Errorf("event %q has nil ParsePayload", ev.Subcommand)
		}
	}
	// Notification must capture message
	for _, ev := range hc.Events {
		if ev.Subcommand == "notification" && !ev.CaptureMessage {
			t.Error("notification event should have CaptureMessage=true")
		}
	}
}

func TestClaudeParsePayload(t *testing.T) {
	raw := []byte(`{"session_id":"sid-1","message":"test","cwd":"/ignored","extra":true}`)
	sid, msg, err := ClaudeParsePayload(raw)
	if err != nil {
		t.Fatalf("ClaudeParsePayload: %v", err)
	}
	if sid != "sid-1" {
		t.Errorf("session_id: got %q, want %q", sid, "sid-1")
	}
	if msg != "test" {
		t.Errorf("message: got %q, want %q", msg, "test")
	}
}

func TestRegistry_Get_Claude(t *testing.T) {
	h := Get("claude")
	if h == nil || h.ID() != "claude" {
		t.Fatalf("Get(\"claude\") = %v, want claude harness", h)
	}
}

func TestRegistry_Get_Unknown_FallsBackToClaude(t *testing.T) {
	h := Get("unknown-xyz-harness")
	if h == nil || h.ID() != "claude" {
		t.Fatalf("Get(unknown) should fallback to claude, got %v", h)
	}
}

func TestRegistry_Default(t *testing.T) {
	h := Default()
	if h == nil || h.ID() != "claude" {
		t.Fatalf("Default() should be claude, got %v", h)
	}
}

// TestFakeHarness_Hookless verifies that a harness with nil HookConfig (no
// hook mechanism) is filtered out by WithHooks and degrades gracefully.
func TestFakeHarness_Hookless(t *testing.T) {
	hookless := &Fake{FakeID: "test-hookless"}
	Register(hookless)

	if hookless.HookConfig() != nil {
		t.Error("hookless harness should return nil HookConfig")
	}

	// WithHooks must NOT include the hookless harness.
	for _, h := range WithHooks() {
		if h.ID() == "test-hookless" {
			t.Error("WithHooks() returned hookless harness")
		}
	}

	// Get / LaunchCommand / DetectState must work without panicking.
	h := Get("test-hookless")
	cmd := h.LaunchCommand("")
	if cmd != "test-hookless" {
		t.Errorf("LaunchCommand(\"\") = %q, want %q", cmd, "test-hookless")
	}
	_, ok := h.DetectState("some pane content")
	if ok {
		t.Error("hookless Fake.DetectState should return ok=false by default")
	}
}

func TestEventNames(t *testing.T) {
	defs := []EventDef{
		{RawEventName: "Stop"},
		{RawEventName: "Notification"},
		{RawEventName: "UserPromptSubmit"},
	}
	got := EventNames(defs)
	want := []string{"Stop", "Notification", "UserPromptSubmit"}
	if len(got) != len(want) {
		t.Fatalf("EventNames len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("EventNames[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}
