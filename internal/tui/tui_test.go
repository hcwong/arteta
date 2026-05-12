package tui

import (
	"sort"
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
	// Tab on cwd field (Focus==1) sets OpenPicker instead of advancing focus.
	f, _, _, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !f.OpenPicker {
		t.Errorf("tab on cwd field should set OpenPicker")
	}
	// Down still cycles focus normally.
	f.OpenPicker = false
	f, _, _, _ = f.Update(tea.KeyMsg{Type: tea.KeyDown})
	if f.Focus != 2 {
		t.Errorf("down on cwd: focus=%d, want 2", f.Focus)
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

func TestFilePicker_SpaceSelectsCurrentDir(t *testing.T) {
	m, _, _ := newTestModel(t)
	m = sendKey(m, "n")
	m = sendNamedKey(m, tea.KeyTab) // name -> cwd
	m = sendNamedKey(m, tea.KeyTab) // cwd -> picker
	if m.mode != ModeFilePicker {
		t.Fatalf("expected ModeFilePicker, got mode=%d", m.mode)
	}
	// Space selects the current directory the picker is showing.
	m = sendKey(m, " ")
	if m.mode != ModeCreate {
		t.Errorf("expected ModeCreate after space, got mode=%d", m.mode)
	}
	if m.create.Focus != 1 {
		t.Errorf("expected Focus==1 after space, got %d", m.create.Focus)
	}
	if m.create.CwdInput.Value() == "" {
		t.Error("expected cwd to be populated after space selection")
	}
}

// ---------------------------------------------------------------------------
// sectionOf
// ---------------------------------------------------------------------------

func TestSectionOf(t *testing.T) {
	tests := []struct {
		name          string
		item          DisplayItem
		wantPriority  int
		wantLabel     string
	}{
		{
			name:         "pinned",
			item:         DisplayItem{Pinned: true},
			wantPriority: 0,
			wantLabel:    "pinned",
		},
		{
			name:         "dormant",
			item:         DisplayItem{Dormant: true},
			wantPriority: 4,
			wantLabel:    "dormant",
		},
		{
			name:         "awaiting input",
			item:         DisplayItem{Status: workflow.Status{LastEvent: "Notification"}},
			wantPriority: 1,
			wantLabel:    "awaiting input",
		},
		{
			name:         "running",
			item:         DisplayItem{Status: workflow.Status{LastEvent: "UserPromptSubmit"}},
			wantPriority: 2,
			wantLabel:    "running",
		},
		{
			name:         "idle via Stop event",
			item:         DisplayItem{Status: workflow.Status{LastEvent: "Stop"}},
			wantPriority: 3,
			wantLabel:    "idle",
		},
		{
			name:         "idle via empty LastEvent (StateUnknown)",
			item:         DisplayItem{Status: workflow.Status{}},
			wantPriority: 3,
			wantLabel:    "idle",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPriority, gotLabel := sectionOf(tc.item)
			if gotPriority != tc.wantPriority || gotLabel != tc.wantLabel {
				t.Errorf("sectionOf(%+v) = (%d, %q), want (%d, %q)",
					tc.item, gotPriority, gotLabel, tc.wantPriority, tc.wantLabel)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Sort order via sectionOf comparator
// ---------------------------------------------------------------------------

func TestSortOrder_SectionOf(t *testing.T) {
	// Build items in reverse priority order.
	items := []DisplayItem{
		{Workflow: workflow.Workflow{Name: "d"}, Dormant: true},                                // priority 4
		{Workflow: workflow.Workflow{Name: "i"}, Status: workflow.Status{LastEvent: "Stop"}},   // priority 3
		{Workflow: workflow.Workflow{Name: "r"}, Status: workflow.Status{LastEvent: "UserPromptSubmit"}}, // priority 2
		{Workflow: workflow.Workflow{Name: "a"}, Status: workflow.Status{LastEvent: "Notification"}},     // priority 1
		{Workflow: workflow.Workflow{Name: "p"}, Pinned: true},                                 // priority 0
	}

	sort.SliceStable(items, func(i, j int) bool {
		pi, _ := sectionOf(items[i])
		pj, _ := sectionOf(items[j])
		return pi < pj
	})

	wantOrder := []string{"p", "a", "r", "i", "d"}
	for idx, want := range wantOrder {
		if items[idx].Workflow.Name != want {
			t.Errorf("items[%d].Name = %q, want %q (full order: %v)",
				idx, items[idx].Workflow.Name, want, itemNames(items))
		}
	}
}

func itemNames(items []DisplayItem) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Workflow.Name
	}
	return names
}

// ---------------------------------------------------------------------------
// renderListBody section headers
// ---------------------------------------------------------------------------

func TestRenderListBody_SectionHeaders(t *testing.T) {
	m, _, _ := newTestModel(t)

	// Items spanning three different sections: awaiting input, running, idle.
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "a1"}, Status: workflow.Status{LastEvent: "Notification"}},
		{Workflow: workflow.Workflow{Name: "r1"}, Status: workflow.Status{LastEvent: "UserPromptSubmit"}},
		{Workflow: workflow.Workflow{Name: "i1"}, Status: workflow.Status{LastEvent: "Stop"}},
	}})
	m = updated.(Model)

	body := m.renderListBody()
	for _, hdr := range []string{"─ awaiting input", "─ running", "─ idle"} {
		if !strings.Contains(body, hdr) {
			t.Errorf("expected header %q in body; got:\n%s", hdr, body)
		}
	}
}

func TestRenderListBody_EmptySectionProducesNoHeader(t *testing.T) {
	m, _, _ := newTestModel(t)

	// Only awaiting-input and idle items — no running items.
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "a1"}, Status: workflow.Status{LastEvent: "Notification"}},
		{Workflow: workflow.Workflow{Name: "i1"}, Status: workflow.Status{LastEvent: "Stop"}},
	}})
	m = updated.(Model)

	body := m.renderListBody()
	if strings.Contains(body, "─ running") {
		t.Errorf("unexpected '─ running' header when no running items; got:\n%s", body)
	}
	if !strings.Contains(body, "─ awaiting input") {
		t.Errorf("expected '─ awaiting input' header; got:\n%s", body)
	}
	if !strings.Contains(body, "─ idle") {
		t.Errorf("expected '─ idle' header; got:\n%s", body)
	}
}

func TestRenderListBody_SingleSectionHasOneHeader(t *testing.T) {
	m, _, _ := newTestModel(t)

	// All items are idle — only one "idle" header should appear.
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "i1"}, Status: workflow.Status{LastEvent: "Stop"}},
		{Workflow: workflow.Workflow{Name: "i2"}, Status: workflow.Status{LastEvent: "Stop"}},
		{Workflow: workflow.Workflow{Name: "i3"}, Status: workflow.Status{LastEvent: "Stop"}},
	}})
	m = updated.(Model)

	body := m.renderListBody()
	count := strings.Count(body, "─ idle")
	if count != 1 {
		t.Errorf("expected exactly 1 '─ idle' header, got %d; body:\n%s", count, body)
	}
}

// ---------------------------------------------------------------------------
// Cursor name preservation after workflowsLoadedMsg
// ---------------------------------------------------------------------------

func TestCursorName_RestoredAfterReload(t *testing.T) {
	m, _, _ := newTestModel(t)

	// Load three items; cursor on "beta" (index 1).
	updated, _ := m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha"}},
		{Workflow: workflow.Workflow{Name: "beta"}},
		{Workflow: workflow.Workflow{Name: "gamma"}},
	}})
	m = updated.(Model)
	m = sendKey(m, "j") // cursor -> 1 ("beta"), sets cursorName = "beta"
	if m.cursor != 1 || m.cursorName != "beta" {
		t.Fatalf("precondition: cursor=%d cursorName=%q, want 1/beta", m.cursor, m.cursorName)
	}

	// Reload with items in a different order; "beta" is now at index 2.
	updated, _ = m.Update(workflowsLoadedMsg{items: []DisplayItem{
		{Workflow: workflow.Workflow{Name: "alpha"}},
		{Workflow: workflow.Workflow{Name: "gamma"}},
		{Workflow: workflow.Workflow{Name: "beta"}},
	}})
	m = updated.(Model)

	if m.cursor != 2 {
		t.Errorf("cursor = %d after reload, want 2 (index of 'beta')", m.cursor)
	}
	if m.items[m.cursor].Workflow.Name != "beta" {
		t.Errorf("cursor points to %q, want 'beta'", m.items[m.cursor].Workflow.Name)
	}
}
