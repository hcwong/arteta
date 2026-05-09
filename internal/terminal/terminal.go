// Package terminal abstracts the terminal-emulator-specific operations
// Arteta needs (open tab, focus tab, close tab). The MVP ships an iTerm
// adapter driven via osascript; future adapters (Ghostty, Kitty) implement
// the same interface.
package terminal

// TabHandle uniquely identifies a tab so Arteta can focus or close it later.
type TabHandle struct {
	WindowID string `json:"window_id"`
	// TabID holds an iTerm session unique id (UUID), not a numeric tab id —
	// `id of tab` is unreliable across iTerm builds.
	TabID string `json:"tab_id"`
}

// IsZero reports whether the handle is uninitialised.
func (h TabHandle) IsZero() bool { return h.WindowID == "" && h.TabID == "" }

// OpenOpts configures a new tab.
type OpenOpts struct {
	// Title is shown in the tab strip. Used for iTerm's tab name.
	Title string
	// Command is the shell command to run in the new tab. Typically
	// "tmux -L arteta attach -t <workflow>".
	Command string
}

// Adapter is the boundary between Arteta and a terminal emulator.
type Adapter interface {
	OpenTab(opts OpenOpts) (TabHandle, error)
	FocusTab(h TabHandle) error
	CloseTab(h TabHandle) error
	TabExists(h TabHandle) (bool, error)
}
