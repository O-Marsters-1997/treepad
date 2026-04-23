# tp

A CLI for managing git worktrees — providing a standardised, extensible set of utilities for working with worktrees.

## Overview

`tp` makes it easy to create, navigate, and manage git worktrees from the command line. The aim is to build a consistent, composable toolset around worktree workflows that can be extended as new patterns emerge.

A primary motivation is **parallelising [Claude Code](https://claude.ai/code) instances**: each worktree gets its own isolated working directory, allowing multiple AI coding sessions to run simultaneously on different tasks without interfering with each other. `tp` provides the primitives to spin up, coordinate, and share context between those worktrees.

## Built with

- [urfave/cli v3](https://cli.urfave.org/) — composable, extensible CLI framework for Go
  - Details for v3 can be found at https://cli.urfave.org/v3/getting-started/

## Installation

### Homebrew (recommended)

```bash
brew install O-Marsters-1997/tap/treepad
```

### go install

```bash
go install github.com/O-Marsters-1997/treepad/cmd/tp@latest
```

This installs the `tp` binary.

### From source

```bash
git clone https://github.com/O-Marsters-1997/treepad
cd treepad
just build
```

This produces a `tp` binary in the project root. Move it somewhere on your `$PATH`.

### After installing

Initialise a config file in your repo (optional — tp works with zero config):

```bash
tp config init
```

Run `tp config show` to confirm which config is active.

## Configuration

`tp` works with zero configuration. See [docs/configuration.md](docs/configuration.md) for more info on how to configure it and what the defaults are.

## Usage

```
tp [--verbose | -v] <command> [options]
```

### Main commands

**`sync`** — Sync configs and generate artifact files across all git worktrees:

```bash
# Sync configs and generate artifact files from the main worktree
tp sync

# Sync only — skip artifact file generation
tp sync --sync-only

# Use the current directory as the config source instead of the main worktree
tp sync --use-current

# Include extra file patterns in the sync
tp sync --include ".prettierrc" --include ".eslintrc.json"

# Debug what tp is doing
tp --verbose sync
```

**`new`** — Create a new git worktree with configs synced and artifact file generated:

```bash
# Create a new worktree for branch 'feature-x' branched from main (cd's into it automatically)
tp new feature-x

# Create a worktree from a different base ref
tp new bugfix-y --base develop

# Create a worktree and open the artifact file
tp new feature-z --open

# Stay in the current directory instead of cd-ing into the new worktree
tp new feature-z -c
```

> **Shell integration:** `tp new` prints a cd directive that is acted on by a shell wrapper function. Add the following to your `~/.zshrc` or `~/.bashrc`:
>
> ```sh
> eval "$(tp shell-init)"
> ```

**`from-spec`** — Create a worktree from a GitHub issue, render a prompt, and hand off to an agent:

```bash
# Create a worktree from a GitHub issue spec
tp from-spec feature-x --issue 42

# Create a worktree from a different base ref
tp from-spec bugfix-z --issue 10 --base develop
```

**`from-spec-bulk`** — Create multiple worktrees from GitHub issues with rendered prompts:

```bash
# Create worktrees for issues 12, 14, and 19
tp from-spec-bulk --issues 12,14,19

# Use a branch prefix
tp from-spec-bulk --issues 12,14,19 --branch-prefix feat/

# Branch from a non-default base
tp from-spec-bulk --issues 22,23 --branch-prefix fix/ --base develop
```

**`remove`** — Remove a git worktree and its associated files:

```bash
# Remove a completed feature branch (switch out of it first)
cd ../main-repo
tp remove feature-x
```

**`prune`** — Remove all worktrees whose branches are merged into a base branch, or force-remove all non-main worktrees:

```bash
# Remove all worktrees whose branches are merged into main
tp prune

# Preview without executing
tp prune --dry-run

# Check merges against a different base branch
tp prune --base develop

# Force-remove all non-main worktrees (with confirmation)
tp prune --all
```

**`cd`** — cd into an existing worktree by branch name:

```bash
# cd into an existing worktree (shell integration handles the directory change)
tp cd feature-x
```

> Requires `eval "$(tp shell-init)"` in your shell rc — the same wrapper used by `new`.

**`status`** — List all worktrees with their branch, dirty state, ahead/behind count, and last commit:

```bash
# Show status of all worktrees in a table
tp status

# Emit JSON for scripting or dashboards
tp status --json
```

**`ui`** — Open a live interactive fleet view (requires a TTY):

```bash
# Open the BubbleTea TUI fleet monitor
tp ui
```

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` | cd into selected worktree and exit |
| `s` | Sync selected worktree configs |
| `S` | Sync all worktrees (fleet sync) |
| `o` | Open artifact file for selected worktree |
| `y` | Yank (copy) path of selected worktree to clipboard |
| `r` | Remove selected worktree (with confirmation) |
| `R` | Force-remove selected worktree — discards uncommitted changes and unmerged commits (with confirmation) |
| `p` | Prune merged worktrees (with confirmation) |
| `?` | Toggle key binding help overlay |
| `q` / `Ctrl-C` | Quit |

Requires `eval "$(tp shell-init)"` for the `Enter`→cd action to work.

**`exec`** — Run a command in a specific worktree with full stdio passthrough:

```bash
# Run a script (detected task runner handles it automatically)
tp exec feature-x build

# Run a raw command in the worktree root
tp exec feature-x cargo test

# List available scripts and detected runner for a worktree
tp exec feature-x
```

The `exec` command auto-detects the project task runner (just, npm/pnpm/yarn/bun, make, poetry/uv) by checking for marker files. If the command matches an enumerated script, it routes through the runner (e.g. `just build`, `pnpm run build`). Otherwise, it executes the command directly in the worktree root. Override auto-detection via `[exec] runner = "just"` in `.treepad.toml`.

**`diff`** — Show the diff of a worktree against a base branch:

```bash
# Show diff vs main branch (colored, paged via git config)
tp diff feature-x

# Diff against a different base branch
tp diff feature-x --base develop

# Write a plain patch to a file (no color)
tp diff feature-x -o ~/my-feature.patch

# Show only changed files and line counts
tp diff feature-x -- --stat

# Limit diff to a specific subdirectory
tp diff feature-x -- -- src/
```

The `diff` command uses `git diff <base>...HEAD` three-dot semantics (matches GitHub PR diff view) and respects your git configuration (pager, delta, diff-so-fancy). Inherits color and pager config from the target worktree's git setup. Pass `--output` / `-o` to write an uncolored patch to a file.

**`config`** — Manage tp configuration:

```bash
# Write a default .treepad.toml to the main worktree root
tp config init

# Write config to the global config path
tp config init --global

# Show the resolved config and which source(s) contributed
tp config show
```

See [docs/commands.md](docs/commands.md) for the full command reference.

## Testing

`tp` has two test layers:

- **Unit/integration** — mocked git runner, runs with `just test` (`go test ./...`). Fast inner-loop feedback.
- **End-to-end** — builds `tp` in-process, drives it against a real throwaway git repo per scenario, asserts on stdout, exit codes, and filesystem state. Runs with `just test-e2e` (`go test -tags=e2e ./cmd/tp/...`).

### Adding an e2e test

1. Create `cmd/tp/testdata/script/<name>.txtar`.
2. Start the script with `tp-init-repo` (creates a clean git repo and `cd`s into it).
3. Call `exec tp <command> [args...]` to run the binary.
4. Assert with `stdout <pattern>`, `exists <path>`, `! exists <path>`, or `grep <pattern> <file>`.

See the [testscript docs](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) for the full command reference.

## Development

| Command           | Description                    |
| ----------------- | ------------------------------ |
| `just build`      | Compile the binary             |
| `just test`       | Run all unit/integration tests |
| `just test-e2e`   | Run end-to-end tests           |
| `just lint`       | Run golangci-lint (via Docker) |
| `just fmt`        | Format all Go files            |
| `just ci`         | Lint, build, and test          |
