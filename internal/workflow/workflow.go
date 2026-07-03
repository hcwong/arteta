// Package workflow defines Arteta's core domain types and the state machine
// that maps hook events to workflow states.
package workflow

import (
	"encoding/json"
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
	Name      string    `json:"name"`
	Cwd       string    `json:"cwd"`
	TmuxSession string  `json:"tmux_session"`
	// Harness is the ID of the AI tool running in pane 0 (e.g. "claude").
	// Defaults to "claude" when absent for backward compatibility.
	Harness   string    `json:"harness,omitempty"`
	// SessionID is the harness-specific token used to resume a previous
	// session. Formerly persisted as "claude_session_id"; the custom
	// UnmarshalJSON migrates old files transparently.
	SessionID string    `json:"session_id,omitempty"`
	GitBranch string    `json:"git_branch,omitempty"`
	Layout    Layout    `json:"layout"`
	ITermTab  *ITermTab `json:"iterm_tab,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// UnmarshalJSON handles migration from the old "claude_session_id" field and
// sets Harness to "claude" when the field is absent (pre-harness workflows).
func (w *Workflow) UnmarshalJSON(data []byte) error {
	// Use a type alias to avoid infinite recursion.
	type raw Workflow
	var v struct {
		raw
		LegacySessionID string `json:"claude_session_id,omitempty"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*w = Workflow(v.raw)
	if w.SessionID == "" && v.LegacySessionID != "" {
		w.SessionID = v.LegacySessionID
	}
	if w.Harness == "" {
		w.Harness = "claude"
	}
	return nil
}

// ITermTab is the persisted iTerm window/tab pointer for fast focus on reopen.
type ITermTab struct {
	WindowID string `json:"window_id"`
	TabID    string `json:"tab_id"`
}

// Status is the latest hook-derived state for a workflow. Hook subprocesses
// own writes to this shape; Arteta reads via fsnotify.
//
// New records store the canonical state string in StateName; LastEvent holds
// the raw harness event name for debugging. Legacy records (pre-harness
// abstraction) have only LastEvent — State() handles both.
type Status struct {
	// StateName is the canonical arteta state written by the hook handler.
	// One of "idle", "running", "awaiting_input", or "".
	StateName   string    `json:"state,omitempty"`
	// LastEvent is the raw harness event name (e.g. "Stop"). Kept for
	// debugging and as a legacy fallback for old status files.
	LastEvent   string    `json:"last_event,omitempty"`
	LastMessage string    `json:"last_message,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

// State returns the canonical workflow state. It checks StateName first (new
// records), then falls back to deriving state from LastEvent (legacy records).
func (s Status) State() State {
	if s.StateName != "" {
		switch s.StateName {
		case "idle":
			return StateIdle
		case "running":
			return StateRunning
		case "awaiting_input":
			return StateAwaitingInput
		}
	}
	// Legacy path: records written before the harness abstraction.
	e, _ := ParseEvent(s.LastEvent)
	return DeriveState(e)
}

// EffectiveState blends a hook-reported state with a screen-detected state,
// giving screen detection precedence when it carries a signal. This is the
// single source of truth for "what state is this workflow really in",
// shared by the homepage and the CLI cycle command so they always agree.
func EffectiveState(hookState, screenState State) State {
	if screenState != StateUnknown {
		return screenState
	}
	return hookState
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
