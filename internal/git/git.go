package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrPathExists       = errors.New("worktree path already exists")
	ErrBranchCheckedOut = errors.New("branch is checked out in another worktree")
	ErrInvalidRef       = errors.New("invalid git ref")
	ErrNotAGitRepo      = errors.New("not a git repository")
	ErrCannotRemoveMain = errors.New("cannot remove main worktree")
)

type WorktreeOpts struct {
	Path   string
	Branch string
	Base   string
	Attach bool
}

type Worktree struct {
	Path   string
	Branch string
	IsMain bool
}

type StatusResult struct {
	Dirty         bool
	UnpushedCount int
}

type Client interface {
	CreateWorktree(opts WorktreeOpts) error
	RemoveWorktree(path string) error
	ListWorktrees() ([]Worktree, error)
	BranchExists(name string) (bool, error)
	DefaultBranch() (string, error)
	RepoRoot() (string, error)
	Status(cwd string) (StatusResult, error)
	DeleteBranch(name string) error
	IsGitRepo() bool
}

type realClient struct {
	dir string
}

func NewReal(dir string) Client {
	return &realClient{dir: dir}
}

func (c *realClient) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w (stderr: %s)",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *realClient) gitAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git -C %s %s: %w (stderr: %s)",
			dir, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *realClient) RepoRoot() (string, error) {
	out, err := c.git("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
	}
	return out, nil
}

func (c *realClient) IsGitRepo() bool {
	_, err := c.git("rev-parse", "--git-dir")
	return err == nil
}

func (c *realClient) DefaultBranch() (string, error) {
	out, err := c.git("symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimPrefix(out, "refs/remotes/origin/"), nil
	}
	// Fallback: check for common default branch names.
	for _, name := range []string{"main", "master"} {
		if ok, _ := c.BranchExists(name); ok {
			return name, nil
		}
	}
	return "", fmt.Errorf("cannot determine default branch: %w", err)
}

func (c *realClient) BranchExists(name string) (bool, error) {
	_, err := c.git("rev-parse", "--verify", "refs/heads/"+name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *realClient) CreateWorktree(opts WorktreeOpts) error {
	if _, err := os.Stat(opts.Path); err == nil {
		return fmt.Errorf("%w: %s", ErrPathExists, opts.Path)
	}

	if opts.Attach {
		_, err := c.git("worktree", "add", opts.Path, opts.Branch)
		if err != nil {
			if strings.Contains(err.Error(), "already checked out") {
				return fmt.Errorf("%w: %s", ErrBranchCheckedOut, opts.Branch)
			}
			return err
		}
		return nil
	}

	if err := ValidateBranchName(opts.Branch); err != nil {
		return err
	}

	base := opts.Base
	if base == "" {
		var err error
		base, err = c.DefaultBranch()
		if err != nil {
			return err
		}
	}

	_, err := c.git("worktree", "add", "-b", opts.Branch, opts.Path, base)
	if err != nil {
		if strings.Contains(err.Error(), "invalid reference") {
			return fmt.Errorf("%w: %s", ErrInvalidRef, base)
		}
		if strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("%w: %s", ErrBranchCheckedOut, opts.Branch)
		}
		return err
	}
	return nil
}

func (c *realClient) RemoveWorktree(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := c.RepoRoot()
	if err != nil {
		return err
	}
	rootAbs, _ := filepath.EvalSymlinks(root)
	pathAbs, _ := filepath.EvalSymlinks(abs)
	if rootAbs == pathAbs {
		return ErrCannotRemoveMain
	}
	_, err = c.git("worktree", "remove", "--force", abs)
	return err
}

func (c *realClient) ListWorktrees() ([]Worktree, error) {
	out, err := c.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out), nil
}

func parseWorktreeList(output string) []Worktree {
	var wts []Worktree
	var current Worktree
	inEntry := false

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			if inEntry {
				wts = append(wts, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
			inEntry = true
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			current.IsMain = true
		case line == "":
			if inEntry {
				wts = append(wts, current)
				current = Worktree{}
				inEntry = false
			}
		}
	}
	if inEntry {
		wts = append(wts, current)
	}

	if len(wts) > 0 {
		wts[0].IsMain = true
	}
	return wts
}

func (c *realClient) Status(cwd string) (StatusResult, error) {
	var result StatusResult

	out, err := c.gitAt(cwd, "status", "--porcelain")
	if err != nil {
		return result, err
	}
	result.Dirty = strings.TrimSpace(out) != ""

	countOut, err := c.gitAt(cwd, "rev-list", "@{u}..HEAD", "--count")
	if err != nil {
		// No upstream tracked — treat as 0 unpushed.
		result.UnpushedCount = 0
		return result, nil
	}
	n, _ := strconv.Atoi(strings.TrimSpace(countOut))
	result.UnpushedCount = n
	return result, nil
}

func (c *realClient) DeleteBranch(name string) error {
	if name == "main" || name == "master" {
		return fmt.Errorf("%w: %s", ErrCannotRemoveMain, name)
	}
	_, err := c.git("branch", "-D", name)
	return err
}

func ValidateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name cannot start with '-'")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("branch name cannot start with '.'")
	}
	if strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("branch name cannot end with '.lock'")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name cannot contain '..'")
	}
	for _, c := range name {
		switch c {
		case ' ', '~', '^', ':', '\\', '*', '?', '[', 0x7f:
			return fmt.Errorf("branch name contains invalid character %q", c)
		}
		if c < 0x20 {
			return fmt.Errorf("branch name contains control character")
		}
	}
	return nil
}
