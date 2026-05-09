// Package installer manages Arteta's hook entries inside Claude's
// settings.json. Writes are additive (existing user hooks are preserved)
// and uninstall removes only Arteta-tagged entries (those whose command
// starts with HookCmd).
package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Events lists the Claude hook events Arteta installs handlers for.
// Order is stable (used for predictable output in Doctor reports).
var Events = []string{"Stop", "Notification", "UserPromptSubmit"}

// Installer mutates a Claude settings.json file.
type Installer struct {
	SettingsPath string
	HookCmd      string // command prefix that identifies an Arteta entry, e.g. "arteta hook"
	Now          func() time.Time
}

// DoctorReport describes what Arteta-managed state currently exists in settings.json.
type DoctorReport struct {
	SettingsPath string
	Exists       bool
	Found        map[string]bool // event -> Arteta hook installed
	OtherCount   map[string]int  // event -> count of non-Arteta entries (informational)
}

// Install adds Arteta hook entries for each event in Events. Idempotent —
// running twice does not duplicate entries. Returns the path of a backup
// file created before write, or "" if no prior file existed.
func (i *Installer) Install() (string, error) {
	settings, existed, err := i.loadSettings()
	if err != nil {
		return "", err
	}
	backup := ""
	if existed {
		b, err := i.writeBackup()
		if err != nil {
			return "", err
		}
		backup = b
	}
	for _, ev := range Events {
		i.ensureEntry(settings, ev)
	}
	if err := i.writeSettings(settings); err != nil {
		return "", err
	}
	return backup, nil
}

// Uninstall removes Arteta-tagged entries and writes settings back.
// Returns count removed and backup path. Missing file is a no-op.
func (i *Installer) Uninstall() (int, string, error) {
	settings, existed, err := i.loadSettings()
	if err != nil {
		return 0, "", err
	}
	if !existed {
		return 0, "", nil
	}
	backup, err := i.writeBackup()
	if err != nil {
		return 0, "", err
	}
	removed := 0
	for _, ev := range Events {
		removed += i.removeEntries(settings, ev)
	}
	if err := i.writeSettings(settings); err != nil {
		return 0, backup, err
	}
	return removed, backup, nil
}

// Doctor reports which Arteta hooks are currently installed.
func (i *Installer) Doctor() (DoctorReport, error) {
	rep := DoctorReport{
		SettingsPath: i.SettingsPath,
		Found:        map[string]bool{},
		OtherCount:   map[string]int{},
	}
	settings, existed, err := i.loadSettings()
	if err != nil {
		return rep, err
	}
	rep.Exists = existed
	hooks, _ := settings["hooks"].(map[string]any)
	for _, ev := range Events {
		entries, _ := hooks[ev].([]any)
		var artetaCount, otherCount int
		for _, e := range entries {
			if i.isArtetaEntry(e) {
				artetaCount++
			} else {
				otherCount++
			}
		}
		rep.Found[ev] = artetaCount > 0
		rep.OtherCount[ev] = otherCount
	}
	return rep, nil
}

// ensureEntry adds an Arteta entry for the event if not already present.
func (i *Installer) ensureEntry(settings map[string]any, event string) {
	hooks := getOrCreateMap(settings, "hooks")
	entries, _ := hooks[event].([]any)
	for _, e := range entries {
		if i.isArtetaEntry(e) {
			return
		}
	}
	entry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": i.HookCmd + " " + hookSubcommand(event),
			},
		},
	}
	hooks[event] = append(entries, entry)
}

// removeEntries strips Arteta-only entries from the given event's array.
// Returns count removed.
func (i *Installer) removeEntries(settings map[string]any, event string) int {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return 0
	}
	entries, _ := hooks[event].([]any)
	if entries == nil {
		return 0
	}
	kept := make([]any, 0, len(entries))
	removed := 0
	for _, e := range entries {
		if i.isArtetaEntry(e) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = kept
	}
	return removed
}

// isArtetaEntry returns true iff the entry's hooks array contains only
// commands tagged with HookCmd. Mixed entries (user + arteta) are NOT
// considered Arteta-owned, to avoid clobbering user hooks.
func (i *Installer) isArtetaEntry(entry any) bool {
	em, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	hooks, ok := em["hooks"].([]any)
	if !ok || len(hooks) == 0 {
		return false
	}
	for _, h := range hooks {
		hm, ok := h.(map[string]any)
		if !ok {
			return false
		}
		cmd, _ := hm["command"].(string)
		if !strings.HasPrefix(cmd, i.HookCmd) {
			return false
		}
	}
	return true
}

// loadSettings reads the settings file, returning an empty map if it does
// not exist (existed=false in that case).
func (i *Installer) loadSettings() (map[string]any, bool, error) {
	data, err := os.ReadFile(i.SettingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, false, nil
		}
		return nil, false, fmt.Errorf("read settings: %w", err)
	}
	if len(data) == 0 {
		return map[string]any{}, true, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("parse settings: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, true, nil
}

func (i *Installer) writeSettings(m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(i.SettingsPath), 0o755); err != nil {
		return fmt.Errorf("mkdir settings dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(i.SettingsPath), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmpPath, i.SettingsPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func (i *Installer) writeBackup() (string, error) {
	data, err := os.ReadFile(i.SettingsPath)
	if err != nil {
		return "", fmt.Errorf("read for backup: %w", err)
	}
	ts := i.Now().UTC().Format("20060102-150405")
	backupPath := i.SettingsPath + ".arteta-backup-" + ts
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return backupPath, nil
}

func getOrCreateMap(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	m := map[string]any{}
	parent[key] = m
	return m
}

// hookSubcommand maps a Claude hook event name to Arteta's CLI subcommand.
func hookSubcommand(event string) string {
	switch event {
	case "Stop":
		return "stop"
	case "Notification":
		return "notification"
	case "UserPromptSubmit":
		return "user-prompt-submit"
	default:
		return strings.ToLower(event)
	}
}
