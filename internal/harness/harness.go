// Package harness abstracts the AI coding tool that runs in pane 0 of a
// workflow. Callers use the registry (Get / Default) to resolve a Harness by
// its ID rather than importing concrete implementations directly.
package harness

import "github.com/hcwong/arteta/internal/workflow"

// Harness is the abstraction for an AI coding tool (Claude Code, Codex, …).
// Each concrete implementation lives in its own file in this package and
// registers itself via init() → Register.
type Harness interface {
	// ID is the stable lowercase identifier stored in the workflow JSON.
	ID() string
	// DisplayName is the human-readable label used in UI and CLI output.
	DisplayName() string
	// LaunchCommand returns the shell command for pane 0. resumeID is the
	// session token from a previous run (empty = fresh session).
	LaunchCommand(resumeID string) string
	// HookConfig describes how this harness delivers lifecycle events to
	// arteta hooks. Returns nil when the harness has no hook mechanism.
	HookConfig() *HookConfig
	// DetectState infers workflow state from raw tmux pane output. Returns
	// (state, true) only when the heuristic is high-confidence; callers treat
	// (StateUnknown, false) as "no signal — trust hook state".
	DetectState(paneContent string) (workflow.State, bool)
}

// HookConfig describes how a harness delivers lifecycle events.
type HookConfig struct {
	// SettingsPath is the config file arteta installs hook entries into.
	SettingsPath string
	// Events lists the harness events arteta subscribes to.
	Events []EventDef
}

// EventDef maps one harness-specific hook event to arteta's canonical state.
type EventDef struct {
	// RawEventName is the key used in the settings file (e.g. "Stop").
	RawEventName string
	// Subcommand is the `arteta hook <subcommand>` name (e.g. "stop").
	Subcommand string
	// State is the canonical arteta state this event implies.
	State workflow.State
	// CaptureMessage indicates whether to persist the message field from
	// the payload (used for Notification events to surface the prompt text).
	CaptureMessage bool
	// ParsePayload extracts the session ID and message from the raw JSON
	// payload the harness sends when calling the hook.
	ParsePayload PayloadParser
}

// PayloadParser extracts the fields arteta needs from a harness hook payload.
type PayloadParser func(raw []byte) (sessionID, message string, err error)

var reg = map[string]Harness{}

// Register makes h available via Get and All. Called from each harness's init.
func Register(h Harness) { reg[h.ID()] = h }

// Get returns the harness for the given ID. If the ID is unknown or empty,
// it falls back to the Claude harness so that old workflow files work without
// an explicit harness field.
func Get(id string) Harness {
	if h, ok := reg[id]; ok {
		return h
	}
	return reg["claude"]
}

// Default returns the Claude harness.
func Default() Harness { return reg["claude"] }

// All returns every registered harness.
func All() []Harness {
	out := make([]Harness, 0, len(reg))
	for _, h := range reg {
		out = append(out, h)
	}
	return out
}

// WithHooks returns harnesses that expose a HookConfig.
func WithHooks() []Harness {
	var out []Harness
	for _, h := range reg {
		if h.HookConfig() != nil {
			out = append(out, h)
		}
	}
	return out
}

// EventNames extracts the RawEventName from each EventDef for use in
// installer and doctor commands.
func EventNames(defs []EventDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.RawEventName
	}
	return out
}
