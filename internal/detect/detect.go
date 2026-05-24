// Package detect infers workflow state from tmux pane content, providing a
// second signal source that can override stale hook-reported state.
package detect

import (
	"regexp"
	"strings"

	"github.com/hcwong/arteta/internal/workflow"
)

const tailSize = 20

// ansiRE matches CSI escape sequences (SGR, cursor moves, etc.) emitted by
// tmux capture-pane -e. Stripped before pattern matching so box-drawing
// characters are not obscured by colour codes.
var ansiRE = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)

// FromPaneContent inspects the last tailSize lines of a tmux capture-pane
// output and returns a best-effort workflow state. The content may contain ANSI
// escape sequences; they are stripped before matching. Returns (state, true)
// only for high-confidence signals; callers should treat (StateUnknown, false)
// as "no override — trust hook state".
func FromPaneContent(content string) (workflow.State, bool) {
	lines := tailLines(content, tailSize)
	clean := stripANSI(strings.Join(lines, "\n"))
	return detectState(strings.Split(clean, "\n"))
}

func detectState(lines []string) (workflow.State, bool) {
	joined := strings.Join(lines, "\n")

	// Box-drawing borders: Claude Code renders permission/confirmation dialogs
	// using ╭─…╮ top and ╰─…╯ bottom. Both appearing near the bottom strongly
	// signals a blocking dialog waiting for approval.
	if strings.Contains(joined, "╭") && strings.Contains(joined, "╰") {
		return workflow.StateAwaitingInput, true
	}

	// Input cursor: Claude's "> " prompt at the last non-blank line means
	// Claude finished and is idle, waiting for the next user message.
	if last := lastNonBlank(lines); strings.HasSuffix(last, "> ") {
		return workflow.StateIdle, true
	}

	return workflow.StateUnknown, false
}

func tailLines(s string, n int) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func lastNonBlank(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
