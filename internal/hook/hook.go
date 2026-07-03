// Package hook implements the `arteta hook <event>` subcommand handlers.
//
// Hooks are invoked by AI harnesses as subprocesses with a JSON payload on
// stdin. They identify their workflow via the ARTETA_WORKFLOW environment
// variable that Arteta sets when launching the harness in a workflow's pane.
// If ARTETA_WORKFLOW is unset, the hook no-ops — non-Arteta sessions stay
// invisible.
package hook

import (
	"fmt"
	"io"
	"time"

	"github.com/hcwong/arteta/internal/harness"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/workflow"
)

// MaxMessageLen caps how much of a Notification's message we persist.
const MaxMessageLen = 256

const workflowEnvVar = "ARTETA_WORKFLOW"

type Handler struct {
	Store  *store.Store
	Now    func() time.Time
	Lookup func(string) string
}

// Handle reads and persists a hook event for the workflow named in
// ARTETA_WORKFLOW. The caller passes the EventDef that describes this event's
// state mapping and payload parser — resolved from the harness registry by the
// CLI layer so the handler itself remains harness-agnostic.
//
// Returns wrote=false (and no error) when invoked outside an Arteta-managed
// session (ARTETA_WORKFLOW unset).
func (h *Handler) Handle(def harness.EventDef, stdin io.Reader) (bool, error) {
	name := h.Lookup(workflowEnvVar)
	if name == "" {
		return false, nil
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return false, fmt.Errorf("read hook payload: %w", err)
	}
	sessionID, message, err := def.ParsePayload(raw)
	if err != nil {
		return false, fmt.Errorf("decode hook payload: %w", err)
	}
	msg := ""
	if def.CaptureMessage {
		msg = truncate(message, MaxMessageLen)
	}
	st := workflow.Status{
		StateName:   def.State.String(),
		LastEvent:   def.RawEventName,
		LastMessage: msg,
		SessionID:   sessionID,
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
