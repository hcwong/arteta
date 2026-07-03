package harness

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hcwong/arteta/internal/detect"
	"github.com/hcwong/arteta/internal/workflow"
)

func init() { Register(claudeHarness{}) }

type claudeHarness struct{}

func (c claudeHarness) ID() string          { return "claude" }
func (c claudeHarness) DisplayName() string { return "Claude Code" }

// LaunchCommand returns the pane-0 command for Claude. The `;` fallback shell
// keeps the pane alive when Claude exits so a single-pane layout does not tear
// down the tmux session.
func (c claudeHarness) LaunchCommand(resumeID string) string {
	base := "claude"
	if resumeID != "" {
		base = "claude --resume " + resumeID
	}
	return base + `; printf '\n[arteta] Claude exited — type "claude" to restart.\n'; exec ${SHELL:-/bin/sh} -l`
}

func (c claudeHarness) HookConfig() *HookConfig {
	home, _ := os.UserHomeDir()
	return &HookConfig{
		SettingsPath: filepath.Join(home, ".claude", "settings.json"),
		Events: []EventDef{
			{
				RawEventName: "Stop",
				Subcommand:   "stop",
				State:        workflow.StateIdle,
				ParsePayload: ClaudeParsePayload,
			},
			{
				RawEventName:   "Notification",
				Subcommand:     "notification",
				State:          workflow.StateAwaitingInput,
				CaptureMessage: true,
				ParsePayload:   ClaudeParsePayload,
			},
			{
				RawEventName: "UserPromptSubmit",
				Subcommand:   "user-prompt-submit",
				State:        workflow.StateRunning,
				ParsePayload: ClaudeParsePayload,
			},
		},
	}
}

func (c claudeHarness) DetectState(content string) (workflow.State, bool) {
	return detect.FromPaneContent(content)
}

// ClaudeParsePayload extracts session_id and message from a Claude Code hook
// payload. Extra fields sent by Claude are silently ignored.
func ClaudeParsePayload(raw []byte) (sessionID, message string, err error) {
	var p struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", err
	}
	return p.SessionID, p.Message, nil
}
