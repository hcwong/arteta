package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestInstaller(t *testing.T) (*Installer, string) {
	t.Helper()
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	now := time.Date(2026, 5, 9, 17, 0, 0, 0, time.UTC)
	return &Installer{
		SettingsPath: settings,
		HookCmd:      "arteta hook",
		Now:          func() time.Time { return now },
	}, settings
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return m
}

func TestInstall_FreshFile_CreatesAllEvents(t *testing.T) {
	inst, path := newTestInstaller(t)
	if _, err := inst.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	m := readSettings(t, path)
	hooks, ok := m["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks key missing or wrong type: %T", m["hooks"])
	}
	for _, ev := range Events {
		entries, ok := hooks[ev].([]any)
		if !ok || len(entries) == 0 {
			t.Errorf("event %q: no entries installed", ev)
		}
	}
}

func TestInstall_PreservesExistingHooks(t *testing.T) {
	inst, path := newTestInstaller(t)
	// Pre-populate with a user-defined hook for Stop and an unrelated event.
	pre := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": "/usr/local/bin/peon-ping stop"},
					},
				},
			},
			"PreToolUse": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": "log-tool-use"},
					},
				},
			},
		},
		"theme": "dark", // unrelated top-level setting
	}
	writeJSON(t, path, pre)

	if _, err := inst.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	m := readSettings(t, path)

	if m["theme"] != "dark" {
		t.Errorf("unrelated key 'theme' was clobbered: %v", m["theme"])
	}
	hooks := m["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("unrelated PreToolUse hooks were dropped")
	}
	stopEntries := hooks["Stop"].([]any)
	if len(stopEntries) != 2 {
		t.Errorf("Stop entries: got %d, want 2 (user's + arteta's)", len(stopEntries))
	}
	// First entry should still be user's peon-ping hook.
	first := stopEntries[0].(map[string]any)
	firstHooks := first["hooks"].([]any)
	firstCmd := firstHooks[0].(map[string]any)["command"].(string)
	if !strings.Contains(firstCmd, "peon-ping") {
		t.Errorf("user's peon-ping hook lost: first cmd is %q", firstCmd)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	inst, path := newTestInstaller(t)
	for i := 0; i < 3; i++ {
		if _, err := inst.Install(); err != nil {
			t.Fatalf("Install #%d: %v", i, err)
		}
	}
	m := readSettings(t, path)
	hooks := m["hooks"].(map[string]any)
	for _, ev := range Events {
		entries := hooks[ev].([]any)
		if len(entries) != 1 {
			t.Errorf("event %q after 3 installs: got %d entries, want 1", ev, len(entries))
		}
	}
}

func TestInstall_BackupCreated_WhenFileExists(t *testing.T) {
	inst, path := newTestInstaller(t)
	writeJSON(t, path, map[string]any{"theme": "light"})
	backup, err := inst.Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if backup == "" {
		t.Fatal("expected backup path, got empty")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("backup file not found at %q: %v", backup, err)
	}
}

func TestInstall_NoBackup_WhenFileMissing(t *testing.T) {
	inst, _ := newTestInstaller(t)
	backup, err := inst.Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if backup != "" {
		t.Errorf("expected no backup for fresh install, got %q", backup)
	}
}

func TestUninstall_RemovesOnlyArtetaEntries(t *testing.T) {
	inst, path := newTestInstaller(t)
	if _, err := inst.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Append a user-owned hook alongside Arteta's for Stop.
	m := readSettings(t, path)
	hooks := m["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	stop = append(stop, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "/usr/local/bin/peon-ping stop"},
		},
	})
	hooks["Stop"] = stop
	writeJSON(t, path, m)

	removed, _, err := inst.Uninstall()
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if removed != 3 { // one per event
		t.Errorf("removed: got %d, want 3", removed)
	}
	m = readSettings(t, path)
	hooks = m["hooks"].(map[string]any)
	stop = hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Errorf("Stop entries after uninstall: got %d, want 1 (the user's)", len(stop))
	}
	cmd := stop[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if !strings.Contains(cmd, "peon-ping") {
		t.Errorf("user's peon-ping hook was removed: cmd is %q", cmd)
	}
}

func TestUninstall_NoFile_NoOp(t *testing.T) {
	inst, _ := newTestInstaller(t)
	removed, backup, err := inst.Uninstall()
	if err != nil {
		t.Errorf("Uninstall on missing file should be a no-op, got error: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed: got %d, want 0", removed)
	}
	if backup != "" {
		t.Errorf("backup: got %q, want empty", backup)
	}
}

func TestDoctor_ReportsInstalled(t *testing.T) {
	inst, _ := newTestInstaller(t)
	if _, err := inst.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	rep, err := inst.Doctor()
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	for _, ev := range Events {
		if !rep.Found[ev] {
			t.Errorf("Doctor.Found[%q] = false, want true", ev)
		}
	}
}

func TestDoctor_MissingFile(t *testing.T) {
	inst, _ := newTestInstaller(t)
	rep, err := inst.Doctor()
	if err != nil {
		t.Fatalf("Doctor on missing file should not error, got %v", err)
	}
	for _, ev := range Events {
		if rep.Found[ev] {
			t.Errorf("Doctor.Found[%q] = true, want false (no file)", ev)
		}
	}
}

func writeJSON(t *testing.T, path string, m map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
