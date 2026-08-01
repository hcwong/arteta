package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcwong/arteta/internal/git"
	"github.com/hcwong/arteta/internal/workflow"
)

// CreateForm is the new-workflow modal state.
type CreateForm struct {
	NameInput      textinput.Model
	CwdInput       textinput.Model
	Layout         workflow.Layout
	Focus          int // 0=name, 1=cwd, 2=layout-radio, 3=branch (worktree mode)
	Err            string
	OpenPicker     bool // set when user presses Tab on cwd field to request filepicker
	WorktreeMode   bool
	WorktreeRepo   string
	AttachWorktree string
	BranchInput    textinput.Model
	ShowBranch     bool // true when "Create new worktree" was selected
}

// allLayouts is the order layouts appear in the radio.
var allLayouts = []workflow.Layout{
	workflow.LayoutSingle,
	workflow.LayoutVSplit,
	workflow.LayoutHSplit,
	workflow.LayoutQuad,
}

func newCreateForm(defaultCwd string) CreateForm {
	name := textinput.New()
	name.Placeholder = "auth-refactor"
	name.Focus()
	name.CharLimit = 64
	name.Width = 40

	cwd := textinput.New()
	cwd.SetValue(defaultCwd)
	cwd.CharLimit = 256
	cwd.Width = 40

	branch := textinput.New()
	branch.Placeholder = "feature-branch"
	branch.CharLimit = 128
	branch.Width = 40

	return CreateForm{
		NameInput:   name,
		CwdInput:    cwd,
		BranchInput: branch,
		Layout:      workflow.LayoutVSplit,
		Focus:       0,
	}
}

// Update routes key events for the create form. Returns the form, a
// command to run, and a "done" tuple if the user confirmed:
//
//	(form, cmd, submitted, cancelled)
func (f CreateForm) Update(msg tea.Msg) (CreateForm, tea.Cmd, bool, bool) {
	fieldCount := f.fieldCount()
	submitField := f.submitFieldIndex()

	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return f, nil, false, true
		case "tab":
			if f.Focus == 1 {
				f.OpenPicker = true
				return f, nil, false, false
			}
			f.Focus = (f.Focus + 1) % fieldCount
			f.applyFocus()
			return f, nil, false, false
		case "down":
			f.Focus = (f.Focus + 1) % fieldCount
			f.applyFocus()
			return f, nil, false, false
		case "shift+tab", "up":
			f.Focus = (f.Focus + fieldCount - 1) % fieldCount
			f.applyFocus()
			return f, nil, false, false
		case "enter":
			if f.Focus < submitField {
				f.Focus = (f.Focus + 1) % fieldCount
				f.applyFocus()
				return f, nil, false, false
			}
			if err := workflow.ValidateName(strings.TrimSpace(f.NameInput.Value())); err != nil {
				f.Err = err.Error()
				return f, nil, false, false
			}
			if strings.TrimSpace(f.CwdInput.Value()) == "" {
				f.Err = "cwd is required"
				return f, nil, false, false
			}
			return f, nil, true, false
		case "left":
			if f.Focus == 2 {
				f.Layout = prevLayout(f.Layout)
				return f, nil, false, false
			}
		case "right":
			if f.Focus == 2 {
				f.Layout = nextLayout(f.Layout)
				return f, nil, false, false
			}
		}
	}

	var cmd tea.Cmd
	switch f.Focus {
	case 0:
		f.NameInput, cmd = f.NameInput.Update(msg)
	case 1:
		f.CwdInput, cmd = f.CwdInput.Update(msg)
	case 3:
		f.BranchInput, cmd = f.BranchInput.Update(msg)
	}
	return f, cmd, false, false
}

func (f CreateForm) fieldCount() int {
	if f.ShowBranch {
		return 4 // name, cwd, layout, branch
	}
	return 3 // name, cwd, layout
}

func (f CreateForm) submitFieldIndex() int {
	if f.ShowBranch {
		return 3
	}
	return 2
}

func (f *CreateForm) applyFocus() {
	if f.Focus == 0 {
		f.NameInput.Focus()
	} else {
		f.NameInput.Blur()
	}
	if f.Focus == 1 {
		f.CwdInput.Focus()
	} else {
		f.CwdInput.Blur()
	}
	if f.Focus == 3 {
		f.BranchInput.Focus()
	} else {
		f.BranchInput.Blur()
	}
}

func (f CreateForm) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New workflow"))
	b.WriteString("\n\n")

	b.WriteString(label("Name", f.Focus == 0))
	b.WriteString(f.NameInput.View())
	b.WriteString("\n\n")

	b.WriteString(label("Cwd ", f.Focus == 1))
	b.WriteString(f.CwdInput.View())
	b.WriteString("\n\n")

	b.WriteString(label("Layout", f.Focus == 2))
	b.WriteString("\n  ")
	for _, l := range allLayouts {
		marker := " "
		if l == f.Layout {
			marker = "•"
		}
		entry := "(" + marker + ") " + string(l)
		if f.Focus == 2 && l == f.Layout {
			entry = selectedStyle.Render(entry)
		}
		b.WriteString(entry + "  ")
	}
	b.WriteString("\n\n")

	if f.ShowBranch {
		b.WriteString(label("Branch", f.Focus == 3))
		b.WriteString(f.BranchInput.View())
		b.WriteString("\n\n")
	}

	if f.WorktreeMode {
		b.WriteString(dimStyle.Render("  worktree mode") + "\n\n")
	}

	if f.Err != "" {
		b.WriteString(errorStyle.Render(f.Err) + "\n\n")
	}

	hint := "[Tab/↑↓] field   [←→] layout   [⏎] create   [Esc] cancel"
	if f.Focus == 1 {
		hint = "[Tab] browse dirs   [ctrl+f] favourites   [↑↓] field   [⏎] create   [Esc] cancel"
	}
	b.WriteString(helpStyle.Render(hint))

	return modalStyle.Render(b.String())
}

func label(name string, focused bool) string {
	style := dimStyle
	if focused {
		style = selectedStyle
	}
	return style.Render(name+": ") + "  "
}

func nextLayout(l workflow.Layout) workflow.Layout {
	for i, x := range allLayouts {
		if x == l {
			return allLayouts[(i+1)%len(allLayouts)]
		}
	}
	return allLayouts[0]
}

func prevLayout(l workflow.Layout) workflow.Layout {
	for i, x := range allLayouts {
		if x == l {
			return allLayouts[(i+len(allLayouts)-1)%len(allLayouts)]
		}
	}
	return allLayouts[0]
}

// newCreateFormFromFavorite builds a create form pre-filled with a favourite
// path. The Cwd is set to path; the Name field is pre-populated with a
// suggested name derived from the path's base directory. Focus starts on the
// Name field so the user can accept or change it.
func newCreateFormFromFavorite(path string) CreateForm {
	f := newCreateForm(path)
	f.NameInput.SetValue(suggestName(path))
	return f
}

// suggestName derives a workflow name from a directory path by taking the
// base name and stripping characters that workflow.ValidateName rejects.
func suggestName(path string) string {
	base := filepath.Base(path)
	var b strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == '.' || r == ' ' {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-_")
	if err := workflow.ValidateName(result); err != nil {
		return ""
	}
	return result
}

// Center returns s centred in a w×h area for modal presentation.
func center(content string, w, h int) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}

// WorktreeOption is a single item in the worktree picker.
type WorktreeOption struct {
	Label    string
	Path     string
	IsCreate bool
	IsSkip   bool
}

// WorktreePicker lets the user choose between creating a new worktree,
// attaching to an existing one, or skipping worktree mode entirely.
type WorktreePicker struct {
	Items  []WorktreeOption
	Cursor int
}

func newWorktreePicker(worktrees []git.Worktree) WorktreePicker {
	items := []WorktreeOption{
		{Label: "Create new worktree", IsCreate: true},
	}
	for _, wt := range worktrees {
		items = append(items, WorktreeOption{
			Label: wt.Branch + " (" + wt.Path + ")",
			Path:  wt.Path,
		})
	}
	items = append(items, WorktreeOption{Label: "No worktree (use project dir)", IsSkip: true})
	return WorktreePicker{Items: items}
}

// Update handles key events. Returns (picker, selectedOption, cancelled).
func (p WorktreePicker) Update(msg tea.KeyMsg) (WorktreePicker, *WorktreeOption, bool) {
	switch msg.String() {
	case "esc":
		return p, nil, true
	case "j", "down":
		if p.Cursor < len(p.Items)-1 {
			p.Cursor++
		}
	case "k", "up":
		if p.Cursor > 0 {
			p.Cursor--
		}
	case "enter":
		return p, &p.Items[p.Cursor], false
	}
	return p, nil, false
}

func (p WorktreePicker) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Worktree"))
	b.WriteString("\n\n")

	for i, item := range p.Items {
		cursor := "  "
		if i == p.Cursor {
			cursor = cursorStyle.Render("▶ ")
		}
		name := item.Label
		if i == p.Cursor {
			name = selectedStyle.Render(name)
		}
		b.WriteString(cursor + name + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[j/k] move   [⏎] select   [Esc] cancel"))
	return modalStyle.Render(b.String())
}
