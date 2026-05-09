package workflow

import (
	"strings"
	"testing"
)

func TestDeriveState(t *testing.T) {
	cases := []struct {
		event Event
		want  State
	}{
		{EventUserPromptSubmit, StateRunning},
		{EventNotification, StateAwaitingInput},
		{EventStop, StateIdle},
		{EventNone, StateUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.event.String(), func(t *testing.T) {
			if got := DeriveState(tc.event); got != tc.want {
				t.Fatalf("DeriveState(%v) = %v, want %v", tc.event, got, tc.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateIdle:          "idle",
		StateRunning:       "running",
		StateAwaitingInput: "awaiting_input",
		StateUnknown:       "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestEventStringRoundtrip(t *testing.T) {
	cases := []Event{EventStop, EventNotification, EventUserPromptSubmit}
	for _, e := range cases {
		got, ok := ParseEvent(e.String())
		if !ok {
			t.Errorf("ParseEvent(%q) returned ok=false", e.String())
		}
		if got != e {
			t.Errorf("ParseEvent(%q) = %v, want %v", e.String(), got, e)
		}
	}
}

func TestParseEventUnknown(t *testing.T) {
	if _, ok := ParseEvent("BogusEvent"); ok {
		t.Error("ParseEvent(\"BogusEvent\") returned ok=true, want false")
	}
	if _, ok := ParseEvent(""); ok {
		t.Error("ParseEvent(\"\") returned ok=true, want false")
	}
}

func TestLayoutValid(t *testing.T) {
	valid := []Layout{LayoutSingle, LayoutVSplit, LayoutHSplit, LayoutQuad}
	for _, l := range valid {
		if !l.Valid() {
			t.Errorf("Layout(%q).Valid() = false, want true", l)
		}
	}
	invalid := []Layout{"", "tri", "QUAD", "vert"}
	for _, l := range invalid {
		if l.Valid() {
			t.Errorf("Layout(%q).Valid() = true, want false", l)
		}
	}
}

func TestParseLayout(t *testing.T) {
	cases := map[string]Layout{
		"single": LayoutSingle,
		"vsplit": LayoutVSplit,
		"hsplit": LayoutHSplit,
		"quad":   LayoutQuad,
	}
	for in, want := range cases {
		got, err := ParseLayout(in)
		if err != nil {
			t.Errorf("ParseLayout(%q) returned error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseLayout(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseLayout("nope"); err == nil {
		t.Error("ParseLayout(\"nope\") returned no error, want error")
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"auth-refactor", "login_bug", "issue123", "a", "x.y"}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) returned error: %v", n, err)
		}
	}
	bad := []string{
		"",                 // empty
		"  ",               // whitespace only
		"name with space",  // space disallowed (would break tmux session names)
		"colon:bad",        // colons disallowed for tmux
		"dot.in.middle.ok", // dots allowed actually — should be in good
		strings.Repeat("a", 256),
	}
	// dot.in.middle.ok was placed in bad by mistake; assert it's good.
	if err := ValidateName("dot.in.middle.ok"); err != nil {
		t.Errorf("ValidateName(%q) returned error: %v (dots should be allowed)", "dot.in.middle.ok", err)
	}
	for _, n := range bad {
		if n == "dot.in.middle.ok" {
			continue
		}
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) returned nil, want error", n)
		}
	}
}

func TestTmuxSessionName(t *testing.T) {
	if got := TmuxSessionName("auth-refactor"); got != "arteta-auth-refactor" {
		t.Errorf("TmuxSessionName(%q) = %q, want %q", "auth-refactor", got, "arteta-auth-refactor")
	}
}
