# treepad

A CLI for managing git worktrees — providing a standardised, extensible set of utilities for working with worktrees.

## Overview

`treepad` makes it easy to create, navigate, and manage git worktrees from the command line. The aim is to build a consistent, composable toolset around worktree workflows that can be extended as new patterns emerge.

A primary motivation is **parallelising [Claude Code](https://claude.ai/code) instances**: each worktree gets its own isolated working directory, allowing multiple AI coding sessions to run simultaneously on different tasks without interfering with each other. `treepad` provides the primitives to spin up, coordinate, and share context between those worktrees.

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
go install github.com/O-Marsters-1997/treepad/cmd/treepad@latest
```

### From source

```bash
git clone https://github.com/O-Marsters-1997/treepad
cd treepad
just build
```

This produces a `treepad` binary in the project root. Move it somewhere on your `$PATH`.

### After installing

Initialise a config file in your repo (optional — treepad works with zero config):

```bash
treepad config init
```

Run `treepad config show` to confirm which config is active.

## Configuration

`treepad` works with zero configuration. See [docs/configuration.md](docs/configuration.md) for more info on how to configure it and what the defaults are.

## Usage

```
treepad [--verbose | -v] <command> [options]
```

### Main commands

**`workspace`** — Sync configs and generate artifact files across all git worktrees:

```bash
# Sync configs and generate artifact files from the main worktree
treepad workspace

# Sync only — skip artifact file generation
treepad workspace --sync-only

# Use the current directory as the config source instead of the main worktree
treepad workspace --use-current

# Include extra file patterns in the sync
treepad workspace --include ".prettierrc" --include ".eslintrc.json"

# Debug what treepad is doing
treepad --verbose workspace
```

**`create`** — Create a new git worktree with configs synced and artifact file generated:

```bash
# Create a new worktree for branch 'feature-x' branched from main
treepad create feature-x

# Create a worktree from a different base ref
treepad create bugfix-y --base develop

# Create a worktree and open the artifact file
treepad create feature-z --open
```

**`remove`** — Remove a git worktree and its associated files:

```bash
# Remove a completed feature branch (switch out of it first)
cd ../main-repo
treepad remove feature-x
```

**`prune`** — Remove all worktrees whose branches are merged into a base branch:

```bash
# Remove all worktrees whose branches are merged into main
treepad prune

# Preview without executing
treepad prune --dry-run

# Check merges against a different base branch
treepad prune --base develop
```

**`status`** — List all worktrees with their branch, dirty state, ahead/behind count, and last commit:

```bash
# Show status of all worktrees in a table
treepad status

# Emit JSON for scripting or dashboards
treepad status --json
```

**`config`** — Manage treepad configuration:

```bash
# Write a default .treepad.toml to the main worktree root
treepad config init

# Write config to the global config path
treepad config init --global

# Show the resolved config and which source(s) contributed
treepad config show
```

See [docs/commands.md](docs/commands.md) for the full command reference.

## Development

| Command      | Description                    |
| ------------ | ------------------------------ |
| `just build` | Compile the binary             |
| `just test`  | Run all tests                  |
| `just lint`  | Run golangci-lint (via Docker) |
| `just fmt`   | Format all Go files            |
| `just ci`    | Lint, build, and test          |
