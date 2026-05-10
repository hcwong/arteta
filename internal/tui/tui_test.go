package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

func newTestModel(t *testing.T) (Model, *service.Service, *store.Store) {
	t.Helper()
	st := store.New(t.TempDir())
	svc := &service.Service{
		Store: st,
		Tmux:  tmux.NewFake(),
		Term:  terminal.NewFake(),
		Now:   func() time.Time { return time.Date(2026, 5, 9, 17, 0, 0, 0, time.UTC) },
	}
	return New(st, svc, "/tmp/repo"), svc, st
}

func sendKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model)
}

func sendNamedKey(m Model, k tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	return updated.(Model)
}

func TestEmptyHomepageRenders(t *testing.T) {
	m, _, _ := newTestModel(t)
	view := m.View()
	if !strings.Contains(view, "No workflows yet") {
		t.Errorf("empty homepage should hint to press n; got:\n%s", view)
	}
}

func TestLoadWorkflows_PopulatesItems(t *testing.T) {
	m, _, _ := newTestModel(t)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha"}},
		{Workflow: workflow.Workflow{Name: "beta"}},
	}})
	m = updated.(Model)
	view := m.View()
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in view, got:\n%s", want, view)
		}
	}
}

func TestJK_MovesCursor(t *testing.T) {
	m, _, _ := newTestModel(t)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "a"}},
		{Workflow: workflow.Workflow{Name: "b"}},
		{Workflow: workflow.Workflow{Name: "c"}},
	}})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("initial cursor: %d", m.cursor)
	}
	m = sendKey(m, "j")
	m = sendKey(m, "j")
	if m.cursor != 2 {
		t.Errorf("after jj: %d, want 2", m.cursor)
	}
	m = sendKey(m, "k")
	if m.cursor != 1 {
		t.Errorf("after k: %d, want 1", m.cursor)
	}
	// Don't go below 0.
	m = sendKey(m, "k")
	m = sendKey(m, "k")
	if m.cursor != 0 {
		t.Errorf("clamped cursor: %d, want 0", m.cursor)
	}
	// Don't go beyond last.
	for i := 0; i < 10; i++ {
		m = sendKey(m, "j")
	}
	if m.cursor != 2 {
		t.Errorf("cursor at bottom: %d, want 2", m.cursor)
	}
}

func TestN_OpensCreateForm(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = sendKey(m, "n")
	if m.mode != ModeCreate {
		t.Errorf("after n: mode=%d, want ModeCreate", m.mode)
	}
	view := m.View()
	if !strings.Contains(view, "New workflow") {
		t.Errorf("create form should render title; got:\n%s", view)
	}
}

func TestQuestion_OpensHelp(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = sendKey(m, "?")
	if m.mode != ModeHelp {
		t.Errorf("after ?: mode=%d, want ModeHelp", m.mode)
	}
	if !strings.Contains(m.View(), "keybindings") {
		t.Errorf("help should list keybindings; got:\n%s", m.View())
	}
}

func TestD_OpensConfirm_ThenY_TriggersClose(t *testing.T) {
	m, svc, st := newTestModel(t)
	if _, err := svc.Create(service.CreateOpts{
		Name:   "doomed",
		Cwd:    "/r",
		Layout: workflow.LayoutSingle,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "doomed", TmuxSession: "arteta-doomed"}},
	}})
	m = updated.(Model)

	m = sendKey(m, "D")
	if m.mode != ModeConfirmDelete {
		t.Fatalf("after D: mode=%d, want ModeConfirmDelete", m.mode)
	}
	if !strings.Contains(m.View(), "doomed") {
		t.Errorf("confirm modal should reference workflow name")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected closeCmd to be returned, got nil")
	}
	// Run the cmd synchronously and feed its msg back.
	msg := cmd()
	if _, ok := msg.(closeDoneMsg); !ok {
		t.Errorf("expected closeDoneMsg, got %T", msg)
	}
	if _, err := st.LoadWorkflow("doomed"); err == nil {
		t.Error("workflow still exists after close")
	}
}

func TestD_OpensConfirm_ThenN_Cancels(t *testing.T) {
	m, _, _ := newTestModel(t)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "wf"}},
	}})
	m = updated.(Model)
	m = sendKey(m, "D")
	if m.mode != ModeConfirmDelete {
		t.Fatal("expected ModeConfirmDelete")
	}
	m = sendKey(m, "n")
	if m.mode != ModeList {
		t.Errorf("after n: mode=%d, want ModeList", m.mode)
	}
}

func TestQ_QuitsViaCmd(t *testing.T) {
	m, _, _ := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should return tea.Quit")
	}
	if msg := cmd(); msg == nil {
		t.Error("tea.Quit cmd should produce a msg")
	}
}

func TestEnter_Dormant_TriggersRevive(t *testing.T) {
	m, _, _ := newTestModel(t)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "ghost", TmuxSession: "arteta-ghost"}, Dormant: true},
	}})
	m = updated.(Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on dormant should return reviveCmd")
	}
}

func TestCreateForm_CycleFocusAndLayout(t *testing.T) {
	f := newCreateForm("/cwd")
	if f.Focus != 0 {
		t.Errorf("initial focus: %d, want 0", f.Focus)
	}
	f, _, _, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if f.Focus != 1 {
		t.Errorf("after tab: %d, want 1", f.Focus)
	}
	f, _, _, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if f.Focus != 2 {
		t.Errorf("after tab tab: %d, want 2", f.Focus)
	}
	// On layout row, right cycles forward.
	want := nextLayout(f.Layout)
	f, _, _, _ = f.Update(tea.KeyMsg{Type: tea.KeyRight})
	if f.Layout != want {
		t.Errorf("right key did not advance layout")
	}
}

func TestCreateForm_SubmitWithoutName_ShowsError(t *testing.T) {
	f := newCreateForm("/cwd")
	// Move focus to layout row so Enter means submit.
	f.Focus = 2
	f.applyFocus()
	f, _, submitted, _ := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitted {
		t.Error("submit should fail with empty name")
	}
	if f.Err == "" {
		t.Error("expected error message for empty name")
	}
}

// withSize returns m updated to have the given terminal dimensions.
func withSize(m Model, w, h int) Model {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model)
}

func TestPreview_HiddenBelowThreshold(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = withSize(m, 120, 40)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha", TmuxSession: "arteta-alpha"}},
	}})
	m = updated.(Model)
	// Prime preview content; it should not appear because width < 140.
	updated, _ = m.Update(previewMsg{name: "alpha", content: "secret-payload"})
	m = updated.(Model)
	view := m.View()
	if strings.Contains(view, "secret-payload") {
		t.Errorf("preview rendered below threshold:\n%s", view)
	}
}

func TestPreview_RendersForSelected(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = withSize(m, 160, 40)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha", TmuxSession: "arteta-alpha"}},
		{Workflow: workflow.Workflow{Name: "beta", TmuxSession: "arteta-beta"}},
	}})
	m = updated.(Model)
	updated, _ = m.Update(previewMsg{name: "alpha", content: "alpha-line-1\nalpha-line-2"})
	m = updated.(Model)
	updated, _ = m.Update(previewMsg{name: "beta", content: "beta-only-content"})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "alpha-line-1") || !strings.Contains(view, "alpha-line-2") {
		t.Errorf("expected alpha preview lines in view; got:\n%s", view)
	}
	if strings.Contains(view, "beta-only-content") {
		t.Errorf("non-selected beta content leaked into view:\n%s", view)
	}
}

func TestPreview_DormantPlaceholder(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = withSize(m, 160, 40)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "ghost", TmuxSession: "arteta-ghost"}, Dormant: true},
	}})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "dormant") {
		t.Errorf("expected dormant placeholder in preview; got:\n%s", view)
	}
}

func TestPreview_CursorMoveTriggersCapture(t *testing.T) {
	m, svc, _ := newTestModel(t)
	m = withSize(m, 160, 40)
	// Seed the fake tmux with two sessions so capture-pane succeeds for both.
	fake := svc.Tmux.(*tmux.Fake)
	if err := fake.NewSession(tmux.NewSessionOpts{Name: "arteta-alpha"}); err != nil {
		t.Fatalf("fake NewSession alpha: %v", err)
	}
	if err := fake.NewSession(tmux.NewSessionOpts{Name: "arteta-beta"}); err != nil {
		t.Fatalf("fake NewSession beta: %v", err)
	}
	fake.SetPaneOutput("arteta-beta", "from-beta-pane")

	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha", TmuxSession: "arteta-alpha"}},
		{Workflow: workflow.Workflow{Name: "beta", TmuxSession: "arteta-beta"}},
	}})
	m = updated.(Model)

	// Move cursor to beta — should return a capture cmd.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("cursor move did not return capture cmd")
	}
	msg := cmd()
	pm, ok := msg.(previewMsg)
	if !ok {
		t.Fatalf("expected previewMsg, got %T", msg)
	}
	if pm.name != "beta" || pm.content != "from-beta-pane" {
		t.Errorf("previewMsg mismatch: %+v", pm)
	}
}

func TestPreview_KeepsLastGoodOnError(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = withSize(m, 160, 40)
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha", TmuxSession: "arteta-alpha"}},
	}})
	m = updated.(Model)
	// First success — primes last-good.
	updated, _ = m.Update(previewMsg{name: "alpha", content: "good-frame"})
	m = updated.(Model)
	// Now an error — the last-good frame should still render.
	updated, _ = m.Update(previewMsg{name: "alpha", err: tmuxErr("boom")})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "good-frame") {
		t.Errorf("last-good frame disappeared after error:\n%s", view)
	}
}

// tmuxErr is a small typed error used in preview-error tests.
type tmuxErr string

func (e tmuxErr) Error() string { return string(e) }

func TestCreateForm_Submit_Succeeds(t *testing.T) {
	f := newCreateForm("/cwd")
	f.NameInput.SetValue("good-name")
	f.Focus = 2
	f.applyFocus()
	_, _, submitted, _ := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !submitted {
		t.Error("expected submit=true with valid name + cwd")
	}
}

func TestCwdTab_OpensPicker(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = sendKey(m, "n") // enter ModeCreate
	// Tab once: name -> cwd
	m = sendNamedKey(m, tea.KeyTab)
	if m.create.Focus != 1 {
		t.Fatalf("expected Focus==1 after first tab, got %d", m.create.Focus)
	}
	// Tab again: cwd -> filepicker
	m = sendNamedKey(m, tea.KeyTab)
	if m.mode != ModeFilePicker {
		t.Errorf("expected ModeFilePicker after tab on cwd, got mode=%d", m.mode)
	}
}

func TestFilePicker_EscReturnsToCwd(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = sendKey(m, "n")
	m = sendNamedKey(m, tea.KeyTab) // name -> cwd
	m = sendNamedKey(m, tea.KeyTab) // cwd -> picker
	if m.mode != ModeFilePicker {
		t.Fatalf("expected ModeFilePicker, got mode=%d", m.mode)
	}
	m = sendNamedKey(m, tea.KeyEsc)
	if m.mode != ModeCreate {
		t.Errorf("expected ModeCreate after Esc, got mode=%d", m.mode)
	}
	if m.create.Focus != 1 {
		t.Errorf("expected Focus==1 after Esc, got %d", m.create.Focus)
	}
}
