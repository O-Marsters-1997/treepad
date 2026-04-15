# Treepad Architecture

This document describes the architecture and module organization.

## Entry Point

**`cmd/tp/main.go`** — CLI bootstrap

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
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `Generate()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`

### `new.go`

- `newCommand()` — top-level new command definition
- `runNew(ctx, cmd)` — action handler for creating new worktrees
  - Parses flags: `--base` (default: "main"), `--open`, `--current (-c)`
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `New()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`, `artifact.ExecOpener`

### `shell_init.go`

- `shellInitCommand()` — prints shell wrapper function for `eval "$(tp shell-init)"`
  - Wrapper intercepts `__TREEPAD_CD__` directive from `tp new` and cd's into the new worktree

### `remove.go`

- `removeCommand()` — top-level remove command definition
- `runRemove(ctx, cmd)` — action handler for removing worktrees
  - Parses branch argument (required)
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `Remove()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`

### `prune.go`

- `pruneCommand()` — top-level prune command definition
- `runPrune(ctx, cmd)` — action handler for pruning merged worktrees
  - Parses flags: `--base` (default: "main"), `--dry-run`, `--all` (force-remove all non-main)
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `Prune()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`, `artifact.ExecOpener`

### `config.go`

- `configCommand()` — top-level config command group
- `configInitCommand()` — `tp config init` subcommand
  - Flag: `--global` (write to global config path instead of repo root)
  - Resolves worktrees and main worktree path
  - Calls `config.WriteDefault(dir, global bool)` which writes annotated `.treepad.toml`
- `configShowCommand()` — `tp config show` subcommand
  - Resolves worktrees and main worktree path
  - Calls `config.Show(repoRoot)` to display resolved config and sources

### `status.go`

- `statusCommand()` — top-level status command definition
- `runStatus(ctx, cmd)` — action handler for listing worktree status
  - Parses flag: `--json` (emit JSON instead of table)
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `Status()`
  - Creates instances of `worktree.ExecRunner`, `sync.FileSyncer`, `artifact.ExecOpener`

### `exec.go`

- `execCommand()` — top-level exec command definition
  - Usage: `tp exec <branch> [command] [args...]`
  - Parses branch argument (required), optional command, and variadic args
- `runExec(ctx, cmd)` — action handler for executing commands in worktrees
  - Instantiates `treepad.Service` with `os.Stdout` and `os.Stdin`
  - Calls `Exec()` with branch, command, and args
  - Returns exit code from child process via `cli.Exit("")

## Config Package (`internal/config/`)

Handles TOML configuration file loading, initialization, and display.

### `config.go`

- `Config` struct — root config object with `Sync`, `Artifact`, `Open`, `Exec` fields
- `SyncConfig` struct — contains `Files` (string array)
- `ArtifactConfig` struct — contains `FilenameTemplate` and `ContentTemplate` (text/template strings)
  - `IsZero()` — reports whether artifact is configured
- `OpenConfig` struct — contains `Command` (string slice of template strings)
  - `IsZero()` — reports whether open command is configured
- `ExecConfig` struct — contains `Runner` (string; valid values: just, npm, pnpm, yarn, bun, make, pip, poetry, uv)
  - `IsZero()` — reports whether exec runner is explicitly configured
- `GlobalConfigPath()` — resolves global config path
  - Resolution order: `$TREEPAD_CONFIG` → `$XDG_CONFIG_HOME/treepad/config.toml` → `~/.config/treepad/config.toml`
- `Load(repoRoot)` — loads `.treepad.toml` from repo, falls back to defaults
  - Returns clear error if legacy `.treepad.json` is found: "found .treepad.json; treepad now uses TOML..."
- `defaultSyncFiles()` — built-in list of files to sync (VS Code, Claude, env)

### `init.go`

- `WriteDefault(dir, global)` — writes annotated TOML config file with defaults
  - If `global=true`, writes to global config path
  - If `global=false`, writes `.treepad.toml` to `dir`
  - Returns path of file written
  - Writes `defaultTOML` constant: documented TOML with all sections and produces VS Code `.code-workspace` output by default

### `show.go`

- `Show(repoRoot)` — returns formatted config summary with source info
  - Checks local `.treepad.toml`, then global config, then defaults
  - Returns human-readable string showing which source(s) contributed
  - Uses TOML encoder for output format
- `loadFile(path)` — reads and parses a single `.treepad.toml` file
  - Returns triple: (Config, found bool, error)
  - Handles missing files and parse errors

## Exec Package (`internal/exec/`)

Task runner detection and script enumeration for the `tp exec` command.

### `runner.go`

- `Runner` struct — describes a detected task runner
  - Fields: `Name` (e.g. "just", "pnpm"), `ScriptCmd` (prefix before script name, e.g. ["pnpm", "run"]), `Scripts` (enumerated script names, sorted)
- `Detect(worktreePath, override)` — identifies task runner in worktree
  - Checks for marker files in order: justfile, package.json, Makefile, pyproject.toml
  - Returns error if multiple runners detected and no override specified
  - Uses override if provided (from `[exec] runner` in `.treepad.toml`)
  - Enumerates available scripts via `ListScripts()`
- `ListScripts(worktreePath, runnerName)` — enumerates scripts for a given runner
  - Supports: just, npm, pnpm, yarn, bun, poetry, uv, make, pip
  - Returns nil for runners without script enumeration (make, pip)
- `detectJSManager(dir)` — selects npm/pnpm/yarn/bun based on lockfile presence and package.json `packageManager` field
- `detectPythonRunner(dir)` — selects poetry or uv based on pyproject.toml contents
- `listJustRecipes(dir)` — parses justfile and extracts recipe names (excludes private recipes starting with `_`)
- `listPackageJSONScripts(dir)` — parses package.json and extracts script names
- `listPyprojectScripts(dir)` — parses pyproject.toml and extracts script names (checks `[project]` scripts first, then `[tool.poetry]`)

## Treepad Package (`internal/treepad/`)

Pure business logic for worktree syncing and artifact file generation. Formerly `internal/workspace/`.

### `service.go`

- `Service` struct — coordinates syncing and artifact generation
  - Fields: `runner` (CommandRunner), `syncer` (Syncer), `opener` (Opener), `out` (io.Writer), `in` (io.Reader)
  - `NewService(runner, syncer, opener, out, in)` — constructor takes io.Reader for stdin access
  - `Generate(ctx, GenerateInput)` — generates artifact files and syncs configs
    - Input: `UseCurrentDir`, `SourcePath`, `SyncOnly`, `OutputDir`, `ExtraPatterns`
    - Resolves config source, loads config, syncs files, generates artifact files
  - `New(ctx, NewInput)` — creates new worktree, syncs configs, generates artifact file
    - Input: `Branch`, `Base`, `Open`, `Current`, `OutputDir`
    - Emits `__TREEPAD_CD__\t<path>` to output unless `Current` is true
  - `Remove(ctx, RemoveInput)` — removes worktree, artifact file, and branch
    - Input: `Branch`, `OutputDir`, `Cwd` (for testing)
    - Pre-flight guards: prevents removing main worktree, prevents removing from within the target worktree
    - Three-step removal: git worktree remove → delete artifact file → git branch -d
  - `Prune(ctx, PruneInput)` — batch removes worktrees or force-removes all non-main
    - Input: `Base`, `OutputDir`, `DryRun`, `All` (force-remove all non-main), `Cwd` (for testing)
    - If `All=true`: validates running from main worktree, lists all non-main worktrees, prompts for confirmation, force-removes all via `forceRemoveWorktree()`
    - If `All=false`: finds merged branches, filters out main/detached/current worktree
    - Executes removals by default; `DryRun: true` previews without removing
    - Returns error if any removals fail (after attempting all)
  - `Status(ctx, StatusInput)` — lists all worktrees with repo-wide status snapshot
    - Input: `JSON` (emit JSON instead of table), `OutputDir` (for artifact path resolution)
    - Output: builds `[]StatusRow` with branch, dirty state, ahead/behind count, last commit info, artifact mtime
    - Renders as aligned table via `text/tabwriter` by default, or JSON array with `--json` flag
  - `Exec(ctx, ExecInput)` — runs a command in a named worktree with full stdio passthrough
    - Input: `Branch`, `Command`, `Args`, `Cwd` (for testing), `Runner` (PassthroughRunner override)
    - Detects task runner via `internal/exec.Detect()` using config override if available
    - If `Command` is empty: lists detected runner and available scripts, returns
    - Otherwise: builds command via `buildCommand()` (routes through runner if command matches enumerated script)
    - Executes via PassthroughRunner and returns child process exit code (non-zero does not produce an error)
- Private helpers:
  - `removeWorktree(ctx, target, mainWT, outputDir)` — removes a single worktree (merge-safe removal), deletes artifact, deletes branch
  - `forceRemoveWorktree(ctx, target, mainWT, outputDir)` — force-removes a worktree via `git worktree remove --force`, deletes artifact, force-deletes branch via `git branch -D`
  - `pruneAll(ctx, worktrees, mainWT, outputDir, cwd, dryRun)` — helper for `--all` mode; lists candidates, prompts user, force-removes each
  - `listWorktrees(ctx)` — lists all worktrees in repo
  - `resolveOutputDir(explicit, repoSlug)` — resolves artifact output directory
  - `loadAndSync(sourceDir, extraPatterns, targets)` — loads config and syncs to targets; returns `config.Config` so artifact config is available
  - `printScripts(runner)` — prints runner name and enumerated scripts to output
  - `buildCommand(runner, command, extraArgs)` — returns executable name and arguments, routing through runner if command matches a known script (adds `--` for npm with extra args)

### `source.go`

- `ResolveSourceDir(useCurrentDir, sourcePath, cwd, worktrees)` — determines config source directory

### `opener.go`

- `Opener` interface — abstracts artifact file opening
- `ExecOpener` struct — implementation that opens files/commands via `artifact.ExecOpener`

## Worktree Package (`internal/worktree/`)

Wrapper around git worktree operations.

### `worktree.go`

- `Worktree` struct — represents a single git worktree with Path, Branch, etc.
- `CommitInfo` struct — summary of a git commit: `ShortSHA`, `Subject`, `Committed` (time.Time)
- `ExecRunner` — executes `git` commands (dependency injection)
- `List(ctx, runner)` — lists all worktrees in a repo
- `MainWorktree(worktrees)` — returns the main worktree (contains `.git` directory)
- `MergedBranches(ctx, runner, base string)` — returns local branches merged into base (excluding base itself)
  - Runs `git branch --merged <base> --format=%(refname:short)`
  - Returns string slice of branch names
- `Dirty(ctx, runner, path)` — reports whether worktree at path has uncommitted changes
  - Runs `git -C <path> status --porcelain`
- `AheadBehind(ctx, runner, path)` — counts commits ahead/behind upstream
  - First checks if upstream exists via `git rev-parse --abbrev-ref @{upstream}`
  - Returns `(ahead, behind, hasUpstream=false, nil)` if no upstream configured (not an error)
  - If upstream exists, runs `git rev-list --left-right --count HEAD...@{upstream}`
- `LastCommit(ctx, runner, path)` — returns info about HEAD commit
  - Runs `git log -1 --format=%h%x00%s%x00%cI`
  - Returns empty `CommitInfo{}` if no commits; error on timestamp parse failure

## Sync Package (`internal/sync/`)

File synchronization across worktrees.

### `sync.go`

- `FileSyncer` — copies files from source to target directories
- Glob pattern matching and batch copying

## Artifact Package (`internal/artifact/`)

Per-worktree file generation from config-supplied templates. No editor names in code — callers supply templates via `.treepad.toml`.

### `artifact.go`

- `Spec` struct — describes artifact generation: `FilenameTemplate` and `ContentTemplate` (both text/template strings)
  - `IsZero()` — reports whether artifact is configured
- `Worktree` struct — template-friendly view: `.Name` (sanitized), `.Path` (absolute), `.RelPath` (relative to output), `.Branch` (raw)
- `TemplateData` struct — context available to templates: `.Slug`, `.Branch`, `.Worktrees`, `.OutputDir`
- `RenderFilename(spec, data)` — executes filename template
- `RenderContent(spec, data)` — executes content template and returns bytes
- `Path(spec, outputDir, data)` — returns absolute path artifact would be written to
- `Write(spec, outputDir, data)` — renders and writes artifact file
- `ToWorktree(branch, path, outputDir)` — builds template-friendly `Worktree` view from raw path/branch

### `open.go`

- `Opener` interface — abstracts artifact file opening
- `ExecOpener` struct — implementation that renders command templates and runs them
  - `Open(ctx, spec, cmd, data)` — renders command template, executes via `CommandRunner`
- `CommandRunner` interface — duck-typed by `worktree.ExecRunner`

## Slug Package (`internal/slug/`)

Utility for deriving short identifiers from repository paths.

### `slug.go`

- `Slug(repoPath)` — generates slug for workspace file naming

## CLI Command Structure

```
tp [--verbose] <command>
├── workspace [options] [source-path]
│   ├── --use-current (-c)
│   ├── --sync-only
│   ├── --output-dir (-o)
│   └── --include (repeatable)
├── new [options] <branch>
│   ├── --base (default: main)
│   ├── --open (-o)
│   └── --current (-c)
├── shell-init
├── remove <branch>
├── prune [options]
│   ├── --base (default: main)
│   ├── --dry-run
│   └── --all (force-remove all non-main, must be from main)
├── status [options]
│   └── --json
├── exec <branch> [command] [args...]
│   └── Auto-detects task runner (just, npm, pnpm, yarn, bun, make, poetry, uv)
│       Routes through runner if command matches enumerated script
│       Override with [exec] runner in .treepad.toml
└── config
    ├── init [--global]
    └── show
```

## Key Design Decisions

1. **CLI Separation** — All CLI wiring (`internal/commands/`) is separate from business logic. Packages like `treepad`, `config`, `worktree`, `artifact`, and `sync` contain pure logic without CLI dependencies.

2. **Dependency Injection** — `CommandRunner` and `Syncer` are injected to enable testing without external commands.

3. **Global Config** — Follows XDG Base Directory spec with fallback to `TREEPAD_CONFIG` env var.

4. **Config Defaults** — Zero-config experience; sensible defaults (VS Code `.code-workspace` files) are built-in and used when `.treepad.toml` is absent.

5. **Config Resolution** — Three-tier lookup in `Show()`:
   - Local `.treepad.toml` (highest priority)
   - Global config (medium priority)
   - Built-in defaults (fallback)

6. **Editor Agnosticism** — No editor names in Go code. Artifact filename, content, and open command are all text/template strings in `.treepad.toml`. VS Code is the default (baked into defaults). Other editors configure via config only.

## Data Flow Example: `tp workspace`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.workspaceCommand()` defines CLI interface
3. `runWorkspace()` parses args, instantiates `treepad.Service`, calls `Generate()`
4. `Service.Generate()` resolves source, loads config via `config.Load()`, syncs files via `sync.FileSyncer`
5. Optionally generates artifact files via `artifact.Write()`

## Data Flow Example: `tp new`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.newCommand()` defines CLI interface
3. `runNew()` parses args, instantiates `treepad.Service`, calls `New()`
4. `Service.New()` runs `git worktree add`, syncs configs, generates artifact file
5. Optionally opens artifact file via `artifact.ExecOpener`
6. Unless `--current` / `-c` is passed, emits `__TREEPAD_CD__\t<path>` to stdout
7. Shell wrapper (from `tp shell-init`) intercepts the directive and cd's into the new worktree

## Data Flow Example: `tp config init --global`

1. `cmd/tp/main.go` initializes CLI
2. `commands.config.configInitCommand()` handles the action
3. If `--global` flag is set, calls `config.WriteDefault("", true)`
4. Otherwise, lists worktrees via `worktree.List()`, gets main worktree, calls `config.WriteDefault(mainPath, false)`
5. File is written to global or local path

## Data Flow Example: `tp config show`

1. `cmd/tp/main.go` initializes CLI
2. `commands.config.configShowCommand()` handles the action
3. Lists worktrees via `worktree.List()`, gets main worktree path
4. Calls `config.Show(mainPath)`
5. `Show()` checks local, global, and defaults; returns formatted summary with sources

## Data Flow Example: `tp remove <branch>`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.removeCommand()` defines CLI interface
3. `runRemove()` parses branch argument, instantiates `treepad.Service`, calls `Remove()`
4. `Service.Remove()` executes three steps:
   - Lists all worktrees, validates branch exists and is not main
   - Pre-flight guard: ensures cwd is not inside the target worktree
   - Removes git worktree via `git worktree remove`
   - Deletes artifact file from output directory (missing file is not an error)
   - Deletes branch locally via `git branch -d`

## Data Flow Example: `tp prune [--base main] [--dry-run] [--all]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.pruneCommand()` defines CLI interface
3. `runPrune()` parses flags, instantiates `treepad.Service`, calls `Prune()`
4. `Service.Prune()` dispatches based on `All` flag:
   - **If `--all` flag set:** calls `pruneAll()`
     - Validates running from main worktree (safety guard)
     - Lists all non-main worktrees
     - Displays candidates and prompts user: "continue? [y/N]:"
     - If confirmed, force-removes each via `forceRemoveWorktree()` (git worktree remove --force, git branch -D)
     - If not confirmed, outputs "aborted" and returns
   - **If `--all` flag not set:** standard merge-based mode
     - Lists all worktrees via `worktree.List()`
     - Gets merged branches via `worktree.MergedBranches(ctx, runner, base)`
     - Filters candidates: merged, not main, not detached, not current worktree
     - If `--dry-run` flag set, prints candidates and returns
     - Otherwise removes each candidate via `removeWorktree()` (safe removal via git worktree remove, git branch -d)
   - Returns error if any removals failed

## Data Flow Example: `tp status [--json]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.statusCommand()` defines CLI interface
3. `runStatus()` parses flags, instantiates `treepad.Service`, calls `Status()`
4. `Service.Status()` executes:
   - Lists all worktrees via `worktree.List()`
   - For each worktree, probes:
     - `worktree.Dirty()` — checks `git status --porcelain`
     - `worktree.AheadBehind()` — compares vs `@{upstream}` if configured
     - `worktree.LastCommit()` — fetches HEAD commit info
   - Computes artifact file path via `artifact.Path()` and checks mtime
   - Builds `[]StatusRow` with all collected info
   - If `--json` flag set, encodes as JSON array; otherwise renders via `text/tabwriter` table
   - Writes to `s.out` and returns

## Data Flow Example: `tp exec <branch> [command] [args...]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.execCommand()` defines CLI interface
3. `runExec()` parses branch, optional command, and variadic args; instantiates `treepad.Service`, calls `Exec()`
4. `Service.Exec()` executes:
   - Lists all worktrees via `worktree.List()` and finds target by branch
   - Checks if already in target worktree (emits warning if so)
   - Loads config and detects task runner via `exec.Detect()` (uses override if configured)
   - If command is empty: prints runner name and available scripts via `printScripts()`
   - Otherwise: builds command via `buildCommand()`:
     - If command matches enumerated script: routes through runner (e.g. "build" → ["pnpm", "run", "build"])
     - Otherwise: executes raw command in worktree root
   - Executes via PassthroughRunner with full stdio passthrough (inherits stdin/stdout/stderr from tp process)
   - Returns exit code from child process (non-zero exit does not produce an error; launch failures do)

---

**Last Updated:** April 15, 2026 (added `tp exec` command with task runner detection, script enumeration, and full stdio passthrough; added `internal/exec` package and `service_exec.go`; added `ExecConfig` to config package)
