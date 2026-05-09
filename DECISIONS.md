# Arteta — Design Decisions

This document captures the foundational decisions made during the MVP design phase, with rationale and a list of items to revisit post-MVP. Refer back here when in doubt about *why* something is built a particular way.

## How to read this doc

Each decision has:
- **Decision**: the chosen path.
- **Why**: the reasoning that won us over.
- **Tradeoffs / what we gave up**: what's worse under this decision.
- **Revisit**: marked ⟳ if we explicitly intend to reconsider after MVP.

---

## 1. Language: Go

**Decision**: Build Arteta in Go.

**Why**:
- The Charm stack (Bubble Tea, Lipgloss, Bubbles) is the most mature TUI ecosystem and minimises time-to-MVP.
- Arteta is a thin orchestrator over `tmux` and `osascript`; we shell out constantly. Go's `os/exec` is more ergonomic than Rust's for this.
- Single static binary, trivial cross-compilation.
- We don't need Rust's perf or memory guarantees here.

**Tradeoffs**: We give up Rust's type safety on edge cases (nil panics still possible). Acceptable for the problem domain.

---

## 2. Tmux architecture: own socket

**Decision**: Arteta runs its own tmux server on a dedicated socket (`tmux -L arteta ...`). User's existing tmux is untouched.

**Why**:
- Isolation — user's own sessions, keybindings, and config don't collide with Arteta's.
- Predictable session naming — Arteta owns the namespace.
- Clean persistence — restoring Arteta sessions doesn't restore unrelated user work.
- Forkability — plugins can rely on a known socket location.

**Tradeoffs**: Users with a customised `~/.tmux.conf` will find Arteta sessions feel different by default.

**Mitigation**: A `--source-user-conf` flag (default behaviour TBD during build) sources the user's `tmux.conf` for Arteta sessions.

---

## 3. Status detection: hooks-based, user-level install

**Decision**: Detect Claude session state via Claude Code hooks (`Stop`, `Notification`, `UserPromptSubmit`), installed at the user level (`~/.claude/settings.json`).

**Why**:
- Hooks are the official, event-driven mechanism — no fragile UI parsing.
- Same approach Peon Ping uses; user already trusts the model.
- User-level install means hooks fire for every Arteta workflow regardless of project.
- Hook payloads carry `session_id` and message text for free, giving us state + summary in one event.

**Install / uninstall safety**:
- `arteta init` appends Arteta's hook entries to `~/.claude/settings.json`, never overwriting existing user hooks. Backs up to `settings.json.arteta-backup-<ts>` first.
- `arteta uninstall` removes only Arteta-tagged entries (greppable via `arteta hook` command path).
- `arteta doctor` prints what's installed and where.

**Tradeoffs**:
- Modifying user-global settings is invasive. Mitigated by additive writes, backups, and an explicit uninstall command.
- Hooks fire for non-Arteta Claude sessions too. Arteta's hook script no-ops when `ARTETA_WORKFLOW` env var is unset, so untracked sessions stay invisible.

**Fallback**: We may also tail `~/.claude/projects/<encoded>/<sid>.jsonl` for richer context, but it's not the primary signal.

---

## 4. Workflow model: 1 workflow = 1 Claude session

**Decision**: A workflow is exactly one Claude session, one tmux session, one working directory, one user-given name.

**Why**:
- Matches the spec's mental model ("a separate view for terminal, Claude Code session, Neovim").
- Homepage state display only makes sense if state is unambiguous. Multiple Claudes per workflow forces aggregation ("2 of 3 idle"), which is worse UX.
- Persistence is simple: one workflow ↔ one `claude_session_id` to resume on restart.
- Claude is the first-class citizen; everything else is supporting context.

**Workflow shape**:
```json
{
  "name": "auth-refactor",
  "cwd": "/Users/josh/repo",
  "tmux_session": "arteta-auth-refactor",
  "claude_session_id": "abc-123",
  "git_branch": "feat/auth",
  "layout": "quad",
  "iterm_tab": {"window_id": "...", "tab_id": "..."},
  "created_at": "..."
}
```

**Tradeoffs**: "Compare two Claude approaches on the same code" requires two workflows with the same cwd. Slightly awkward but workable.

---

## 5. MVP scope

**In**:
- Homepage TUI (list workflows, vim navigation).
- Create workflow (name + cwd + layout).
- Open workflow (focus existing iTerm tab or create new).
- Status detection via hooks; homepage refreshes via fsnotify.
- Hook lifecycle commands: `arteta init`, `arteta uninstall`, `arteta doctor`.
- Persistence: workflows survive Arteta and tmux restarts.
- Close workflow: kill tmux session, remove state, close iTerm tab.
- Four fixed layouts: single, vsplit, hsplit, quadrant.
- nvim pane and `git diff` pane (in `quad` layout).

**Out**:
- Mid-workflow layout switching.
- Customisable pane content per layout.
- Plugin system (terminal adapters, diff adapters).
- Tmux scroll-back / pane content persistence.
- Non-iTerm terminal support (the abstraction exists, only the iTerm impl ships).
- Customisable hotkeys.
- Workflow templates.
- Hunk / bat as the diff renderer (default is plain `git diff`).

---

## 6. Layouts: 4 fixed, immutable per workflow ⟳

**Decision**: Layout (`single` | `vsplit` | `hsplit` | `quad`) is chosen at workflow creation and is immutable for the lifetime of the workflow in MVP. To change, close and recreate.

**Pane content per layout (hardcoded for MVP)**:
- `single`: claude
- `vsplit`: claude (left) | terminal (right)
- `hsplit`: claude (top) / terminal (bottom)
- `quad`: claude (TL) | terminal (TR) | nvim (BL) | git diff (BR)

**Why immutable for MVP**: Migrating pane content during a layout change is fiddly and not worth MVP cost.

**Revisit ⟳**: Mid-workflow layout switching is the first thing to look at after MVP smoke-tests well. User accepted with reluctance — this is a known UX tax.

---

## 7. iTerm integration: AppleScript, behind `TerminalAdapter` interface ⟳

**Decision**: Drive iTerm via `osascript` AppleScript calls. Behind a `terminal.Adapter` interface so other terminal impls can be swapped in.

**Interface (sketched)**:
```go
type Adapter interface {
    OpenTab(title string, cmd string) (TabHandle, error)
    FocusTab(h TabHandle) error
    CloseTab(h TabHandle) error
    TabExists(h TabHandle) (bool, error)
}
```

**Why AppleScript**:
- Zero install — built into macOS.
- The interface boundary means we can swap to the iTerm Python API or other terminal adapters without touching domain code.

**Tradeoffs**: AppleScript is finicky and ~100ms per call. Tab ID staleness (user manually closes a tab) is a real concern. Mitigation: when focus fails, fall back to opening a new tab.

**Revisit ⟳**: Move to the iTerm Python API for the next version — better events, richer introspection, fewer staleness footguns. User explicitly flagged this for the next iteration.

---

## 8. Persistence: JSON files on disk ⟳

**Decision**: State lives under `~/.local/state/arteta/` (XDG state convention) as JSON files. Two files per workflow.

**Layout**:
```
~/.local/state/arteta/
├── config.json
├── workflows/
│   └── <name>.json   # Arteta-owned, infrequent writes
└── sessions/
    └── <name>.json   # hook-owned, frequent writes
```

**Why two files per workflow**:
- No write contention — Arteta owns workflow files; hook subprocesses own status files.
- Atomic per-file writes (tmpfile + rename) — no global state corruption if a process dies mid-write.
- fsnotify on `sessions/` only — Arteta watches that dir for state changes.

**Why JSON over SQLite for MVP**:
- Hooks are shell-invoked subprocesses. JSON via `cat > foo.json.tmp && mv` is trivial; SQLite would force them through the `sqlite3` CLI or a wrapper, defeating the point.
- Debugging story: `cat workflows/foo.json` beats `sqlite3 db "SELECT..."`.
- Stdlib only.

**Revisit ⟳**: SQLite makes sense once we have an event-history feature ("show me every time Claude has asked for input on this workflow"). That's append-only, relational, and SQL is the right tool.

---

## 9. State machine: 3 states, auto-respawn

**Decision**: Three states — `running`, `awaiting_input`, `idle`. Derived from the latest hook event in the status file.

**Transitions**:

| `last_event`        | → state            |
|---------------------|--------------------|
| `UserPromptSubmit`  | `running`          |
| `Notification`      | `awaiting_input`   |
| `Stop`              | `idle`             |

**On Claude process exit** (crash, user typed `exit`, etc.): Arteta auto-respawns with `claude --resume <session_id>` to preserve continuity. Logged but not surfaced as an error in normal flow.

**Why no `no_claude` state**: Claude is the workflow. A workflow without Claude is broken, not a steady state. To intentionally end a workflow, use `arteta close`.

**Edge cases punted to post-MVP**:
- Hook events out-of-order or dropped.
- "Notification was already responded to" — relies on `UserPromptSubmit` flipping state on next prompt.
- Heartbeat / "stuck" detection.
- Confirm-before-respawn when user typed `exit`.

---

## 10. Reconciliation on restart: dormant model ⟳

**Decision**: On Arteta startup, persisted workflows whose tmux sessions are missing are marked `dormant`. They appear on the homepage with a "[dormant — Enter to revive]" indicator. Press Enter to recreate the tmux session and restart Claude with `claude --resume`.

**Why dormant over auto-revive**:
- After a machine reboot, users may want to triage which workflows still matter before reviving N tmux sessions and Claude processes.
- One Enter to revive is cheap; auto-revive that wasn't wanted is annoying and burns tokens.

**Revisit ⟳**: If "press Enter to revive" gets tedious, switch to auto-revive on Arteta start with an opt-out flag.

---

## 11. Hook implementation: `arteta hook` subcommands, env-var keyed

**Decision**: Hooks are subcommands of the main `arteta` binary (`arteta hook stop`, `arteta hook notification`, `arteta hook user-prompt-submit`). The hook reads JSON from stdin, looks up `$ARTETA_WORKFLOW` env var, and writes the status file atomically.

**Why subcommands over a separate `arteta-hook` binary**:
- One distribution unit, one PATH entry, one version.
- Shared code with main Arteta — paths, JSON shape, no duplication.
- `arteta hook` is the greppable marker for `init`/`uninstall`/`doctor`.

**Why `ARTETA_WORKFLOW` env var over cwd-based lookup**:
- Two workflows can share a cwd (compare-two-Claudes scenario). cwd lookup ambiguates.
- Env var is what Arteta directly controls when launching `claude`.
- If `ARTETA_WORKFLOW` is unset, hook no-ops — non-Arteta Claude sessions stay invisible.

---

## 12. Architecture and TDD posture

**Decision**: Hexagonal architecture. Pure domain logic + adapter interfaces. Strict TDD on the core; integration tests for shell-outs.

**Repo layout**:
```
arteta/
├── cmd/arteta/         # main + cobra subcommands
├── internal/
│   ├── workflow/       # core domain: state machine, transitions
│   ├── store/          # JSON persistence
│   ├── tmux/           # tmux.Client iface + real impl + fake
│   ├── terminal/       # terminal.Adapter iface + iterm impl + fake
│   ├── hook/           # hook subcommand handlers
│   ├── installer/      # settings.json install/uninstall/doctor
│   ├── tui/            # Bubble Tea models + views
│   └── reconcile/      # restart reconciliation
├── SPEC.md
├── DECISIONS.md
└── go.mod
```

**TDD discipline**:
- **Strict (test-first)**: `workflow`, `store`, `hook`, `installer`, `reconcile`. No production line without a failing test first.
- **Integration tests**: real `tmux -L arteta-test` on a temp socket, gated by `-tags=integration`.
- **TUI**: `Update` and `View` are pure functions of model state. Test by feeding `tea.Msg`s and asserting on output. Not strict test-first.
- **Adapter interfaces**: real impl exists alongside a fake; domain tests use the fake.

---

## 13. UI: vim keybindings, modal create

**Decision**: Vim-style keybindings throughout. Assume users are vim users.

**Homepage keybinds**:
- `j/k` or `↓/↑` — move selection
- `Enter` — open selected workflow
- `n` — new workflow → modal
- `D` (shift-d, with confirm) — close workflow
- `r` — refresh
- `q` or `Ctrl+C` — quit Arteta TUI (workflows keep running)
- `?` — help overlay

**New workflow modal**: name (required, unique), cwd (defaults to `$PWD`), layout (radio).

**Quit semantics**: `q` exits the TUI process. Tmux sessions, iTerm tabs, and Claude processes keep running. To shut down a single workflow, use `D` or `arteta close <name>`.

**Returning from a workflow → homepage**: native iTerm tab navigation (Cmd+1, etc.). No custom "back" key.

---

## 14. Distribution: `go install` only ⟳

**Decision**: For MVP, install is `go install github.com/<you>/arteta/cmd/arteta@latest`. No Homebrew, no prebuilt binaries, no installer.

**Why**: MVP users are technical enough to have Go on their machine. Release engineering overhead isn't worth it before the tool is proven.

**Revisit ⟳**: Homebrew tap and prebuilt GitHub Releases binaries once the tool is stable enough to merit broader distribution.

---

## 15. `claude_session_id` propagation

**Decision**: Claude hooks write `session_id` into the status file. Arteta picks it up via fsnotify and persists it to the workflow file. On respawn or revive, runs `claude --resume <session_id>`.

**Why**: One write path, no duplication. Hooks already receive `session_id` for free in their JSON payload.

**Edge case**: First launch before any hook fires → no `session_id` stored yet. If Arteta crashes in that window, revival uses `claude` (no resume). Worst case: a couple of seconds of empty conversation lost. Acceptable.

---

## Revisit checklist (post-MVP)

In rough priority order:

1. **Mid-workflow layout switching** — first thing to revisit, user explicitly flagged.
2. **iTerm Python API** — replace osascript for richer events and fewer staleness issues.
3. **Auto-revive on Arteta start** — if dormant model gets tedious.
4. **SQLite for event history** — when an event-history feature lands.
5. **Plugin system** — terminal adapters, diff adapters (hunk, bat), customisable pane content. Best designed *after* MVP so the abstraction is shaped by real use.
6. **Customisable hotkeys** — config file driven.
7. **Homebrew + prebuilt binaries** — when stable.
8. **Tmux scroll-back persistence** (resurrect-style) — for full restore-after-reboot.
9. **Heartbeat / stuck detection** — improve state machine accuracy.
10. **Multi-instance protection** — pidfile lock if running two TUIs gets confusing.
