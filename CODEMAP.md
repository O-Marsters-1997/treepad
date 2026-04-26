# Treepad Architecture

This document describes the architecture and module organization.

## Entry Point

**`cmd/tp/main.go`** — CLI bootstrap

- Initializes the `urfave/cli` v3 application with two global flags:
  - `--verbose` / `-v` — sets `slog` to `DEBUG` level on stderr
  - `--profile` — attaches a `profile.Recorder` to `cmd.Metadata["profiler"]`; `After` hook calls `rec.Summary()` to print a per-stage timing table to stderr
- `commandDeps()` (in `commands/base.go`) extracts the recorder from metadata and sets `d.Profiler`
- Calls `commands.Router()` to get all available CLI commands
- Runs the CLI with a signal-notified context (SIGINT/SIGTERM)

## Commands Package (`internal/commands/`)

Central location for all CLI command definitions. Separates CLI wiring from business logic.

### `router.go`

- `Router()` — returns `[]*cli.Command` with all top-level commands registered: sync, config, new, shell-init, remove, prune, status, ui, cd, base, doctor, exec, diff, from-spec, from-spec-bulk

### `base.go`

- `commandDeps(cmd)` — builds production `deps.Deps`; if a `profile.Recorder` is stored in `cmd.Root().Metadata["profiler"]`, wires it into `d.Profiler` so lifecycle operations emit timed stages
- `requireBranch(cmd)` — extracts the first positional argument as a branch name or returns an error
- `baseCommand()` — `tp base` command: calls `cd.Base()` to emit a cd directive for the main worktree

### `cd.go`

- `cdCommand()` — `tp cd <branch>` command: calls `cd.CD()` to emit a cd directive for the named worktree
  - Shell completes with available branch names via `completeWorktreeBranch`

### `completion.go`

- `completeWorktreeBranch(ctx, cmd)` — shell completion helper: prints all non-detached branch names
- `completeRemoveBranch(ctx, cmd)` — shell completion helper: prints non-main, non-detached branches
- `completeExecBranch(ctx, cmd)` — shell completion helper: prints branches only when no arg is present

### `shell_init.go`

- `shellInitCommand()` — prints the shell wrapper function for `eval "$(tp shell-init)"`
  - Wrapper captures `TREEPAD_CD_FD=3` — the binary writes the cd path to fd 3 (captured separately), while stdout flows live to the terminal via fd 4
  - `tp cd -` toggles back to the previous worktree using `$TP_PREV_WORKTREE` env var
  - `$TP_PREV_WORKTREE` is updated before every cd to enable toggle-back functionality

### `sync.go`

- `syncCommand()` — `tp sync [options] [source-path]` command definition
  - Flags: `--current` / `-c` / `--use-current` (alias), `--sync-only`, `--output-dir` / `-o`, `--include`
- `runSync(ctx, cmd)` — calls `treepad.Generate()` with parsed flags

### `new.go`

- `newCommand()` — `tp new [options] <branch>` command definition
  - Flags: `--base` / `-b` (default: "main"), `--open` / `-o`, `--current` / `-c`
- `runNew(ctx, cmd)` — calls `lifecycle.New()` with parsed flags

### `remove.go`

- `removeCommand()` — `tp remove <branch>` command definition
  - Shell completes with `completeRemoveBranch`
- `runRemove(ctx, cmd)` — calls `lifecycle.Remove()` with branch

### `prune.go`

- `pruneCommand()` — `tp prune [options]` command definition
  - Flags: `--base` / `-b` (default: "main"), `--dry-run` / `-n`, `--all` / `-a`, `--yes` / `-y`
- `runPrune(ctx, cmd)` — calls `lifecycle.Prune()` with parsed flags

### `doctor.go`

- `doctorCommand()` — `tp doctor [options]` command definition
  - Flags: `--json` / `-j`, `--stale-days` (default: 30), `--base` / `-b` (default: "main"), `--offline`, `--strict`
- `runDoctor(ctx, cmd)` — calls `treepad.Doctor()` with parsed flags

### `config.go`

- `configCommand()` — `tp config` command group
- `configInitCommand()` — `tp config init [--global]`
  - Calls `treepad.ConfigInit()` which delegates to `config.WriteDefault()`
- `configShowCommand()` — `tp config show`
  - Calls `treepad.ConfigShow()` which delegates to `config.Show()`

### `status.go`

- `statusCommand()` — `tp status [--json]` command definition
- `runStatus(ctx, cmd)` — calls `treepad.Status()` via `DefaultDeps()`

### `exec.go`

- `execCommand()` — `tp exec <branch> [command] [args...]` command definition
- `runExec(ctx, cmd)` — calls `treepad.Exec()`, returns child process exit code via `cli.Exit("")`

### `diff.go`

- `diffCommand()` — `tp diff [options] <branch> [-- <git-diff-args>...]` command definition
  - Flags: `--base` / `-b`, `--output` / `-o`
- `runDiff(ctx, cmd)` — calls `treepad.Diff()` with branch, base, output path, and extra args

## Config Package (`internal/config/`)

Handles TOML configuration file loading, initialization, and display.

### `config.go`

- `Config` struct — root config object with `Sync`, `Artifact`, `Open`, `Hooks`, `Exec`, `FromSpec`, `Diff` fields
- `SyncConfig` struct — contains `Include` (string array of gitignore-style patterns)
- `ArtifactConfig` struct — contains `FilenameTemplate` and `ContentTemplate` (text/template strings)
  - `IsZero()` — reports whether artifact is configured
- `OpenConfig` struct — contains `Command` (string slice of template strings)
  - `IsZero()` — reports whether open command is configured
- `DiffConfig` struct — contains `Base` (string; default `"origin/main"`)
  - `IsZero()` — reports whether diff configuration is present
- `ExecConfig` struct — contains `Runner` (string; valid values: just, npm, pnpm, yarn, bun, make, pip, poetry, uv)
  - `IsZero()` — reports whether exec runner is explicitly configured
- `FromSpecConfig` struct — contains `Skills`, `AgentCommand`
  - `IsZero()` — reports whether from-spec configuration is present
- `GlobalConfigPath()` — resolves global config path
  - Resolution order: `$TREEPAD_CONFIG` → `$XDG_CONFIG_HOME/treepad/config.toml` → `~/.config/treepad/config.toml`
- `Load(repoRoot)` — loads `.treepad.toml` from repo, falls back to defaults
  - Returns clear error if legacy `.treepad.json` is found: "found .treepad.json; treepad now uses TOML..."
  - Merges file config with defaults: explicitly configured sections override defaults (IsZero check per section)
- `defaultSyncInclude()` — built-in list of files to sync (VS Code, Claude, env)

### `init.go`

- `WriteDefault(dir, global)` — writes annotated TOML config file with defaults
  - If `global=true`, writes to global config path
  - If `global=false`, writes `.treepad.toml` to `dir`
  - Returns path of file written
  - Writes `defaultTOML` constant: documented TOML with all sections and produces VS Code `.code-workspace` output by default

### `artifact.go`

- `(ArtifactConfig).Spec() artifact.Spec` — converts ArtifactConfig to artifact.Spec
- `MakeTemplateData(repoSlug, branch, worktreePath, outputDir) artifact.TemplateData` — builds template data for a single worktree
- `ResolveArtifactPath(ArtifactConfig, repoSlug, branch, wtPath, outputDir) (path string, ok bool, error)` — returns the absolute artifact path for a worktree; `ok=false` when no filename template is configured

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
- `Resolve(fs fs.FS, override)` — identifies task runner using `fs.FS` interface
  - Accepts `fs.FS` instead of real filesystem paths (enables testing and flexible filesystem backends)
  - Checks for marker files in order: justfile, package.json, Makefile, pyproject.toml
  - Returns error if multiple runners detected and no override specified
  - Uses override if provided (from `[exec] runner` in `.treepad.toml`)
  - Enumerates available scripts via pure `ListScripts()` (no side effects)
- `ListScripts(worktreePath, runnerName)` — enumerates scripts for a given runner
  - Supports: just, npm, pnpm, yarn, bun, poetry, uv, make, pip
  - Returns nil for runners without script enumeration (make, pip)
- `detectJSManager(dir)` — selects npm/pnpm/yarn/bun based on lockfile presence and package.json `packageManager` field
- `detectPythonRunner(dir)` — selects poetry or uv based on pyproject.toml contents
- `listJustRecipes(dir)` — parses justfile and extracts recipe names (excludes private recipes starting with `_`)
- `listPackageJSONScripts(dir)` — parses package.json and extracts script names
- `listPyprojectScripts(dir)` — parses pyproject.toml and extracts script names (checks `[project]` scripts first, then `[tool.poetry]`)

## Treepad Package (`internal/treepad/`)

Business logic entry points. Each public function is a standalone top-level function (no Service struct). All share `deps.Deps` for dependency injection.

### `generate.go`

- `GenerateInput` struct — `UseCurrentDir`, `SourcePath`, `SyncOnly`, `OutputDir`, `ExtraPatterns`, `Branch` (empty = fleet-wide)
- `Generate(ctx, deps.Deps, GenerateInput)` — syncs configs and optionally generates artifact files
  - Resolves source directory via `ResolveSourceDir()`
  - Builds `[]lifecycle.SyncTarget` from all worktrees except source
  - If `Branch` is set, filters targets to the single named branch
  - Calls `lifecycle.LoadAndSync()` then iterates worktrees to call `artifact.Write()` (skipped if `SyncOnly`)

### `exec.go`

- `ExecInput` struct — `Branch`, `Command`, `Args`, `Cwd` (testing override), `Runner` (PassthroughRunner override)
- `Exec(ctx, deps.Deps, ExecInput) (int, error)` — runs a command in the named worktree with full stdio passthrough
  - Locates worktree by branch; warns if already inside it
  - Detects task runner via `exec.Resolve()` (uses config override if set)
  - If `Command` is empty: prints detected runner and scripts via `printScripts()`
  - Otherwise: builds command via `buildCommand()` (routes through runner if command is a known script)
  - Returns child process exit code (non-zero does not produce an error)

### `diff.go`

- `DiffInput` struct — `Branch`, `Base` (empty → from config or `"origin/main"`), `OutputFile`, `ExtraArgs`, `Runner`
- `Diff(ctx, deps.Deps, DiffInput) error` — diffs target worktree against base using three-dot semantics (`<base>...HEAD`)
  - Resolves base: calls `resolveBase(worktrees)` → loads `config.Diff.Base` from main worktree, fallback `"origin/main"`
  - OutputFile set: captures `git -C <path> diff --no-color <base>...HEAD`, writes patch to file
  - OutputFile empty: executes `git diff` via PassthroughRunner (inherits pager, color, delta config)

### `doctor.go`

- `DoctorInput` struct — `JSON`, `StaleDays` (default 30), `Base`, `Offline`, `Strict`, `OutputDir`
- `DoctorFinding` struct — `Branch`, `Path`, `Kind`, `Detail` (JSON-serialisable)
- `Doctor(ctx, deps.Deps, DoctorInput) error` — reports cross-worktree health findings
  - Per worktree runs: `doctorCheckAge` (stale/dirty-old), `doctorCheckMerged` (merged-present), `doctorCheckRemoteGone` (remote-gone; skipped when `Offline`), `doctorCheckArtifact` (artifact-missing), `doctorCheckConfigDrift` (config-drift vs main)
  - Prunable worktrees get a `prunable` finding immediately
  - `Strict=true` returns an error if any findings were reported

### `status.go`

- `StatusInput` struct — `JSON`, `OutputDir`
- `StatusRow` struct — branch, path, is_main, dirty, ahead, behind, has_upstream, last_commit, artifact_path, last_touched, prunable, prunable_reason (JSON-serialisable)
- `Status(ctx, deps.Deps, StatusInput) error` — snapshot status for all worktrees
  - Calls `refreshStatus()` → `repo.Load()` + `collectStatusRows()`
  - JSON flag: encodes `[]StatusRow` as JSON; otherwise renders via `writeStatusTable()`
- `collectStatusRows(ctx, deps.Deps, repo.Context, config.ArtifactConfig)` — probes each worktree
  - Queries `worktree.Dirty()`, `worktree.AheadBehind()`, `worktree.LastCommit()`; resolves artifact mtime
  - Skips git queries for prunable worktrees

### `tui.go` / `tui_update.go` / `tui_view.go`

- `UI(ctx, deps.Deps, StatusInput) error` — BubbleTea TUI fleet monitor
  - Returns `ErrNotTTY` (exit code 2) if stdout is not a terminal
  - Enters alt-screen; auto-refreshes every 2 seconds
  - `uiMode` constants: `uiModeNormal`, `uiModeConfirmRemove`, `uiModeConfirmForceRemove`, `uiModeConfirmPrune`, `uiModeConfirmShell`, `uiModeHelp`, `uiModeFilter`
  - Key events: `s`/`S` sync, `r`/`R` remove (confirm → `y`), `p` prune (confirm → `y`), `o` open, `d` diff, `e` shell (confirm → `y` → `doShell()`: spawns `$SHELL` or `/bin/sh` in worktree dir via `tea.ExecProcess`; TUI suspends, resumes on shell exit), `y` yank (OSC-52), `/` enter filter mode (fuzzy match on branch or path basename), `Esc` clear filter, `?` help overlay, `Enter` cd+quit
  - Filter mode (`uiModeFilter`) intercepts all keystrokes; Enter commits filter, Esc cancels
  - `selectedPath` set on Enter → after `p.Run()`, calls `uiEmitCD()` → `cd.EmitCD()` sentinel

### `filter.go`

- `filterRows(rows []StatusRow, query string) []StatusRow` — fuzzy-match rows by branch name and path basename using `sahilm/fuzzy`; returns matched rows ordered by best score; empty query returns rows unchanged

### `source.go`

- `ResolveSourceDir(useCurrentDir, sourcePath, cwd, worktrees)` — thin wrapper over `repo.ResolveSourceDir()`

### `config_ops.go`

- `ConfigInit(ctx, deps.Deps, ConfigInitInput)` — writes `.treepad.toml` (local or global) via `config.WriteDefault()`
- `ConfigShow(ctx, deps.Deps, ConfigShowInput)` — prints resolved config and sources via `config.Show()`

## Treepad Sub-Packages

### `internal/treepad/deps/`

- `Deps` struct — all injectable dependencies shared by every treepad operation
  - `Runner worktree.CommandRunner`, `Syncer sync.Syncer`, `Opener artifact.Opener`, `HookRunner hook.Runner`, `PTRunner passthrough.Runner`
  - `Profiler profile.Profiler` — records per-stage wall-time durations; `DefaultDeps` sets `profile.Disabled()` (no-op); `commandDeps()` replaces it with the `profile.Recorder` when `--profile` is passed
  - `Out io.Writer` — stdout: machine payloads (`__TREEPAD_CD__`, JSON, tables)
  - `Log *ui.Printer` — stderr: tagged user-facing narrative
  - `In io.Reader`
  - `IsTerminal func(w io.Writer) bool` — injectable TTY check
  - `CDSentinel func() io.Writer` — test override for the fd-3 cd sentinel writer
- `DefaultDeps(out, errw io.Writer, in io.Reader) Deps` — wires production implementations; `Profiler` defaults to `profile.Disabled()`

### `internal/treepad/lifecycle/`

Owns the worktree creation, removal, and pruning verbs.

- `CreateResult` struct — `RC repo.Context`, `Cfg config.Config`, `WorktreePath`, `ArtifactPath`
- `SyncTarget` struct — `Path`, `Branch`
- `CreateWorktreeWithSync(ctx, deps.Deps, branch, base, outputDir) (CreateResult, error)` — runs `git worktree add`, syncs configs, writes artifact; fires `pre_new`/`post_new` hooks
- `LoadAndSync(ctx, deps.Deps, sourceDir, extraPatterns, []SyncTarget, repoSlug, outputDir) (config.Config, error)` — loads config, syncs files to all targets; fires `pre_sync`/`post_sync` per target
- `OpenWorktree(ctx, deps.Deps, openCmd, branch, wtPath, artifactPath, outputDir) error` — opens artifact (or worktree dir) via configured open command
- `RemoveWorktreeAndArtifact(ctx, deps.Deps, target, main worktree.Worktree, outputDir string, force bool) error` — `git worktree remove [--force]`, deletes artifact, `git branch -d` (or `-D`); fires `pre_remove`/`post_remove` hooks
- `New(ctx, deps.Deps, NewInput) (mainPath string, error)` — calls `CreateWorktreeWithSync`, optionally opens artifact, emits cd sentinel unless `Current=true`
- `Remove(ctx, deps.Deps, RemoveInput) error` — guards: not-main, not-cwd-inside; delegates to `RemoveWorktreeAndArtifact`
- `Prune(ctx, deps.Deps, PruneInput) error` — `All=true`: force-removes all non-main (requires cwd in main); `All=false`: removes merged worktrees (skips dirty, ahead, current); `DryRun=true`: preview only; `Yes=true`: skips confirmation prompt

### `internal/treepad/cd/`

Thin wrappers that adapt `deps.Deps` to `cdshell.Deps`.

- `CD(ctx, deps.Deps, CDInput) error` — delegates to `cdshell.CD()`
- `Base(ctx, deps.Deps, BaseInput) error` — delegates to `cdshell.Base()`
- `EmitCD(deps.Deps, path)` — re-exported from cdshell for callers that hold `deps.Deps`
- `MaybeWarnStaleWrapper(deps.Deps, hasAgentCommand bool)` — re-exported from cdshell

### `internal/treepad/cdshell/`

Owns the `__TREEPAD_CD__` shell-bridge protocol.

- `EmitCD(Deps, path)` — writes the cd path to fd 3 (when `TREEPAD_CD_FD` is set by the shell wrapper) or falls back to writing `__TREEPAD_CD__\t<path>` to `d.Out`
- `CD(ctx, Deps, CDInput) error` — looks up worktree by branch, calls `EmitCD`
- `Base(ctx, Deps, BaseInput) error` — looks up main worktree, calls `EmitCD`; errors if already on main
- `MaybeWarnStaleWrapper(Deps, hasAgentCommand bool)` — prints a hint when `agent_command` is configured but the new fd-3 shell wrapper is not installed

### `internal/treepad/fromspec/`

- `FromSpecInput` struct — `Issue`, `Branch`, `Base`, `Current`, `OutputDir`, `Prompt`
- `FromSpec(ctx, deps.Deps, FromSpecInput) (exitCode int, error)` — fetches GitHub issue body, calls `CreateWorktreeWithSync`, writes `PROMPT.md`, runs `agent_command` via `PTRunner`; emits cd sentinel unless `Current=true`
  - Re-uses existing `PROMPT.md` if already present in the worktree
- `FromSpecBulkInput` struct — `Issues []int`, `BranchPrefix`, `Base`, `OutputDir`, `Prompt`
- `BulkResult` struct — per-issue outcome record
- `FromSpecBulk(ctx, deps.Deps, FromSpecBulkInput) ([]BulkResult, failedCount int, error)` — creates one worktree per issue; never launches an agent, never emits cd sentinel; partial failures are non-fatal; prints summary on completion

### `internal/treepad/repo/`

Resolves the repository context shared by every treepad operation.

- `Context` struct — `Worktrees`, `Main worktree.Worktree`, `Slug`, `OutputDir`
- `Load(ctx, runner, explicitOutputDir) (Context, error)` — lists worktrees, finds main, derives slug and output dir
- `ListWorktrees(ctx, runner) ([]worktree.Worktree, error)` — wraps `worktree.List()` with an empty-list error
- `ResolveOutputDir(explicit, repoSlug) (string, error)` — returns explicit if non-empty; otherwise `~/<repoSlug>-workspaces/`
- `ResolveSourceDir(useCurrentFlag, sourcePath, cwd, worktrees) (string, error)` — pure function; picks source from flag, explicit path, or main worktree
- `CwdInside(cwd, wtPath) bool` — reports whether cwd is inside wtPath (inclusive)
- `RequireCwdInside(cwd, wtPath, msg) error` — returns error with msg when cwd is not inside wtPath

### `internal/treepad/cwd/`

- `Resolve(override string) (string, error)` — returns override if non-empty, else `os.Getwd()`

### `internal/treepad/treepadtest/`

Test helpers: `fakes.go` (fake Runner/Syncer/Opener/HookRunner implementations), `fixtures.go` (standard worktree fixture sets), `runner.go` (command runner that records calls).

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

## Hook Package (`internal/hook/`)

Lifecycle hooks defined in `.treepad.toml` and run at specific points in `tp` operations.

### `hook.go`

- `Event` type — string constant: `PreNew`, `PostNew`, `PreRemove`, `PostRemove`, `PreSync`, `PostSync`
- `HookEntry` struct — `Command string`, `Only []string`, `Except []string` (glob branch filters)
- `Config` struct — holds `[]HookEntry` for each event; `IsZero()`, `For(Event) []HookEntry`
- `Data` struct — template context: `Branch`, `WorktreePath`, `Slug`, `HookType`, `OutputDir`
- `Runner` interface — `Run(ctx, []HookEntry, Data) error`
- `PostErr` struct — non-fatal post-hook error; callers log as warning
- `Run(ctx, Runner, Config, Event, Data) error` — executes hooks for a single event
- `RunSandwich(ctx, profile.Profiler, Runner, Config, pre, post Event, Data, do func() error) (*PostErr, error)` — runs pre → do → post; times each hook phase as `"<event>_hooks"` stages on the profiler; pre failure aborts; post failure returns `*PostErr` with nil main error

### `runner.go`

- `ExecRunner` struct — renders each hook command as a Go `text/template` and executes via `sh -c`
  - Skips entries whose `Only`/`Except` filters do not match `data.Branch`
  - Not supported on Windows (`GOOS=windows` returns an error)

### `filter.go`

- `shouldRun(entry HookEntry, branch string) bool` — evaluates `Only`/`Except` glob filters against the branch name

## Passthrough Package (`internal/passthrough/`)

Stdio-passthrough runner for child processes where `tp` must inherit the terminal.

### `passthrough.go`

- `Runner` interface — `Run(ctx, workdir, name string, args ...string) (exitCode int, error)`
- `OSRunner` struct — production implementation; sets `Stdin`/`Stdout`/`Stderr` to `os.Stdin`/`os.Stdout`/`os.Stderr` and `Dir` to workdir; returns child process exit code (non-zero is not an error)

## Profile Package (`internal/profile/`)

Lightweight wall-time stage profiler. Activated by the `--profile` global flag; no-op otherwise.

### `profile.go`

- `Profiler` interface — `Stage(name string) func()`, `Summary(w io.Writer, totalLabel string)`
- `Recorder` struct — production implementation; accumulates per-stage durations with a mutex; repeated `Stage()` calls with the same name add to the existing total
  - `Stage(name) func()` — records wall-time elapsed; call as `defer p.Stage("foo")()`
  - `Summary(w, totalLabel)` — prints a table of stages sorted by duration descending; longest stage marked with `◀`; format: `stage | duration | pct`
- `NewRecorder() *Recorder` — creates a production recorder using `time.Now`
- `Disabled() Profiler` — returns a shared no-op profiler (no allocations)
- `OrDisabled(p Profiler) Profiler` — returns `p` if non-nil, else `Disabled()`; use at package boundaries where `Deps.Profiler` may be unset

**Stage names used in production:**

| Stage | Where timed |
|---|---|
| `repo.load` | `lifecycle.CreateWorktreeWithSync` |
| `config.load` | `lifecycle.CreateWorktreeWithSync`, `lifecycle.LoadAndSync` |
| `git.worktree_add` | `lifecycle.CreateWorktreeWithSync` |
| `artifact.write` | `lifecycle.CreateWorktreeWithSync` |
| `git.worktree_prune` | `lifecycle.pruneGitWorktreeMetadata` |
| `pre_new_hooks` / `post_new_hooks` | `hook.RunSandwich` via lifecycle |
| `pre_sync_hooks` / `post_sync_hooks` | `hook.RunSandwich` via lifecycle |
| `pre_remove_hooks` / `post_remove_hooks` | `hook.RunSandwich` via lifecycle |

## TTY Package (`internal/tty/`)

- `IsTerminal(fd uintptr) bool` — platform-specific TTY detection (unix via `golang.org/x/term`; Windows stub in `tty_windows.go`)

## CLI Command Structure

```
tp [--verbose] <command>
├── sync [options] [source-path]
│   ├── --current (-c, --use-current alias)
│   ├── --sync-only
│   ├── --output-dir (-o)
│   └── --include (repeatable)
├── new [options] <branch>
│   ├── --base (-b, default: main)
│   ├── --open (-o)
│   └── --current (-c)
├── from-spec [options] <branch>
│   ├── --issue (-i, required)
│   ├── --base (-b, default: main)
│   ├── --current (-c)
│   └── --prompt (-p)
├── from-spec-bulk [options]
│   ├── --issues (-i, required, comma-separated)
│   ├── --branch-prefix
│   ├── --base (-b, default: main)
│   └── --prompt (-p)
├── shell-init
├── remove <branch>
├── prune [options]
│   ├── --base (-b, default: main)
│   ├── --dry-run (-n)
│   ├── --all (-a, force-remove all non-main, must be from main)
│   └── --yes (-y, skip confirmation)
├── cd <branch | ->
├── base
├── status [options]
│   └── --json (-j)
├── ui
├── exec <branch> [command] [args...]
│   └── Auto-detects task runner (just, npm, pnpm, yarn, bun, make, poetry, uv)
│       Routes through runner if command matches enumerated script
│       Override with [exec] runner in .treepad.toml
├── diff [options] <branch> [-- <git-diff-args>...]
│   ├── --base (-b, default: from config or origin/main)
│   └── --output (-o, optional)
├── doctor [options]
│   ├── --json (-j)
│   ├── --stale-days (default: 30)
│   ├── --base (-b, default: main)
│   ├── --offline
│   └── --strict
└── config
    ├── init [--global (-g)]
    └── show
```

## Key Design Decisions

1. **CLI Separation** — All CLI wiring (`internal/commands/`) is separate from business logic. Packages like `treepad`, `config`, `worktree`, `artifact`, and `sync` contain pure logic without CLI dependencies.

2. **Dependency Injection** — `deps.Deps` (in `internal/treepad/deps/`) bundles all injectable dependencies. Tests construct `Deps` directly with fakes; production callers use `DefaultDeps()`.

3. **Sub-package decomposition** — The `internal/treepad/` package delegates to focused sub-packages: `lifecycle/` (create/remove/prune), `cd/` + `cdshell/` (shell bridge), `fromspec/` (spec-driven creation), `repo/` (repository context), `cwd/` (working directory). Top-level functions in `treepad/` are thin orchestrators.

4. **Shell Bridge Protocol** — `tp cd`/`tp new`/`tp base`/`tp ui` cannot change the parent shell's directory directly. Instead they write a path to fd 3 (when `TREEPAD_CD_FD=3` is set by the shell wrapper) or fall back to `__TREEPAD_CD__\t<path>` on stdout. The shell function in `shell-init` captures fd 3 separately and calls `cd`.

5. **Global Config** — Follows XDG Base Directory spec with fallback to `TREEPAD_CONFIG` env var.

6. **Config Defaults** — Zero-config experience; sensible defaults (VS Code `.code-workspace` files) are built-in and used when `.treepad.toml` is absent.

7. **Config Resolution** — Three-tier lookup in `Show()`:
   - Local `.treepad.toml` (highest priority)
   - Global config (medium priority)
   - Built-in defaults (fallback)

8. **Editor Agnosticism** — No editor names in Go code. Artifact filename, content, and open command are all text/template strings in `.treepad.toml`. VS Code is the default. Other editors configure via config only.

## Data Flow Example: `tp sync`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.syncCommand()` defines CLI interface
3. `runSync()` builds `deps.DefaultDeps()`, calls `treepad.Generate()`
4. `Generate()` resolves source directory, derives output dir via `repo.ResolveOutputDir()`
5. Builds `[]lifecycle.SyncTarget` from all worktrees except source; filters to single branch if `--branch` set
6. Calls `lifecycle.LoadAndSync()` → loads config, syncs each target via `sync.FileSyncer`, fires `pre_sync`/`post_sync` hooks
7. Unless `--sync-only`, calls `artifact.Write()` per worktree

## Data Flow Example: `tp new`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.newCommand()` defines CLI interface
3. `runNew()` builds `deps.DefaultDeps()`, calls `lifecycle.New()`
4. `lifecycle.New()` calls `CreateWorktreeWithSync()`:
   - Fires `pre_new` hook, runs `git worktree add -b <branch> <path> <base>`
   - Calls `lifecycle.LoadAndSync()` for the new worktree (fires `pre_sync`/`post_sync`)
   - Calls `artifact.Write()`, fires `post_new` hook
5. If `--open`: calls `lifecycle.OpenWorktree()` via configured `open.command`
6. Unless `--current` / `-c`: calls `cd.EmitCD()` — writes path to fd 3 (shell wrapper captures it) or falls back to `__TREEPAD_CD__\t<path>` on stdout

## Data Flow Example: `tp remove <branch>`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.removeCommand()` defines CLI interface
3. `runRemove()` builds `deps.DefaultDeps()`, calls `lifecycle.Remove()`
4. `lifecycle.Remove()` guards: not-main, not-cwd-inside
5. Calls `lifecycle.RemoveWorktreeAndArtifact()`:
   - Fires `pre_remove` hook, runs `git worktree remove`
   - Deletes artifact file (missing file is not an error)
   - Runs `git branch -d`, fires `post_remove` hook

## Data Flow Example: `tp prune [--base main] [--dry-run] [--all]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.pruneCommand()` defines CLI interface
3. `runPrune()` builds `deps.DefaultDeps()`, calls `lifecycle.Prune()`
4. `lifecycle.Prune()` calls `repo.Load()` to get worktree list, then dispatches:
   - **`--all`:** `gatherAll()` collects all non-main worktrees; validates cwd in main worktree
   - **default:** `gatherMerged()` queries `worktree.MergedBranches()`, skips dirty/ahead/current-cwd worktrees
5. `executePrune()`:
   - `--dry-run`: prints candidates and returns
   - Otherwise: prompts "continue? [y/N]:" (unless `--yes`); calls `RemoveWorktreeAndArtifact()` per candidate (force=true for `--all`)
   - Runs `git worktree prune` at the end to clean stale metadata
   - Returns error if any removals failed

## Data Flow Example: `tp cd <branch>`

1. `commands.cdCommand()` calls `cd.CD()`
2. `cd.CD()` adapts deps → `cdshell.CD()`
3. `cdshell.CD()` lists worktrees, locates by branch, calls `EmitCD()`
4. `EmitCD()` checks `TREEPAD_CD_FD` env var; writes path to fd 3 when set (captured by shell wrapper); falls back to `__TREEPAD_CD__\t<path>` on stdout

## Data Flow Example: `tp status [--json]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.statusCommand()` builds `deps.DefaultDeps()`, calls `treepad.Status()`
3. `Status()` → `refreshStatus()` → `repo.Load()` + `config.Load()` + `collectStatusRows()`
4. `collectStatusRows()` probes each worktree: `worktree.Dirty()`, `worktree.AheadBehind()`, `worktree.LastCommit()`; resolves artifact mtime
5. JSON flag: encodes `[]StatusRow` to stdout; otherwise renders via `writeStatusTable()` using `text/tabwriter`

## Data Flow Example: `tp ui`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.uiCommand()` builds `deps.DefaultDeps()`, calls `treepad.UI()`
3. `UI()` checks TTY via `d.IsTerminal(d.Out)`; returns `ErrNotTTY` (exit code 2) if not a terminal
4. Constructs `uiModel`, enters alt-screen via `tea.NewProgram(..., tea.WithAltScreen(), ...)`
5. BubbleTea event loop:
   - `Init()` dispatches `doRefresh()` and `doTick()` (2-second tick)
   - `doRefresh()` calls `refreshStatus()` asynchronously; result arrives as `uiRefreshMsg`
   - Tick fires every 2s; skipped if an action is in flight
   - `/` key enters `uiModeFilter`; keystrokes update `filterStr`; Enter commits, Esc cancels; committed filter applied via `filterRows()` (fuzzy match)
   - `e` key enters `uiModeConfirmShell`; `y` confirms → `doShell()`: `tea.ExecProcess($SHELL)` in worktree dir; TUI suspends, resumes on shell exit
   - Key events dispatch sync, remove, prune, open, diff, shell as async `tea.Cmd`s
   - `y` key stores path in `yankPath`; `View()` emits OSC-52 escape; cleared next tick via `uiYankClearMsg`
   - Enter sets `selectedPath` and returns `tea.Quit`
6. After `p.Run()` returns, if `selectedPath` is non-empty: calls `uiEmitCD()` → `cd.EmitCD()` → shell wrapper cd's

## Data Flow Example: `tp diff <branch> [--base ref] [-o file] [-- <git-diff-args>...]`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.diffCommand()` builds `deps.DefaultDeps()`, calls `treepad.Diff()`
3. `Diff()`:
   - Lists worktrees via `repo.ListWorktrees()`, finds target by branch
   - Returns error if branch not found or worktree is prunable
   - Resolves base via `resolveBase()` → `config.Load(mainPath).Diff.Base` or `"origin/main"`
   - **Output file set:** `git -C <targetPath> diff --no-color <base>...HEAD [extra-args]` captured, written to file
   - **No output file:** `git diff <base>...HEAD [extra-args]` via `passthrough.OSRunner` with inherited stdio

## Data Flow Example: `tp doctor`

1. `cmd/tp/main.go` parses flags and calls `commands.Router()`
2. `commands.doctorCommand()` builds `deps.DefaultDeps()`, calls `treepad.Doctor()`
3. `Doctor()`:
   - Calls `repo.Load()` + `config.Load()` + `worktree.MergedBranches()`
   - Per worktree: runs five health checks (age, merged, remote-gone, artifact, config-drift)
   - Prunable worktrees get a `prunable` finding directly
   - JSON flag: encodes `[]DoctorFinding` to stdout; otherwise renders via `writeDoctorTable()`
   - `--strict`: returns error if `len(findings) > 0`

---

**Last Updated:** April 26, 2026

**Recent Changes:**
- `internal/treepad/` monolith refactored into focused sub-packages: `lifecycle/`, `cd/`, `cdshell/`, `fromspec/`, `deps/`, `repo/`, `cwd/`
- `Service` struct removed; replaced by standalone top-level functions (`Generate`, `Exec`, `Diff`, `Doctor`, `Status`, `UI`)
- `Deps` struct moved to `internal/treepad/deps/` package
- Added `tp doctor` command for cross-worktree health reporting (stale, merged-present, remote-gone, artifact-missing, config-drift)
- Added `tp base` command to return to the main worktree
- Shell bridge upgraded to fd-3 protocol (`TREEPAD_CD_FD`): binary writes cd path to fd 3 (captured by wrapper), stdout flows live to terminal
- TUI `/` key enters fuzzy filter mode (branch + path basename) via `filterRows()` using `sahilm/fuzzy`
- `tp prune` safety enhanced: skips dirty and ahead-of-upstream worktrees in merge-based mode
- Added `--profile` global flag: attaches `profile.Recorder` to `Deps.Profiler`; prints per-stage timing table to stderr after command finishes
- Added `internal/profile/` package: `Profiler` interface, `Recorder` (production), `Disabled()` (no-op)
- TUI `e` key: enters `uiModeConfirmShell`; confirmed → `tea.ExecProcess($SHELL)` in selected worktree; TUI suspends then resumes
- TUI auto-refresh interval is 2 seconds (not 5)
