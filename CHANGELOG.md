# Changelog

## Unreleased

### Added

- **`tp ui`** — interactive BubbleTea TUI fleet commander. Opens a live full-screen alt-screen view of all worktrees with auto-refresh (every 5s) and keyboard-driven actions:
  - `Enter` — exit and cd into the selected worktree (requires shell integration)
  - `s` / `S` — sync selected worktree / sync all worktrees
  - `o` — open artifact file for the selected worktree
  - `y` — yank the selected worktree's path to the system clipboard via OSC-52
  - `r` / `p` — remove worktree / prune merged worktrees (inline confirmation prompt)
  - `?` — toggle key binding help overlay
  - `q` / `Ctrl-C` — quit
  - Requires a TTY; exits with code 2 when stdout is not a terminal

### Removed

- **`tp status --watch`** flag — replaced by `tp ui`. The `--watch` flag was a simple polling loop; `tp ui` supersedes it with a fully interactive TUI.
