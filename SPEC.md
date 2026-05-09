# Summary

Arteta is a TUI-based manager for managing multiple Claude Code sessions.

# Problem Statement

Managing Claude Code sessions using a traditional Tmux and Neovim setup can be tricky. Problems include:

* It's hard for the user to know at a glance which Claude Code sessions have completed, navigate to them easily, and continue the process.
* It's difficult to maintain context across different Claude Code sessions, and context switching incurs cognitive cost.

Neovim and Tmux, while useful, are not the most ideal format for managing multiple Claude sessions.

That said, I still believe Neovim and Tmux are the ideal building blocks for interacting with Claude sessions. Tmux offers managed sessions, and Neovim is a fuss-free way to keep all interaction with Claude CLI-based.

# Design Philosophy

Arteta simplifies the process by building a TUI that manages Claude sessions under the hood using Tmux and Neovim. It is built in a language that compiles to executables — Go or Rust.

Arteta only assumes two things: the user has Neovim and Tmux installed.

Arteta is designed to be forkable. It provides barebones functionality based on the UI flow described below, but power users can customise via plugins, and things are designed to be forkable.

For example, the initial version is built for iTerm and runs within iTerm2, but the interface is designed well enough that users can swap in Ghostty or their terminal of choice. Similarly — described in more detail later — when viewing code diffs, if a user wants their own diff experience, it should be as easy as swapping in a plugin and rebuilding.

Keyboard-first interaction with sensible hotkeys.

# User Experience

This is based on my current frictions when using Claude over multiple sessions in the terminal.

## Handling progression

I currently use Peon Ping to figure out when a session has completed. This isn't ideal, because I still need to navigate to the session that's asking for input or has completed. I want a central "home" page to see all my active sessions — which are running, completed, and which require my input. For those requiring input, I should see a small summary of what's being asked (a question, a permission, etc.). This can just be a truncated version of the message Claude sent.

## Switching between sessions

Right now, I have a Tmux session for each workflow I'm working on. Roughly speaking, this could be a git branch — but conceptually, it's different views of the same thing.

I have a separate view for the terminal, the Claude Code session, Neovim of the code, and so on. In general, I want easy access to these via the TUI. For example, I could switch between viewing the terminal, the Claude Code session, Neovim of the code, and the diff in a sidebar. It should also be flexible with split-screen: I might want a half-and-half of the Claude Code interactive session and a terminal, or Neovim, or the diff. Or I might want a quadrant view. Let's start with single window, vertical split, horizontal split, and quadrant view.

Each workflow should also be named descriptively. Arteta must require the user to name a session so we can switch back and forth easily between workflows. You can incorporate useful libraries for viewing diffs like [hunk](https://github.com/modem-dev/hunk), but this can also be a plugin — the base case can always just be a prettified `git diff`, or `bat` (better cat).

## UI

We always start with the Arteta homepage. Clicking on a workflow opens an iTerm tab named after the workflow. If I navigate back to the homepage and click on the same workflow, it should focus the same tab if it's already open, then allow me to cycle between the workflow UI mentioned earlier.

Once my work is done, I close the tab and go back to the homepage to tackle the next thing. If I tackle something in Workflow A, close it, then reopen it later, the views should be preserved — I can see my Claude Code session and the terminal with all the commands I ran. Arteta will give a command to close a workflow once I'm done with it.

Arteta should survive restarts. I have tmux-resurrect locally; maybe use that as a base.
