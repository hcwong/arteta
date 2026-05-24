package detect

import (
	"testing"

	"github.com/hcwong/arteta/internal/workflow"
)

func TestFromPaneContent_BoxPatternAwaitingInput(t *testing.T) {
	content := `
some output above
╭─────────────────────────────────────────────╮
│ Claude wants to run: bash(command="ls -la")  │
│                                              │
│ Allow? (y/n/d for details)                   │
╰─────────────────────────────────────────────╯
`
	state, ok := FromPaneContent(content)
	if !ok {
		t.Fatal("expected confident result, got ok=false")
	}
	if state != workflow.StateAwaitingInput {
		t.Errorf("got %v, want StateAwaitingInput", state)
	}
}

func TestFromPaneContent_IdlePrompt(t *testing.T) {
	content := "previous output\n> "
	state, ok := FromPaneContent(content)
	if !ok {
		t.Fatal("expected confident result, got ok=false")
	}
	if state != workflow.StateIdle {
		t.Errorf("got %v, want StateIdle", state)
	}
}

func TestFromPaneContent_ANSIStripped(t *testing.T) {
	// Box characters wrapped in ANSI colour codes.
	content := "\x1b[32m╭─────╮\x1b[0m\nsome text\n\x1b[32m╰─────╯\x1b[0m"
	state, ok := FromPaneContent(content)
	if !ok {
		t.Fatal("expected confident result with ANSI stripped, got ok=false")
	}
	if state != workflow.StateAwaitingInput {
		t.Errorf("got %v, want StateAwaitingInput", state)
	}
}

func TestFromPaneContent_RunningOutput_NoOverride(t *testing.T) {
	// Scrolling output with no box or prompt — should produce no override.
	content := "⠋ Thinking...\nsome tool output\nmore lines\n"
	_, ok := FromPaneContent(content)
	if ok {
		t.Error("expected ok=false for ambiguous running output")
	}
}

func TestFromPaneContent_EmptyContent_NoOverride(t *testing.T) {
	_, ok := FromPaneContent("")
	if ok {
		t.Error("expected ok=false for empty content")
	}
}

func TestFromPaneContent_IdlePromptWithBlankLines(t *testing.T) {
	// Trailing blank lines after the prompt should still be detected.
	content := "output\n> \n\n"
	state, ok := FromPaneContent(content)
	if !ok {
		t.Fatal("expected confident result")
	}
	if state != workflow.StateIdle {
		t.Errorf("got %v, want StateIdle", state)
	}
}

func TestFromPaneContent_TailOnly(t *testing.T) {
	// Box appears only in the tail; many lines of output above it.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line of output")
	}
	lines = append(lines, "╭──────╮")
	lines = append(lines, "│ yes? │")
	lines = append(lines, "╰──────╯")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	state, ok := FromPaneContent(content)
	if !ok {
		t.Fatal("expected confident result")
	}
	if state != workflow.StateAwaitingInput {
		t.Errorf("got %v, want StateAwaitingInput", state)
	}
}
