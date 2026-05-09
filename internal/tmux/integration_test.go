//go:build integration

package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRealTmux_CapturePane creates a session, sleeps long enough for the
// shell to write its banner, and asserts capture-pane returns non-empty.
// Gated behind -tags=integration; needs `tmux` on PATH.
func TestRealTmux_CapturePane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	socket := fmt.Sprintf("arteta-itest-%d", time.Now().UnixNano())
	c := NewReal(socket)
	name := "capture-test"
	if err := c.NewSession(NewSessionOpts{Name: name, Cmd: "bash -c 'echo hello-capture; sleep 60'"}); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = c.KillSession(name) }()
	// Give bash a moment to print before tmux captures.
	time.Sleep(200 * time.Millisecond)
	out, err := c.CapturePane(name, 0)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if !strings.Contains(out, "hello-capture") {
		t.Errorf("CapturePane output missing seeded text:\n%s", out)
	}
}
