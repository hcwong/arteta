package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initBareRepo creates a bare repo + a clone with one commit so that
// origin/HEAD exists and worktree operations have a valid base.
func initBareRepo(t *testing.T) (cloneDir string) {
	t.Helper()
	tmp := t.TempDir()

	bare := filepath.Join(tmp, "origin.git")
	run(t, tmp, "git", "init", "--bare", bare)

	clone := filepath.Join(tmp, "clone")
	run(t, tmp, "git", "clone", bare, clone)
	run(t, clone, "git", "config", "user.email", "test@test.com")
	run(t, clone, "git", "config", "user.name", "Test")
	writeFile(t, filepath.Join(clone, "README.md"), "init")
	run(t, clone, "git", "add", ".")
	run(t, clone, "git", "commit", "-m", "initial")
	run(t, clone, "git", "push", "origin", "HEAD")

	return clone
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepoRoot(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	root, err := c.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	// Resolve symlinks (macOS /tmp → /private/tmp).
	wantAbs, _ := filepath.EvalSymlinks(clone)
	gotAbs, _ := filepath.EvalSymlinks(root)
	if gotAbs != wantAbs {
		t.Errorf("RepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestRepoRoot_NotARepo(t *testing.T) {
	c := NewReal(t.TempDir())
	_, err := c.RepoRoot()
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
}

func TestDefaultBranch(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	branch, err := c.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" && branch != "master" {
		t.Errorf("DefaultBranch = %q, want main or master", branch)
	}
}

func TestIsGitRepo(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	if !c.IsGitRepo() {
		t.Error("IsGitRepo returned false for a git repo")
	}

	c2 := NewReal(t.TempDir())
	if c2.IsGitRepo() {
		t.Error("IsGitRepo returned true for a non-repo dir")
	}
}

func TestCreateWorktree_And_ListWorktrees(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	wtPath := filepath.Join(t.TempDir(), "my-feature")

	err := c.CreateWorktree(WorktreeOpts{
		Path:   wtPath,
		Branch: "my-feature",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("worktree directory not created")
	}

	wts, err := c.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	found := false
	for _, wt := range wts {
		if wt.Branch == "my-feature" {
			found = true
			gotAbs, _ := filepath.EvalSymlinks(wt.Path)
			wantAbs, _ := filepath.EvalSymlinks(wtPath)
			if gotAbs != wantAbs {
				t.Errorf("worktree path = %q, want %q", gotAbs, wantAbs)
			}
		}
	}
	if !found {
		t.Errorf("branch my-feature not found in worktree list: %+v", wts)
	}
}

func TestCreateWorktree_Attach(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	// Create a branch first.
	run(t, clone, "git", "branch", "existing-branch")

	wtPath := filepath.Join(t.TempDir(), "existing")
	err := c.CreateWorktree(WorktreeOpts{
		Path:   wtPath,
		Branch: "existing-branch",
		Attach: true,
	})
	if err != nil {
		t.Fatalf("CreateWorktree attach: %v", err)
	}
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatal("worktree directory not created on attach")
	}
}

func TestCreateWorktree_PathExists(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	existing := t.TempDir() // already exists

	err := c.CreateWorktree(WorktreeOpts{
		Path:   existing,
		Branch: "feat",
	})
	if err == nil {
		t.Fatal("expected error when path exists")
	}
	if !isErrPathExists(err) {
		t.Errorf("expected ErrPathExists, got: %v", err)
	}
}

func isErrPathExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrPathExists.Error())
}

func TestCreateWorktree_CustomBase(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	// Create a second commit on a side branch to use as base.
	run(t, clone, "git", "checkout", "-b", "side")
	writeFile(t, filepath.Join(clone, "side.txt"), "side")
	run(t, clone, "git", "add", ".")
	run(t, clone, "git", "commit", "-m", "side commit")
	run(t, clone, "git", "checkout", "-") // back to default branch

	wtPath := filepath.Join(t.TempDir(), "from-side")
	err := c.CreateWorktree(WorktreeOpts{
		Path:   wtPath,
		Branch: "from-side",
		Base:   "side",
	})
	if err != nil {
		t.Fatalf("CreateWorktree with base: %v", err)
	}
	// The worktree should have the side.txt file from the side branch.
	if _, err := os.Stat(filepath.Join(wtPath, "side.txt")); os.IsNotExist(err) {
		t.Error("worktree missing side.txt — base ref not honoured")
	}
}

func TestRemoveWorktree(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	wtPath := filepath.Join(t.TempDir(), "to-remove")

	if err := c.CreateWorktree(WorktreeOpts{Path: wtPath, Branch: "to-remove"}); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if err := c.RemoveWorktree(wtPath); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after remove")
	}
}

func TestBranchExists(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	defBranch, _ := c.DefaultBranch()
	ok, err := c.BranchExists(defBranch)
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !ok {
		t.Error("BranchExists returned false for default branch")
	}

	ok, err = c.BranchExists("nonexistent-branch-xyz")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if ok {
		t.Error("BranchExists returned true for nonexistent branch")
	}
}

func TestDeleteBranch(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	run(t, clone, "git", "branch", "doomed")
	if err := c.DeleteBranch("doomed"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	ok, _ := c.BranchExists("doomed")
	if ok {
		t.Error("branch still exists after DeleteBranch")
	}
}

func TestDeleteBranch_RefusesMain(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	for _, name := range []string{"main", "master"} {
		if err := c.DeleteBranch(name); err == nil {
			t.Errorf("DeleteBranch(%q) should error", name)
		}
	}
}

func TestStatus_Clean(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	st, err := c.Status(clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Dirty {
		t.Error("expected clean status")
	}
}

func TestStatus_Dirty(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	writeFile(t, filepath.Join(clone, "dirty.txt"), "uncommitted")
	st, err := c.Status(clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Dirty {
		t.Error("expected dirty status")
	}
}

func TestStatus_Unpushed(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)

	writeFile(t, filepath.Join(clone, "new.txt"), "content")
	run(t, clone, "git", "add", ".")
	run(t, clone, "git", "commit", "-m", "unpushed")

	st, err := c.Status(clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.UnpushedCount != 1 {
		t.Errorf("UnpushedCount = %d, want 1", st.UnpushedCount)
	}
}

func TestListWorktrees_MainIsMarked(t *testing.T) {
	clone := initBareRepo(t)
	c := NewReal(clone)
	wts, err := c.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) == 0 {
		t.Fatal("expected at least 1 worktree (the main one)")
	}
	mainCount := 0
	for _, wt := range wts {
		if wt.IsMain {
			mainCount++
		}
	}
	if mainCount != 1 {
		t.Errorf("expected exactly 1 main worktree, got %d in %+v", mainCount, wts)
	}
}

func TestValidateBranchName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"valid-branch", false},
		{"feat/my-feature", false},
		{"123", false},
		{"", true},
		{".hidden", true},
		{"bad..branch", true},
		{"bad branch", true},
		{"bad~branch", true},
		{"bad^branch", true},
		{"bad:branch", true},
		{"bad\\branch", true},
		{"branch.lock", true},
		{"-starts-with-dash", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBranchName(tc.name)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateBranchName(%q) = nil, want error", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateBranchName(%q) = %v, want nil", tc.name, err)
			}
		})
	}
}
