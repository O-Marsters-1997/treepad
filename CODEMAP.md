# Treepad Architecture

This document describes the architecture and module organization.

## Entry Point

**`cmd/workspace/main.go`** — CLI bootstrap

- Initializes the `urfave/cli` v3 application with the verbose flag
- Calls `commands.Router()` to get all available CLI commands
- Runs the CLI with context and os.Args

## Commands Package (`internal/commands/`)

Central location for all CLI command definitions. Separates CLI wiring from business logic.

### `router.go`

- `Router()` — returns `[]*cli.Command` with all top-level commands
- Routes to workspace and config command groups

### `workspace.go`

- `workspaceCommand()` — top-level workspace command definition
- `runWorkspace(ctx, cmd)` — action handler for workspace operations
  - Parses flags: `--use-current`, `--sync-only`, `--output-dir`, `--include`
  - Instantiates `workspace.Orchestrator` and calls `Run()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`

### `config.go`

- `configCommand()` — top-level config command group
- `configInitCommand()` — `treepad config init` subcommand
  - Flag: `--global` (write to global config path instead of repo root)
  - Resolves worktrees and main worktree path
  - Calls `config.WriteDefault(dir, global bool)`
- `configShowCommand()` — `treepad config show` subcommand
  - Resolves worktrees and main worktree path
  - Calls `config.Show(repoRoot)` to display resolved config and sources

## Config Package (`internal/config/`)

Handles configuration file loading, initialization, and display.

### `config.go`

- `Config` struct — root config object with `Sync` field
- `SyncConfig` struct — contains `Files` (string array)
- `GlobalConfigPath()` — resolves global config path
  - Resolution order: `$TREEPAD_CONFIG` → `$XDG_CONFIG_HOME/treepad/config.json` → `~/.config/treepad/config.json`
- `Load(repoRoot)` — loads `.treepad.json` from repo, falls back to defaults
- `defaultSyncFiles()` — built-in list of files to sync (VS Code, Claude, env)

### `init.go`

- `WriteDefault(dir, global)` — writes config file with defaults
  - If `global=true`, writes to global config path
  - If `global=false`, writes `.treepad.json` to `dir`
  - Returns path of file written

### `show.go`

- `Show(repoRoot)` — returns formatted config summary with source info
  - Checks local `.treepad.json`, then global config, then defaults
  - Returns human-readable string showing which source(s) contributed
- `loadFile(path)` — reads and parses a single config JSON file
  - Returns triple: (Config, found bool, error)
  - Handles missing files and parse errors

## Workspace Package (`internal/workspace/`)

Pure business logic for worktree syncing and workspace file generation.

### `orchestrator.go`

- `Orchestrator` struct — coordinates syncing and generation
- `RunInput` struct — input parameters
  - `UseCurrentDir`, `SourcePath`, `SyncOnly`, `OutputDir`, `ExtraPatterns`
- `Run(ctx, input)` — main orchestration logic
  - Resolves config source (explicit path, current dir, or main worktree)
  - Loads config and applies extra patterns
  - Syncs files across worktrees
  - Optionally generates `.code-workspace` files

### `source.go`

- Helpers for resolving the config source directory

## Worktree Package (`internal/worktree/`)

Wrapper around git worktree operations.

### `worktree.go`

- `Worktree` struct — represents a single git worktree with Path, Branch, etc.
- `ExecRunner` — executes `git` commands (dependency injection)
- `List(ctx, runner)` — lists all worktrees in a repo
- `MainWorktree(worktrees)` — returns the main worktree (contains `.git` directory)

## Sync Package (`internal/sync/`)

File synchronization across worktrees.

### `sync.go`

- `FileSyncer` — copies files from source to target directories
- Glob pattern matching and batch copying

## Codeworkspace Package (`internal/codeworkspace/`)

VS Code `.code-workspace` file generation.

### `generate.go`

- `Generate(repo, outputDir)` — creates workspace files for all worktrees

### `extensions.go`

- Helpers for syncing VSCode extensions

## Slug Package (`internal/slug/`)

Utility for deriving short identifiers from repository paths.

### `slug.go`

- `Slug(repoPath)` — generates slug for workspace file naming

## CLI Command Structure

```
treepad [--verbose] <command>
├── workspace [options] [source-path]
│   ├── --use-current (-c)
│   ├── --sync-only
│   ├── --output-dir (-o)
│   └── --include (repeatable)
└── config
    ├── init [--global]
    └── show
```

## Key Design Decisions

1. **CLI Separation** — All CLI wiring (`internal/commands/`) is separate from business logic. Packages like `workspace`, `config`, `worktree`, and `sync` contain pure logic without CLI dependencies.

2. **Dependency Injection** — `ExecRunner` and `FileSyncer` are injected to enable testing without external commands.

3. **Global Config** — Follows XDG Base Directory spec with fallback to `TREEPAD_CONFIG` env var.

4. **Config Defaults** — Zero-config experience; sensible defaults are built-in and used when `.treepad.json` is absent.

5. **Config Resolution** — Three-tier lookup in `Show()`:
   - Local `.treepad.json` (highest priority)
   - Global config (medium priority)
   - Built-in defaults (fallback)

## Data Flow Example: `treepad workspace`

1. `main.go` parses flags and calls `commands.Router()`
2. `commands.workspace.workspaceCommand()` defines CLI interface
3. `runWorkspace()` parses args and calls `workspace.Orchestrator.Run()`
4. `Orchestrator` resolves source, loads config via `config.Load()`, syncs files via `sync.FileSyncer`
5. Optionally generates workspace files via `codeworkspace.Generate()`

## Data Flow Example: `treepad config init --global`

1. `main.go` initializes CLI
2. `commands.config.configInitCommand()` handles the action
3. If `--global` flag is set, calls `config.WriteDefault("", true)`
4. Otherwise, lists worktrees via `worktree.List()`, gets main worktree, calls `config.WriteDefault(mainPath, false)`
5. File is written to global or local path

## Data Flow Example: `treepad config show`

1. `main.go` initializes CLI
2. `commands.config.configShowCommand()` handles the action
3. Lists worktrees via `worktree.List()`, gets main worktree path
4. Calls `config.Show(mainPath)`
5. `Show()` checks local, global, and defaults; returns formatted summary with sources

---

**Last Updated:** April 13, 2026
