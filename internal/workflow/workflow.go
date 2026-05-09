// Package workflow defines Arteta's core domain types and the state machine
// that maps Claude hook events to workflow states.
package workflow

import (
	"fmt"
	"regexp"
	"time"
)

// State is the externally visible status of a workflow's Claude session.
type State int

const (
	StateUnknown State = iota
	StateIdle
	StateRunning
	StateAwaitingInput
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateAwaitingInput:
		return "awaiting_input"
	default:
		return "unknown"
	}
}

// Event is a Claude Code hook event we care about.
type Event int

const (
	EventNone Event = iota
	EventStop
	EventNotification
	EventUserPromptSubmit
)

func (e Event) String() string {
	switch e {
	case EventStop:
		return "Stop"
	case EventNotification:
		return "Notification"
	case EventUserPromptSubmit:
		return "UserPromptSubmit"
	default:
		return ""
	}
}

func ParseEvent(s string) (Event, bool) {
	switch s {
	case "Stop":
		return EventStop, true
	case "Notification":
		return EventNotification, true
	case "UserPromptSubmit":
		return EventUserPromptSubmit, true
	default:
		return EventNone, false
	}
}

// DeriveState returns the workflow state implied by the latest hook event.
// See DECISIONS.md §9 for the transition table.
func DeriveState(e Event) State {
	switch e {
	case EventUserPromptSubmit:
		return StateRunning
	case EventNotification:
		return StateAwaitingInput
	case EventStop:
		return StateIdle
	default:
		return StateUnknown
	}
}

// Layout is the pane arrangement for a workflow's tmux session.
type Layout string

const (
	LayoutSingle Layout = "single"
	LayoutVSplit Layout = "vsplit"
	LayoutHSplit Layout = "hsplit"
	LayoutQuad   Layout = "quad"
)

func (l Layout) Valid() bool {
	switch l {
	case LayoutSingle, LayoutVSplit, LayoutHSplit, LayoutQuad:
		return true
	default:
		return false
	}
}

func ParseLayout(s string) (Layout, error) {
	l := Layout(s)
	if !l.Valid() {
		return "", fmt.Errorf("invalid layout %q (want: single|vsplit|hsplit|quad)", s)
	}
	return l, nil
}

// Workflow is the persisted shape of a single Arteta workflow.
type Workflow struct {
	Name            string    `json:"name"`
	Cwd             string    `json:"cwd"`
	TmuxSession     string    `json:"tmux_session"`
	ClaudeSessionID string    `json:"claude_session_id,omitempty"`
	GitBranch       string    `json:"git_branch,omitempty"`
	Layout          Layout    `json:"layout"`
	ITermTab        *ITermTab `json:"iterm_tab,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// ITermTab is the persisted iTerm window/tab pointer for fast focus on reopen.
type ITermTab struct {
	WindowID string `json:"window_id"`
	TabID    string `json:"tab_id"`
}

// Status is the latest hook-derived state for a workflow. Hook subprocesses
// own writes to this shape; Arteta reads via fsnotify. State is derived on
// demand from LastEvent rather than persisted, so the source of truth is
// always the raw event the hook saw.
type Status struct {
	LastEvent   string    `json:"last_event"`
	LastMessage string    `json:"last_message,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

// State derives the current State from the persisted LastEvent.
func (s Status) State() State {
	e, _ := ParseEvent(s.LastEvent)
	return DeriveState(e)
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

const maxNameLen = 64

// ValidateName enforces a character set safe for tmux session names and
// for use as a JSON filename component.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("name too long (%d > %d)", len(name), maxNameLen)
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("name %q contains invalid characters (allowed: A-Z a-z 0-9 . _ -)", name)
	}
	return nil
}

// TmuxSessionName returns the tmux session name for an Arteta workflow.
func TmuxSessionName(workflowName string) string {
	return "arteta-" + workflowName
}
