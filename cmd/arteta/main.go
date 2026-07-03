// Command arteta is the entrypoint binary. With no subcommand, it launches
// the Bubble Tea homepage. Subcommands handle install/doctor/uninstall,
// workflow lifecycle from the CLI, and the harness hook callbacks.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hcwong/arteta/internal/harness"
	"github.com/hcwong/arteta/internal/hook"
	"github.com/hcwong/arteta/internal/installer"
	"github.com/hcwong/arteta/internal/service"
	"github.com/hcwong/arteta/internal/store"
	"github.com/hcwong/arteta/internal/terminal"
	"github.com/hcwong/arteta/internal/tmux"
	"github.com/hcwong/arteta/internal/tui"
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
		Short: "Arteta — manage AI coding sessions across iTerm tabs",
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
	root.AddCommand(newRestartCmd())
	root.AddCommand(newCycleCmd("next", service.DirNext))
	root.AddCommand(newCycleCmd("prev", service.DirPrev))
	return root
}

// newCycleCmd builds the `next`/`prev` subcommands that focus the adjacent
// workflow needing attention (awaiting input or idle), without returning to
// the homepage. Bind these to an iTerm2 or tmux global hotkey.
func newCycleCmd(use string, dir service.Direction) *cobra.Command {
	verb := "next"
	if dir == service.DirPrev {
		verb = "previous"
	}
	return &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Focus the %s workflow awaiting input or idle", verb),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, svc, err := buildService()
			if err != nil {
				return err
			}
			name, state, err := svc.Cycle(dir)
			if err != nil {
				return err
			}
			if name == "" {
				fmt.Println("No workflows awaiting input or idle.")
				return nil
			}
			fmt.Printf("→ %s (%s)\n", name, state)
			return nil
		},
	}
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
		Short: "Install Arteta hooks into each harness's settings file",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				exe = "arteta"
			}
			for _, h := range harness.WithHooks() {
				ins := installerFor(h, exe)
				backup, err := ins.Install()
				if err != nil {
					return fmt.Errorf("%s: %w", h.DisplayName(), err)
				}
				fmt.Printf("Installed Arteta hooks into %s\n", ins.SettingsPath)
				if backup != "" {
					fmt.Printf("Backup: %s\n", backup)
				}
			}
			return nil
		},
	}
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Arteta hooks from each harness's settings file",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				exe = "arteta"
			}
			for _, h := range harness.WithHooks() {
				ins := installerFor(h, exe)
				removed, backup, err := ins.Uninstall()
				if err != nil {
					return fmt.Errorf("%s: %w", h.DisplayName(), err)
				}
				fmt.Printf("Removed %d Arteta hook entries from %s\n", removed, ins.SettingsPath)
				if backup != "" {
					fmt.Printf("Backup: %s\n", backup)
				}
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
			exe, err := os.Executable()
			if err != nil {
				exe = "arteta"
			}
			for _, h := range harness.WithHooks() {
				ins := installerFor(h, exe)
				rep, err := ins.Doctor()
				if err != nil {
					return fmt.Errorf("%s: %w", h.DisplayName(), err)
				}
				fmt.Printf("[%s] Settings: %s (exists=%t)\n", h.DisplayName(), rep.SettingsPath, rep.Exists)
				for _, ev := range ins.Events {
					status := "missing"
					if rep.Found[ev] {
						status = "installed"
					}
					other := rep.OtherCount[ev]
					fmt.Printf("  %-18s %s  (other entries: %d)\n", ev, status, other)
				}
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

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart pane 0 (harness) in all live workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, svc, err := buildService()
			if err != nil {
				return err
			}
			n, err := svc.RestartAll()
			if err != nil {
				return err
			}
			s := "s"
			if n == 1 {
				s = ""
			}
			fmt.Printf("Restarted %d live workflow%s\n", n, s)
			return nil
		},
	}
}

// newHookCmd builds the `arteta hook` subcommand tree. Subcommands are
// registered dynamically from all harnesses that have a HookConfig, so adding
// a new harness automatically exposes its events here without touching this
// function. Duplicate subcommand names across harnesses are deduplicated.
func newHookCmd() *cobra.Command {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Internal: handlers invoked by AI harness hooks",
	}

	seen := map[string]bool{}
	for _, h := range harness.WithHooks() {
		hc := h.HookConfig()
		for _, ev := range hc.Events {
			if seen[ev.Subcommand] {
				continue
			}
			seen[ev.Subcommand] = true
			def := ev // capture loop variable
			hookCmd.AddCommand(&cobra.Command{
				Use:    def.Subcommand,
				Short:  fmt.Sprintf("Handle the %s hook event", def.RawEventName),
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
					if _, err := h.Handle(def, os.Stdin); err != nil {
						return err
					}
					return nil
				},
			})
		}
	}
	return hookCmd
}

// installerFor builds an Installer configured from a harness's HookConfig.
func installerFor(h harness.Harness, exe string) *installer.Installer {
	hc := h.HookConfig()
	return &installer.Installer{
		SettingsPath: hc.SettingsPath,
		Events:       harness.EventNames(hc.Events),
		HookCmd:      exe + " hook",
		Now:          func() time.Time { return time.Now().UTC() },
	}
}
