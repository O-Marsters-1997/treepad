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

**`workspace`** — Sync configs and generate artifact files across all git worktrees:

```bash
# Sync configs and generate artifact files from the main worktree
tp workspace

# Sync only — skip artifact file generation
tp workspace --sync-only

# Use the current directory as the config source instead of the main worktree
tp workspace --use-current

# Include extra file patterns in the sync
tp workspace --include ".prettierrc" --include ".eslintrc.json"

# Debug what tp is doing
tp --verbose workspace
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
