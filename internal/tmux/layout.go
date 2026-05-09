package tmux

import (
	"fmt"

	"github.com/hcwong/arteta/internal/workflow"
)

// BuildOpts configures BuildLayout.
type BuildOpts struct {
	Client    Client
	Name      string // tmux session name
	Cwd       string
	Layout    workflow.Layout
	ClaudeCmd string // typically "claude" or "claude --resume <sid>"
	Env       map[string]string
}

// Pane content commands per layout, hardcoded for MVP (DECISIONS.md §6).
const (
	cmdTerminal = "" // empty → default shell
	cmdNvim     = "nvim ."
	cmdGitDiff  = "git diff --color=always | less -R"
)

// BuildLayout creates a session with the requested layout and pane content.
// Pane order matches DECISIONS.md §6:
//   - single: [claude]
//   - vsplit: [claude, terminal]
//   - hsplit: [claude, terminal]
//   - quad:   [claude, terminal, nvim, git-diff]
func BuildLayout(opts BuildOpts) error {
	if !opts.Layout.Valid() {
		return fmt.Errorf("invalid layout %q", opts.Layout)
	}
	c := opts.Client
	if err := c.NewSession(NewSessionOpts{
		Name: opts.Name,
		Cwd:  opts.Cwd,
		Cmd:  opts.ClaudeCmd,
		Env:  opts.Env,
	}); err != nil {
		return fmt.Errorf("new-session: %w", err)
	}
	switch opts.Layout {
	case workflow.LayoutSingle:
		return nil
	case workflow.LayoutVSplit:
		return c.SplitWindow(SplitOpts{Target: opts.Name, Dir: SplitVertical, Cwd: opts.Cwd, Cmd: cmdTerminal, Env: opts.Env})
	case workflow.LayoutHSplit:
		return c.SplitWindow(SplitOpts{Target: opts.Name, Dir: SplitHorizontal, Cwd: opts.Cwd, Cmd: cmdTerminal, Env: opts.Env})
	case workflow.LayoutQuad:
		// Pane 0 already exists (claude). Add 3 more, then tile.
		for _, pane := range []struct {
			dir SplitDir
			cmd string
		}{
			{SplitVertical, cmdTerminal}, // pane 1: terminal (right of claude)
			{SplitHorizontal, cmdNvim},   // pane 2: nvim (under claude or under terminal — tiled below)
			{SplitHorizontal, cmdGitDiff},
		} {
			if err := c.SplitWindow(SplitOpts{Target: opts.Name, Dir: pane.dir, Cwd: opts.Cwd, Cmd: pane.cmd, Env: opts.Env}); err != nil {
				return fmt.Errorf("split-window: %w", err)
			}
		}
		return c.SelectLayout(opts.Name, "tiled")
	}
	return fmt.Errorf("unhandled layout %q", opts.Layout)
}
