// Command arteta is the entrypoint binary. With no subcommand, it launches
// the Bubble Tea homepage. Subcommands handle install/doctor/uninstall,
// workflow lifecycle from the CLI, and the Claude hook callbacks.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/hcwong/arteta/internal/hook"
	"github.com/hcwong/arteta/internal/installer"
	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/tui"
	"github.com/hcwong/arteta/internal/workflow"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "arteta",
		Short: "Arteta — manage Claude Code sessions across iTerm tabs",
		// Default action: launch the TUI.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
		SilenceUsage: true,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newUninstallCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newCloseCmd())
	root.AddCommand(newHookCmd())
	return root
}

// buildService returns a Service wired to the real adapters and a Store at
// the default root.
func buildService() (*store.Store, *service.Service, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, nil, fmt.Errorf("mkdir state root: %w", err)
	}
	st := store.New(root)
	svc := &service.Service{
		Store:      st,
		Tmux:       tmux.NewReal(tmux.DefaultSocket),
		Term:       terminal.NewITerm(),
		Now:        func() time.Time { return time.Now().UTC() },
		SocketName: tmux.DefaultSocket,
	}
	return st, svc, nil
}

func runTUI() error {
	st, svc, err := buildService()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return tui.Run(st, svc, cwd)
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Install Arteta hooks into Claude's settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			ins, err := defaultInstaller()
			if err != nil {
				return err
			}
			backup, err := ins.Install()
			if err != nil {
				return err
			}
			fmt.Printf("Installed Arteta hooks into %s\n", ins.SettingsPath)
			if backup != "" {
				fmt.Printf("Backup: %s\n", backup)
			}
			return nil
		},
	}
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Arteta hooks from Claude's settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			ins, err := defaultInstaller()
			if err != nil {
				return err
			}
			removed, backup, err := ins.Uninstall()
			if err != nil {
				return err
			}
			fmt.Printf("Removed %d Arteta hook entries from %s\n", removed, ins.SettingsPath)
			if backup != "" {
				fmt.Printf("Backup: %s\n", backup)
			}
			return nil
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report installed Arteta hooks and detect missing pieces",
		RunE: func(cmd *cobra.Command, args []string) error {
			ins, err := defaultInstaller()
			if err != nil {
				return err
			}
			rep, err := ins.Doctor()
			if err != nil {
				return err
			}
			fmt.Printf("Settings: %s (exists=%t)\n", rep.SettingsPath, rep.Exists)
			for _, ev := range installer.Events {
				status := "missing"
				if rep.Found[ev] {
					status = "installed"
				}
				other := rep.OtherCount[ev]
				fmt.Printf("  %-18s %s  (other entries: %d)\n", ev, status, other)
			}
			return nil
		},
	}
}

func newCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <name>",
		Short: "Close a workflow (kills tmux session, closes iTerm tab, deletes state)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, svc, err := buildService()
			if err != nil {
				return err
			}
			if err := svc.Close(args[0]); err != nil {
				return err
			}
			fmt.Printf("Closed workflow %q\n", args[0])
			return nil
		},
	}
}

func newHookCmd() *cobra.Command {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Internal: handlers invoked by Claude Code hooks",
	}
	hookCmd.AddCommand(hookSubcmd("stop", workflow.EventStop))
	hookCmd.AddCommand(hookSubcmd("notification", workflow.EventNotification))
	hookCmd.AddCommand(hookSubcmd("user-prompt-submit", workflow.EventUserPromptSubmit))
	return hookCmd
}

func hookSubcmd(name string, ev workflow.Event) *cobra.Command {
	return &cobra.Command{
		Use:    name,
		Short:  fmt.Sprintf("Handle the Claude %s hook event", ev.String()),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := buildService()
			if err != nil {
				return err
			}
			h := &hook.Handler{
				Store:  st,
				Now:    func() time.Time { return time.Now().UTC() },
				Lookup: os.Getenv,
			}
			if _, err := h.Handle(ev, os.Stdin); err != nil {
				return err
			}
			return nil
		},
	}
}

func defaultInstaller() (*installer.Installer, error) {
	settingsPath, err := defaultSettingsPath()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "arteta"
	}
	return &installer.Installer{
		SettingsPath: settingsPath,
		HookCmd:      exe + " hook",
		Now:          func() time.Time { return time.Now().UTC() },
	}, nil
}

func defaultSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}
