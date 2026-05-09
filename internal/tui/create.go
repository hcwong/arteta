package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcwong/arteta/internal/workflow"
)

// CreateForm is the new-workflow modal state.
type CreateForm struct {
	NameInput textinput.Model
	CwdInput  textinput.Model
	Layout    workflow.Layout
	Focus     int // 0=name, 1=cwd, 2=layout-radio
	Err       string
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

	return CreateForm{
		NameInput: name,
		CwdInput:  cwd,
		Layout:    workflow.LayoutVSplit,
		Focus:     0,
	}
}

// Update routes key events for the create form. Returns the form, a
// command to run, and a "done" tuple if the user confirmed:
//
//	(form, cmd, submitted, cancelled)
func (f CreateForm) Update(msg tea.Msg) (CreateForm, tea.Cmd, bool, bool) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return f, nil, false, true
		case "tab", "down":
			f.Focus = (f.Focus + 1) % 3
			f.applyFocus()
			return f, nil, false, false
		case "shift+tab", "up":
			f.Focus = (f.Focus + 2) % 3
			f.applyFocus()
			return f, nil, false, false
		case "enter":
			if f.Focus < 2 {
				// Treat enter as next-field unless we're on the layout row.
				f.Focus = (f.Focus + 1) % 3
				f.applyFocus()
				return f, nil, false, false
			}
			// On layout row, enter means submit.
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
	}
	return f, cmd, false, false
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

	if f.Err != "" {
		b.WriteString(errorStyle.Render(f.Err) + "\n\n")
	}

	hint := "[Tab/↑↓] field   [←→] layout   [⏎] create   [Esc] cancel"
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

// Center returns s centred in a w×h area for modal presentation.
func center(content string, w, h int) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}
