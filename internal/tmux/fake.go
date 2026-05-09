package tmux

import (
	"fmt"
	"sync"
)

// Fake is an in-memory Client used by tests. Methods record their calls
// and the in-memory session graph is queryable via Sessions / Panes.
type Fake struct {
	mu       sync.Mutex
	sessions map[string]*FakeSession
	Calls    []string
}

// FakeSession tracks the state of a session in the Fake.
type FakeSession struct {
	Name   string
	Cwd    string
	Env    map[string]string
	Panes  []FakePane
	Layout string
}

type FakePane struct {
	Cwd     string
	Cmd     string
	Env     map[string]string
	Current string // current foreground command
}

func NewFake() *Fake {
	return &Fake{sessions: map[string]*FakeSession{}}
}

func (f *Fake) Sessions() map[string]*FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]*FakeSession, len(f.sessions))
	for k, v := range f.sessions {
		out[k] = v
	}
	return out
}

func (f *Fake) HasSession(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "HasSession:"+name)
	_, ok := f.sessions[name]
	return ok, nil
}

func (f *Fake) ListSessions() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "ListSessions")
	out := make([]string, 0, len(f.sessions))
	for n := range f.sessions {
		out = append(out, n)
	}
	return out, nil
}

func (f *Fake) NewSession(opts NewSessionOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "NewSession:"+opts.Name)
	if _, exists := f.sessions[opts.Name]; exists {
		return fmt.Errorf("session %q already exists", opts.Name)
	}
	current := opts.Cmd
	if current == "" {
		current = "bash"
	}
	f.sessions[opts.Name] = &FakeSession{
		Name: opts.Name,
		Cwd:  opts.Cwd,
		Env:  copyEnv(opts.Env),
		Panes: []FakePane{
			{Cwd: opts.Cwd, Cmd: opts.Cmd, Env: copyEnv(opts.Env), Current: foregroundOf(opts.Cmd)},
		},
	}
	return nil
}

func (f *Fake) KillSession(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "KillSession:"+name)
	if _, ok := f.sessions[name]; !ok {
		return fmt.Errorf("session %q not found", name)
	}
	delete(f.sessions, name)
	return nil
}

func (f *Fake) SplitWindow(opts SplitOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "SplitWindow:"+opts.Target)
	s, ok := f.sessions[opts.Target]
	if !ok {
		return fmt.Errorf("session %q not found", opts.Target)
	}
	s.Panes = append(s.Panes, FakePane{
		Cwd:     opts.Cwd,
		Cmd:     opts.Cmd,
		Env:     copyEnv(opts.Env),
		Current: foregroundOf(opts.Cmd),
	})
	return nil
}

func (f *Fake) SelectLayout(session, layout string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "SelectLayout:"+session+":"+layout)
	s, ok := f.sessions[session]
	if !ok {
		return fmt.Errorf("session %q not found", session)
	}
	s.Layout = layout
	return nil
}

func (f *Fake) PaneCommands(session string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "PaneCommands:"+session)
	s, ok := f.sessions[session]
	if !ok {
		return nil, fmt.Errorf("session %q not found", session)
	}
	out := make([]string, 0, len(s.Panes))
	for _, p := range s.Panes {
		out = append(out, p.Current)
	}
	return out, nil
}

// SetPaneCurrent overrides the current command of a pane (used in tests
// to simulate "claude exited" or "user is now in bash").
func (f *Fake) SetPaneCurrent(session string, paneIdx int, cmd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[session]
	if !ok {
		return fmt.Errorf("session %q not found", session)
	}
	if paneIdx < 0 || paneIdx >= len(s.Panes) {
		return fmt.Errorf("pane %d out of range (have %d)", paneIdx, len(s.Panes))
	}
	s.Panes[paneIdx].Current = cmd
	return nil
}

func copyEnv(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// foregroundOf returns the leading word of cmd — what tmux would report
// as `pane_current_command`.
func foregroundOf(cmd string) string {
	if cmd == "" {
		return "bash"
	}
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == ' ' {
			return cmd[:i]
		}
	}
	return cmd
}
