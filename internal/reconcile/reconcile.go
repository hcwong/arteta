// Package reconcile classifies persisted workflows on Arteta startup
// against the live tmux state. Workflows whose tmux session is missing
// are marked Dormant; the user revives them on demand.
package reconcile

import (
	"fmt"

	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/workflow"
)

// Result splits the persisted workflows into live (tmux session exists)
// and dormant (tmux session missing).
type Result struct {
	Live    []workflow.Workflow
	Dormant []workflow.Workflow
}

// Reconcile classifies the given workflows against the live tmux state.
// Returns the first error encountered from the tmux client.
func Reconcile(workflows []workflow.Workflow, c tmux.Client) (Result, error) {
	var r Result
	for _, w := range workflows {
		ok, err := c.HasSession(w.TmuxSession)
		if err != nil {
			return Result{}, fmt.Errorf("has-session(%q): %w", w.TmuxSession, err)
		}
		if ok {
			r.Live = append(r.Live, w)
		} else {
			r.Dormant = append(r.Dormant, w)
		}
	}
	return r, nil
}
