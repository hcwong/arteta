package terminal

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ITerm drives iTerm2 via osascript AppleScript.
//
// The exposed surface is simple: each method materialises an AppleScript
// string and shells it to `osascript -e ...`. The AppleScript is generated
// by pure helper functions that are unit-testable; the actual osascript
// invocation is exercised by manual smoke tests during development.
type ITerm struct {
	// Run is the subprocess invoker. Override in tests to record calls
	// without actually shelling out.
	Run func(script string) (stdout string, err error)
}

// NewITerm returns an ITerm using the real osascript binary.
func NewITerm() *ITerm {
	return &ITerm{Run: runOsascript}
}

func runOsascript(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("osascript: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (i *ITerm) OpenTab(opts OpenOpts) (TabHandle, error) {
	script := openTabScript(opts)
	out, err := i.Run(script)
	if err != nil {
		return TabHandle{}, err
	}
	return parseTabHandle(out)
}

func (i *ITerm) FocusTab(h TabHandle) error {
	_, err := i.Run(focusTabScript(h))
	return err
}

func (i *ITerm) CloseTab(h TabHandle) error {
	_, err := i.Run(closeTabScript(h))
	return err
}

func (i *ITerm) TabExists(h TabHandle) (bool, error) {
	out, err := i.Run(tabExistsScript(h))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

// AppleScript template builders. Each escapes user-supplied strings so
// embedded quotes can't break out of the script. Keeping them separate
// makes the templates straightforward to test without iTerm running.

// openTabScript returns AppleScript that creates a new iTerm window or tab,
// runs the given command, and prints "<windowID>\t<tabID>" to stdout so the
// Go side can capture the handle.
func openTabScript(opts OpenOpts) string {
	cmd := escapeAS(opts.Command)
	title := escapeAS(opts.Title)
	// Always create a new tab in the current window if one exists; otherwise
	// create a new window. This matches the spec's "open Iterm tab" model.
	return `tell application "iTerm"
	activate
	if (count of windows) is 0 then
		set newWindow to (create window with default profile command "` + cmd + `")
		set targetTab to current tab of newWindow
		set windowID to id of newWindow
	else
		tell current window
			set targetTab to (create tab with default profile command "` + cmd + `")
		end tell
		set windowID to id of current window
	end if
	tell targetTab
		set name to "` + title + `"
		set tabID to id of it
	end tell
	return (windowID as string) & "	" & (tabID as string)
end tell`
}

func focusTabScript(h TabHandle) string {
	w := escapeAS(h.WindowID)
	tab := escapeAS(h.TabID)
	return `tell application "iTerm"
	activate
	tell window id ` + w + `
		select
		repeat with t in tabs
			if (id of t as string) is "` + tab + `" then
				tell t to select
				exit repeat
			end if
		end repeat
	end tell
end tell`
}

func closeTabScript(h TabHandle) string {
	w := escapeAS(h.WindowID)
	tab := escapeAS(h.TabID)
	return `tell application "iTerm"
	tell window id ` + w + `
		repeat with t in tabs
			if (id of t as string) is "` + tab + `" then
				close t
				exit repeat
			end if
		end repeat
	end tell
end tell`
}

func tabExistsScript(h TabHandle) string {
	w := escapeAS(h.WindowID)
	tab := escapeAS(h.TabID)
	return `tell application "iTerm"
	try
		tell window id ` + w + `
			repeat with t in tabs
				if (id of t as string) is "` + tab + `" then
					return "true"
				end if
			end repeat
		end tell
	on error
		return "false"
	end try
	return "false"
end tell`
}

// escapeAS escapes a string for embedding inside double-quoted AppleScript.
// AppleScript escapes via backslash, same as Go.
func escapeAS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// parseTabHandle parses the "<windowID>\t<tabID>" output from openTabScript.
func parseTabHandle(s string) (TabHandle, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "\t", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return TabHandle{}, fmt.Errorf("unexpected osascript output %q", s)
	}
	return TabHandle{WindowID: parts[0], TabID: parts[1]}, nil
}
