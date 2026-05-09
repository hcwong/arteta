package reconcile

import (
	"testing"

	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

func wf(name string) workflow.Workflow {
	return workflow.Workflow{
		Name:        name,
		TmuxSession: workflow.TmuxSessionName(name),
	}
}

func TestReconcile_AllLive(t *testing.T) {
	f := tmux.NewFake()
	for _, n := range []string{"a", "b"} {
		_ = f.NewSession(tmux.NewSessionOpts{Name: workflow.TmuxSessionName(n)})
	}
	r, err := Reconcile([]workflow.Workflow{wf("a"), wf("b")}, f)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(r.Live) != 2 || len(r.Dormant) != 0 {
		t.Errorf("expected 2 live 0 dormant, got %d/%d", len(r.Live), len(r.Dormant))
	}
}

func TestReconcile_AllDormant(t *testing.T) {
	f := tmux.NewFake()
	r, err := Reconcile([]workflow.Workflow{wf("a"), wf("b")}, f)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(r.Dormant) != 2 || len(r.Live) != 0 {
		t.Errorf("expected 2 dormant 0 live, got %d/%d", len(r.Dormant), len(r.Live))
	}
}

func TestReconcile_Mixed(t *testing.T) {
	f := tmux.NewFake()
	_ = f.NewSession(tmux.NewSessionOpts{Name: workflow.TmuxSessionName("alive")})
	r, err := Reconcile([]workflow.Workflow{wf("alive"), wf("missing")}, f)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(r.Live) != 1 || r.Live[0].Name != "alive" {
		t.Errorf("Live: %+v", r.Live)
	}
	if len(r.Dormant) != 1 || r.Dormant[0].Name != "missing" {
		t.Errorf("Dormant: %+v", r.Dormant)
	}
}

func TestReconcile_EmptyInput(t *testing.T) {
	f := tmux.NewFake()
	r, err := Reconcile(nil, f)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(r.Live) != 0 || len(r.Dormant) != 0 {
		t.Errorf("expected empty result, got %+v", r)
	}
}
