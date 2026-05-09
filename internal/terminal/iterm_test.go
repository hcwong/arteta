package terminal

import (
	"errors"
	"strings"
	"testing"
)

func TestEscapeAS(t *testing.T) {
	cases := map[string]string{
		`hello`:           `hello`,
		`with "quotes"`:   `with \"quotes\"`,
		`back\slash`:      `back\\slash`,
		`mix "and" \back`: `mix \"and\" \\back`,
	}
	for in, want := range cases {
		if got := escapeAS(in); got != want {
			t.Errorf("escapeAS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenTabScript_EmbedsCommandAndTitle(t *testing.T) {
	s := openTabScript(OpenOpts{Title: "auth-refactor", Command: "tmux -L arteta attach -t arteta-auth-refactor"})
	if !strings.Contains(s, `tmux -L arteta attach -t arteta-auth-refactor`) {
		t.Error("openTabScript missing command")
	}
	if !strings.Contains(s, `set name of targetSession to "auth-refactor"`) {
		t.Error("openTabScript missing title set")
	}
	if !strings.Contains(s, `tell application "iTerm"`) {
		t.Error("openTabScript missing iTerm tell block")
	}
}

func TestOpenTabScript_EscapesHostileInput(t *testing.T) {
	s := openTabScript(OpenOpts{Title: `it's "evil"`, Command: `echo "x"; rm -rf /`})
	// Quotes in the user-supplied strings must be escaped so they can't break out
	// of the AppleScript string literal.
	if strings.Contains(s, `it's "evil"`) {
		t.Error("openTabScript did not escape quotes in title")
	}
	if !strings.Contains(s, `it's \"evil\"`) {
		t.Errorf("openTabScript title escaping wrong:\n%s", s)
	}
}

func TestParseTabHandle(t *testing.T) {
	h, err := parseTabHandle("0xABC\t12")
	if err != nil {
		t.Fatalf("parseTabHandle: %v", err)
	}
	if h.WindowID != "0xABC" || h.TabID != "12" {
		t.Errorf("parseTabHandle: got %+v", h)
	}
}

func TestParseTabHandle_BadInput(t *testing.T) {
	bads := []string{"", "no-tab", "\t12", "win\t"}
	for _, b := range bads {
		if _, err := parseTabHandle(b); err == nil {
			t.Errorf("parseTabHandle(%q) returned nil error, want error", b)
		}
	}
}

func TestITerm_OpenTab_Roundtrip(t *testing.T) {
	captured := ""
	it := &ITerm{
		Run: func(script string) (string, error) {
			captured = script
			return "0xWIN\t42", nil
		},
	}
	h, err := it.OpenTab(OpenOpts{Title: "wf", Command: "claude"})
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	if h.WindowID != "0xWIN" || h.TabID != "42" {
		t.Errorf("handle: %+v", h)
	}
	if !strings.Contains(captured, "claude") {
		t.Error("script didn't include command")
	}
}

func TestITerm_TabExists_True(t *testing.T) {
	it := &ITerm{Run: func(script string) (string, error) { return "true", nil }}
	ok, err := it.TabExists(TabHandle{WindowID: "1", TabID: "2"})
	if err != nil || !ok {
		t.Errorf("TabExists: ok=%v err=%v", ok, err)
	}
}

func TestITerm_TabExists_False(t *testing.T) {
	it := &ITerm{Run: func(script string) (string, error) { return "false", nil }}
	ok, _ := it.TabExists(TabHandle{WindowID: "1", TabID: "2"})
	if ok {
		t.Error("TabExists: ok=true, want false")
	}
}

func TestITerm_RunErrorPropagates(t *testing.T) {
	it := &ITerm{Run: func(string) (string, error) { return "", errors.New("boom") }}
	if err := it.FocusTab(TabHandle{WindowID: "1", TabID: "2"}); err == nil {
		t.Error("FocusTab: nil error, want error")
	}
}

func TestFake_Lifecycle(t *testing.T) {
	f := NewFake()
	h, err := f.OpenTab(OpenOpts{Title: "x", Command: "claude"})
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	if h.IsZero() {
		t.Error("expected non-zero handle")
	}
	ok, _ := f.TabExists(h)
	if !ok {
		t.Error("TabExists: false, want true")
	}
	if err := f.FocusTab(h); err != nil {
		t.Errorf("FocusTab: %v", err)
	}
	if err := f.CloseTab(h); err != nil {
		t.Errorf("CloseTab: %v", err)
	}
	ok, _ = f.TabExists(h)
	if ok {
		t.Error("TabExists after Close: true, want false")
	}
}

func TestFake_FocusMissingTab_Errors(t *testing.T) {
	f := NewFake()
	if err := f.FocusTab(TabHandle{WindowID: "1", TabID: "999"}); err == nil {
		t.Error("FocusTab on missing: nil error, want error")
	}
}
