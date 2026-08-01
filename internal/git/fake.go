package git

import (
	"fmt"
	"sync"
)

type Fake struct {
	mu         sync.Mutex
	worktrees  map[string]Worktree // keyed by path
	branches   map[string]bool
	repoRoot   string
	defaultBr  string
	isRepo     bool
	Calls      []string
	StatusFunc func(cwd string) (StatusResult, error)
}

func NewFake(repoRoot string) *Fake {
	return &Fake{
		worktrees: map[string]Worktree{},
		branches:  map[string]bool{},
		repoRoot:  repoRoot,
		defaultBr: "main",
		isRepo:    true,
	}
}

func (f *Fake) SetNotARepo() { f.isRepo = false }

func (f *Fake) RepoRoot() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "RepoRoot")
	if !f.isRepo {
		return "", ErrNotAGitRepo
	}
	return f.repoRoot, nil
}

func (f *Fake) IsGitRepo() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "IsGitRepo")
	return f.isRepo
}

func (f *Fake) DefaultBranch() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "DefaultBranch")
	return f.defaultBr, nil
}

func (f *Fake) BranchExists(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "BranchExists:"+name)
	return f.branches[name], nil
}

func (f *Fake) CreateWorktree(opts WorktreeOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "CreateWorktree:"+opts.Path)
	if _, exists := f.worktrees[opts.Path]; exists {
		return fmt.Errorf("%w: %s", ErrPathExists, opts.Path)
	}
	for _, wt := range f.worktrees {
		if wt.Branch == opts.Branch {
			return fmt.Errorf("%w: %s", ErrBranchCheckedOut, opts.Branch)
		}
	}
	f.worktrees[opts.Path] = Worktree{Path: opts.Path, Branch: opts.Branch}
	f.branches[opts.Branch] = true
	return nil
}

func (f *Fake) RemoveWorktree(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "RemoveWorktree:"+path)
	if _, exists := f.worktrees[path]; !exists {
		return fmt.Errorf("worktree %q not found", path)
	}
	delete(f.worktrees, path)
	return nil
}

func (f *Fake) ListWorktrees() ([]Worktree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "ListWorktrees")
	out := []Worktree{{Path: f.repoRoot, Branch: f.defaultBr, IsMain: true}}
	for _, wt := range f.worktrees {
		out = append(out, wt)
	}
	return out, nil
}

func (f *Fake) Status(cwd string) (StatusResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "Status:"+cwd)
	if f.StatusFunc != nil {
		return f.StatusFunc(cwd)
	}
	return StatusResult{}, nil
}

func (f *Fake) DeleteBranch(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "DeleteBranch:"+name)
	if name == "main" || name == "master" {
		return fmt.Errorf("%w: %s", ErrCannotRemoveMain, name)
	}
	delete(f.branches, name)
	return nil
}
