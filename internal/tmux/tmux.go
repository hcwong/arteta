// Package tmux abstracts the tmux operations Arteta needs over its
// dedicated socket (`tmux -L arteta`). The Client interface is the
// boundary used by the rest of the codebase; tests use Fake.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DefaultSocket is the tmux socket name Arteta uses by default. See DECISIONS.md §2.
const DefaultSocket = "arteta"

// SplitDir controls how a pane is split.
type SplitDir int

const (
	// SplitVertical creates a left/right column split (tmux -h).
	SplitVertical SplitDir = iota
	// SplitHorizontal creates a top/bottom row split (tmux -v).
	SplitHorizontal
)

// NewSessionOpts configures a fresh tmux session.
type NewSessionOpts struct {
	Name string
	Cwd  string
	// Cmd is the shell command to run in the first pane. Empty means a default shell.
	Cmd string
	// Env adds environment variables to the session (used for ARTETA_WORKFLOW).
	Env map[string]string
}

// SplitOpts configures a split-window operation.
type SplitOpts struct {
	Target string // session or session:window.pane
	Dir    SplitDir
	Cwd    string
	Cmd    string
	Env    map[string]string
}

// Client is the operations Arteta needs from tmux.
type Client interface {
	HasSession(name string) (bool, error)
	ListSessions() ([]string, error)
	NewSession(opts NewSessionOpts) error
	KillSession(name string) error
	SplitWindow(opts SplitOpts) error
	SelectLayout(session, layout string) error
	// PaneCommands returns the current foreground command for each pane in the
	// session, ordered by pane index.
	PaneCommands(session string) ([]string, error)
}

// realClient shells out to `tmux -L <socket>`.
type realClient struct {
	socket string
}

// NewReal constructs a Client backed by the real `tmux` binary on the given socket.
func NewReal(socket string) Client {
	if socket == "" {
		socket = DefaultSocket
	}
	return &realClient{socket: socket}
}

func (c *realClient) base(args ...string) []string {
	return append([]string{"-L", c.socket}, args...)
}

func (c *realClient) run(args ...string) ([]byte, error) {
	cmd := exec.Command("tmux", args...)
	cmd.Env = artetaTmuxEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("tmux %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// artetaTmuxEnv returns the environment to hand to a child tmux process. We
// strip $TMUX so that running arteta from inside a personal tmux session
// doesn't cause tmux to refuse to spawn a nested server (it will refuse
// even when the child uses a different -L socket).
func artetaTmuxEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (c *realClient) HasSession(name string) (bool, error) {
	cmd := exec.Command("tmux", c.base("has-session", "-t", "="+name)...)
	cmd.Env = artetaTmuxEnv()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// tmux exits non-zero when the session doesn't exist or no server is running.
			return false, nil
		}
		return false, fmt.Errorf("has-session: %w", err)
	}
	return true, nil
}

func (c *realClient) ListSessions() ([]string, error) {
	out, err := c.run(c.base("list-sessions", "-F", "#{session_name}")...)
	if err != nil {
		// tmux prints "no server running" to stderr if no sessions exist; treat as empty.
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "no current session") {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	names := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

func (c *realClient) NewSession(opts NewSessionOpts) error {
	args := c.base("new-session", "-d", "-s", opts.Name)
	if opts.Cwd != "" {
		args = append(args, "-c", opts.Cwd)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}
	if opts.Cmd != "" {
		args = append(args, opts.Cmd)
	}
	_, err := c.run(args...)
	return err
}

func (c *realClient) KillSession(name string) error {
	_, err := c.run(c.base("kill-session", "-t", name)...)
	return err
}

func (c *realClient) SplitWindow(opts SplitOpts) error {
	dirFlag := "-h"
	if opts.Dir == SplitHorizontal {
		dirFlag = "-v"
	}
	args := c.base("split-window", dirFlag, "-t", opts.Target)
	if opts.Cwd != "" {
		args = append(args, "-c", opts.Cwd)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}
	if opts.Cmd != "" {
		args = append(args, opts.Cmd)
	}
	_, err := c.run(args...)
	return err
}

func (c *realClient) SelectLayout(session, layout string) error {
	_, err := c.run(c.base("select-layout", "-t", session, layout)...)
	return err
}

func (c *realClient) PaneCommands(session string) ([]string, error) {
	out, err := c.run(c.base("list-panes", "-t", session, "-F", "#{pane_current_command}")...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	cmds := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			cmds = append(cmds, l)
		}
	}
	return cmds, nil
}
