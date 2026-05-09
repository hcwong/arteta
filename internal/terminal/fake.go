package terminal

import (
	"fmt"
	"strconv"
	"sync"
)

// Fake is an in-memory Adapter for tests. It assigns synthetic window/tab
// IDs and records every call.
type Fake struct {
	mu       sync.Mutex
	tabs     map[string]openFakeTab
	nextID   int
	Calls    []string
}

type openFakeTab struct {
	WindowID string
	TabID    string
	Title    string
	Command  string
}

func NewFake() *Fake {
	return &Fake{tabs: map[string]openFakeTab{}}
}

func key(h TabHandle) string { return h.WindowID + ":" + h.TabID }

func (f *Fake) OpenTab(opts OpenOpts) (TabHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	h := TabHandle{
		WindowID: "1",
		TabID:    strconv.Itoa(f.nextID),
	}
	f.tabs[key(h)] = openFakeTab{WindowID: h.WindowID, TabID: h.TabID, Title: opts.Title, Command: opts.Command}
	f.Calls = append(f.Calls, "OpenTab:"+opts.Title)
	return h, nil
}

func (f *Fake) FocusTab(h TabHandle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "FocusTab:"+key(h))
	if _, ok := f.tabs[key(h)]; !ok {
		return fmt.Errorf("tab %s not found", key(h))
	}
	return nil
}

func (f *Fake) CloseTab(h TabHandle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "CloseTab:"+key(h))
	delete(f.tabs, key(h))
	return nil
}

func (f *Fake) TabExists(h TabHandle) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.tabs[key(h)]
	return ok, nil
}

// Tabs returns a snapshot of currently-open fake tabs.
func (f *Fake) Tabs() []openFakeTab {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]openFakeTab, 0, len(f.tabs))
	for _, t := range f.tabs {
		out = append(out, t)
	}
	return out
}
