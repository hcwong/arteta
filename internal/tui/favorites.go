package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// FavoritesPicker is the modal list of saved project paths.
type FavoritesPicker struct {
	Paths    []string
	Cursor   int
	fromForm bool // true = opened from the create form; only fills Cwd on select
}

func newFavoritesPicker(paths []string, fromForm bool) FavoritesPicker {
	return FavoritesPicker{Paths: paths, fromForm: fromForm}
}

// Update handles keyboard input for the picker.
// Returns the updated picker, the selected path (if enter was pressed),
// the index to delete (if d was pressed, else -1), and whether esc was pressed.
func (p FavoritesPicker) Update(msg tea.KeyMsg) (FavoritesPicker, string, int, bool) {
	switch msg.String() {
	case "j", "down":
		if p.Cursor < len(p.Paths)-1 {
			p.Cursor++
		}
	case "k", "up":
		if p.Cursor > 0 {
			p.Cursor--
		}
	case "enter":
		if len(p.Paths) > 0 {
			return p, p.Paths[p.Cursor], -1, false
		}
	case "d":
		if len(p.Paths) > 0 {
			return p, "", p.Cursor, false
		}
	case "esc":
		return p, "", -1, true
	}
	return p, "", -1, false
}

func (p FavoritesPicker) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Favourite paths"))
	b.WriteString("\n\n")

	if len(p.Paths) == 0 {
		b.WriteString(dimStyle.Render("No favourites yet. Press F on a workflow to add one."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("[Esc] back"))
		return modalStyle.Render(b.String())
	}

	home, _ := os.UserHomeDir()
	for i, path := range p.Paths {
		selected := i == p.Cursor
		cursor := "  "
		if selected {
			cursor = cursorStyle.Render("▶ ")
		}
		name := filepath.Base(path)
		display := path
		if home != "" {
			display = strings.Replace(path, home, "~", 1)
		}
		nameStr := name
		if selected {
			nameStr = selectedStyle.Render(name)
		} else {
			nameStr = titleStyle.Render(name)
		}
		b.WriteString(cursor + nameStr + "  " + dimStyle.Render(display))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[j/k] navigate   [⏎] select   [d] delete   [Esc] back"))
	return modalStyle.Render(b.String())
}
