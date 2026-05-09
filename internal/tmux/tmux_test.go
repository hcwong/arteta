package tmux

import (
	"testing"

	"github.com/hcwong/arteta/internal/workflow"
)

func TestFake_LifecycleHappyPath(t *testing.T) {
	f := NewFake()

	if ok, _ := f.HasSession("none"); ok {
		t.Error("HasSession on missing returned true")
	}

	err := f.NewSession(NewSessionOpts{Name: "wf1", Cwd: "/tmp", Cmd: "claude", Env: map[string]string{"ARTETA_WORKFLOW": "wf1"}})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	ok, _ := f.HasSession("wf1")
	if !ok {
		t.Error("HasSession after create returned false")
	}

	sessions, _ := f.ListSessions()
	if len(sessions) != 1 || sessions[0] != "wf1" {
		t.Errorf("ListSessions: got %v, want [wf1]", sessions)
	}

	cmds, _ := f.PaneCommands("wf1")
	if len(cmds) != 1 || cmds[0] != "claude" {
		t.Errorf("PaneCommands: got %v, want [claude]", cmds)
	}

	if err := f.KillSession("wf1"); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	ok, _ = f.HasSession("wf1")
	if ok {
		t.Error("HasSession after kill returned true")
	}
}

func TestFake_CapturePane(t *testing.T) {
	f := NewFake()
	if _, err := f.CapturePane("missing", 0); err == nil {
		t.Error("CapturePane on missing session: nil error, want error")
	}
	if err := f.NewSession(NewSessionOpts{Name: "wf", Cmd: "claude"}); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	got, err := f.CapturePane("wf", 0)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if got != "" {
		t.Errorf("CapturePane default: got %q, want empty", got)
	}
	f.SetPaneOutput("wf", "hello\nworld")
	got, err = f.CapturePane("wf", 0)
	if err != nil {
		t.Fatalf("CapturePane after SetPaneOutput: %v", err)
	}
	if got != "hello\nworld" {
		t.Errorf("CapturePane after SetPaneOutput: got %q, want %q", got, "hello\nworld")
	}
}

func TestFake_DoubleCreate_Errors(t *testing.T) {
	f := NewFake()
	if err := f.NewSession(NewSessionOpts{Name: "x"}); err != nil {
		t.Fatalf("first NewSession: %v", err)
	}
	if err := f.NewSession(NewSessionOpts{Name: "x"}); err == nil {
		t.Error("second NewSession returned nil, want error")
	}
}

func TestBuildLayout_Single(t *testing.T) {
	f := NewFake()
	err := BuildLayout(BuildOpts{
		Client:    f,
		Name:      "wf",
		Cwd:       "/repo",
		Layout:    workflow.LayoutSingle,
		ClaudeCmd: "claude",
		Env:       map[string]string{"ARTETA_WORKFLOW": "wf"},
	})
	if err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	s := f.Sessions()["wf"]
	if s == nil {
		t.Fatal("session not created")
	}
	if len(s.Panes) != 1 {
		t.Errorf("single: got %d panes, want 1", len(s.Panes))
	}
	if s.Panes[0].Cmd != "claude" {
		t.Errorf("pane 0 cmd: got %q, want %q", s.Panes[0].Cmd, "claude")
	}
	if s.Panes[0].Env["ARTETA_WORKFLOW"] != "wf" {
		t.Errorf("ARTETA_WORKFLOW not propagated to pane env")
	}
}

func TestBuildLayout_VSplit(t *testing.T) {
	f := NewFake()
	if err := BuildLayout(BuildOpts{Client: f, Name: "wf", Cwd: "/repo", Layout: workflow.LayoutVSplit, ClaudeCmd: "claude"}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	s := f.Sessions()["wf"]
	if len(s.Panes) != 2 {
		t.Errorf("vsplit: got %d panes, want 2", len(s.Panes))
	}
	if s.Panes[0].Cmd != "claude" {
		t.Errorf("pane 0 should be claude, got %q", s.Panes[0].Cmd)
	}
	// pane 1 is terminal (empty cmd → default shell)
	if s.Panes[1].Cmd != "" {
		t.Errorf("pane 1 should be terminal (empty cmd), got %q", s.Panes[1].Cmd)
	}
}

func TestBuildLayout_HSplit(t *testing.T) {
	f := NewFake()
	if err := BuildLayout(BuildOpts{Client: f, Name: "wf", Cwd: "/repo", Layout: workflow.LayoutHSplit, ClaudeCmd: "claude"}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	s := f.Sessions()["wf"]
	if len(s.Panes) != 2 {
		t.Errorf("hsplit: got %d panes, want 2", len(s.Panes))
	}
}

func TestBuildLayout_Quad(t *testing.T) {
	f := NewFake()
	if err := BuildLayout(BuildOpts{Client: f, Name: "wf", Cwd: "/repo", Layout: workflow.LayoutQuad, ClaudeCmd: "claude"}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	s := f.Sessions()["wf"]
	if len(s.Panes) != 4 {
		t.Errorf("quad: got %d panes, want 4", len(s.Panes))
	}
	wantCmds := []string{"claude", "", "nvim .", "git diff --color=always | less -R"}
	for i, want := range wantCmds {
		if s.Panes[i].Cmd != want {
			t.Errorf("pane %d cmd: got %q, want %q", i, s.Panes[i].Cmd, want)
		}
	}
	if s.Layout != "tiled" {
		t.Errorf("quad layout: got %q, want %q", s.Layout, "tiled")
	}
}

func TestBuildLayout_InvalidLayout(t *testing.T) {
	f := NewFake()
	err := BuildLayout(BuildOpts{Client: f, Name: "wf", Layout: workflow.Layout("nope")})
	if err == nil {
		t.Error("BuildLayout with invalid layout returned nil, want error")
	}
}
