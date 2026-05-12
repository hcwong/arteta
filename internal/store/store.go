// Package store persists Arteta state as JSON files under a root dir.
//
// Layout (see DECISIONS.md §8):
//
//	<root>/
//	├── config.json
//	├── workflows/<name>.json   (Arteta-owned, infrequent writes)
//	└── sessions/<name>.json    (hook-owned, frequent writes)
//
// All writes are atomic via tmpfile + rename to avoid partial-write corruption.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hcwong/arteta/internal/workflow"
)

// ErrNotFound is returned when a workflow or status file does not exist.
var ErrNotFound = errors.New("not found")

// Store reads and writes Arteta state under a root directory.
type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Root() string          { return s.root }
func (s *Store) WorkflowsDir() string  { return filepath.Join(s.root, "workflows") }
func (s *Store) SessionsDir() string   { return filepath.Join(s.root, "sessions") }
func (s *Store) ConfigPath() string    { return filepath.Join(s.root, "config.json") }
func (s *Store) PinsPath() string      { return filepath.Join(s.root, "pins.json") }
func (s *Store) workflowPath(name string) string {
	return filepath.Join(s.WorkflowsDir(), name+".json")
}
func (s *Store) statusPath(name string) string {
	return filepath.Join(s.SessionsDir(), name+".json")
}

// DefaultRoot computes the default state directory, honouring XDG_STATE_HOME.
func DefaultRoot() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "arteta"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "arteta"), nil
}

// SaveWorkflow writes the workflow as JSON, creating subdirs as needed.
func (s *Store) SaveWorkflow(w workflow.Workflow) error {
	if err := os.MkdirAll(s.WorkflowsDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir workflows: %w", err)
	}
	return writeJSONAtomic(s.workflowPath(w.Name), w)
}

// LoadWorkflow reads a single workflow by name. Returns ErrNotFound if absent.
func (s *Store) LoadWorkflow(name string) (workflow.Workflow, error) {
	var w workflow.Workflow
	if err := readJSON(s.workflowPath(name), &w); err != nil {
		return workflow.Workflow{}, err
	}
	return w, nil
}

// LoadAllWorkflows reads every persisted workflow. Returns nil slice if none.
func (s *Store) LoadAllWorkflows() ([]workflow.Workflow, error) {
	entries, err := os.ReadDir(s.WorkflowsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workflows dir: %w", err)
	}
	out := make([]workflow.Workflow, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var w workflow.Workflow
		if err := readJSON(filepath.Join(s.WorkflowsDir(), e.Name()), &w); err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		out = append(out, w)
	}
	return out, nil
}

// DeleteWorkflow removes the workflow file and its corresponding status file.
// Idempotent: missing files are not an error.
func (s *Store) DeleteWorkflow(name string) error {
	if err := os.Remove(s.workflowPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove workflow file: %w", err)
	}
	if err := os.Remove(s.statusPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove status file: %w", err)
	}
	return nil
}

// LoadPins reads the list of pinned workflow names. Returns nil if no pins file exists.
func (s *Store) LoadPins() ([]string, error) {
	var pins []string
	if err := readJSON(s.PinsPath(), &pins); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return pins, nil
}

// SavePins persists the list of pinned workflow names atomically.
func (s *Store) SavePins(pins []string) error {
	return writeJSONAtomic(s.PinsPath(), pins)
}

// SaveStatus writes a status file for a workflow. Used by the hook subcommand.
func (s *Store) SaveStatus(name string, st workflow.Status) error {
	if err := os.MkdirAll(s.SessionsDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir sessions: %w", err)
	}
	return writeJSONAtomic(s.statusPath(name), st)
}

// LoadStatus reads the latest status for a workflow. Returns ErrNotFound if absent.
func (s *Store) LoadStatus(name string) (workflow.Status, error) {
	var st workflow.Status
	if err := readJSON(s.statusPath(name), &st); err != nil {
		return workflow.Status{}, err
	}
	return st, nil
}

// readJSON unmarshals a JSON file into v. Returns ErrNotFound if the file does not exist.
func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}

// writeJSONAtomic marshals v to JSON and writes path atomically via tmpfile + rename.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
