// Package hook implements the `arteta hook <event>` subcommand handlers.
//
// Hooks are invoked by Claude Code as subprocesses with a JSON payload on
// stdin. They identify their workflow via the ARTETA_WORKFLOW environment
// variable that Arteta sets when launching `claude` in a workflow's pane.
// If ARTETA_WORKFLOW is unset, the hook no-ops — non-Arteta Claude sessions
// stay invisible.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/workflow"
)

// MaxMessageLen caps how much of a Notification's message we persist. The
// homepage truncates further; this is a sanity ceiling so we don't write
// megabytes if Claude sends a giant message.
const MaxMessageLen = 256

const workflowEnvVar = "ARTETA_WORKFLOW"

type Handler struct {
	Store  *store.Store
	Now    func() time.Time
	Lookup func(string) string
}

// payload captures the subset of Claude's hook payload that Arteta uses.
// Claude sends additional fields we ignore.
type payload struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// Handle reads and persists a hook event for the workflow named in
// ARTETA_WORKFLOW. Returns wrote=false (and no error) when invoked outside
// an Arteta-managed Claude session.
func (h *Handler) Handle(event workflow.Event, stdin io.Reader) (bool, error) {
	name := h.Lookup(workflowEnvVar)
	if name == "" {
		return false, nil
	}
	var p payload
	if err := json.NewDecoder(stdin).Decode(&p); err != nil {
		return false, fmt.Errorf("decode hook payload: %w", err)
	}
	msg := ""
	if event == workflow.EventNotification {
		msg = truncate(p.Message, MaxMessageLen)
	}
	st := workflow.Status{
		LastEvent:   event.String(),
		LastMessage: msg,
		SessionID:   p.SessionID,
		Timestamp:   h.Now(),
	}
	if err := h.Store.SaveStatus(name, st); err != nil {
		return false, fmt.Errorf("save status: %w", err)
	}
	return true, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
