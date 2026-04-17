# Hooks

Hooks let you run shell commands at specific points in `tp`'s lifecycle — before or after creating worktrees, removing them, and syncing files. They are defined in `.treepad.toml` and run in the same shell environment as the `tp` process.

## How hooks work

- **Pre-hooks** (`pre_*`) run before the operation. If a pre-hook command exits non-zero, the operation is aborted and the error is returned to the caller.
- **Post-hooks** (`post_*`) run after the operation completes. A failure is logged as a warning but never aborts the command.
- Hooks run sequentially. If a command in a list fails, subsequent commands in that list are not run.
- An empty or absent hook list is a no-op; there is no performance cost.

## Events

| Event | When it fires | Blocks on failure |
|---|---|---|
| `pre_new` | Before `git worktree add` | Yes |
| `post_new` | After artifact file is written | No (warning logged) |
| `pre_remove` | Before `git worktree remove` | Yes |
| `post_remove` | After `git branch -d` | No (warning logged) |
| `pre_sync` | Before each worktree's file sync | Yes |
| `post_sync` | After each worktree's file sync | No (warning logged) |

Sync events fire per-worktree. For `tp sync` syncing three worktrees, `pre_sync`/`post_sync` fire three times.

## Template variables

Each hook command is a Go `text/template` string. The following variables are available:

| Variable | Description |
|---|---|
| `{{.Branch}}` | Raw branch name (e.g. `feature/auth`) |
| `{{.WorktreePath}}` | Absolute path of the worktree on disk |
| `{{.Slug}}` | Repository slug (sanitized repo dir name, e.g. `myrepo`) |
| `{{.HookType}}` | The event being fired (e.g. `post_new`) |
| `{{.OutputDir}}` | Artifact output directory |

Template rendering happens before the command is executed. A template error (e.g., referencing a non-existent field) is treated as a pre-hook failure.

## Configuration

Hooks are declared in `.treepad.toml` using TOML array-of-tables syntax. Each entry has a `command` field and optional `only`/`except` branch filters.

```toml
[[hooks.pre_new]]
command = "go mod download"

[[hooks.post_new]]
command = "echo 'created {{.Branch}} at {{.WorktreePath}}' >> .treepad/activity.log"

[[hooks.post_new]]
command = "direnv allow {{.WorktreePath}}"

[[hooks.pre_remove]]
command = "git -C {{.WorktreePath}} diff --exit-code HEAD"

[[hooks.post_sync]]
command = "direnv allow {{.WorktreePath}}"
```

Unset or absent hook lists are silently skipped.

## Branch filtering

Each hook entry supports `only` and `except` fields, which accept glob patterns (`*` matches within a path segment; `**` crosses path separators).

```toml
[[hooks.post_new]]
command = "go mod download"
only = ["feat/*", "fix/*"]   # only run for feat/ and fix/ branches

[[hooks.post_new]]
command = "echo 'hotfix started'"
only = ["hotfix/**"]         # run for hotfix/ and any deeper nesting

[[hooks.pre_remove]]
command = "git -C {{.WorktreePath}} diff --exit-code HEAD"
except = ["throwaway/*"]     # skip safety check on throwaway branches
```

Filter semantics:

- If `only` is set, the branch must match at least one pattern — otherwise the entry is skipped.
- If `except` is set, the branch must not match any pattern — otherwise the entry is skipped.
- Both conditions apply when both are set (AND semantics): the branch must satisfy `only` and fail `except`.
- An entry with neither field runs for all branches.

> **Note:** Hooks are not supported on Windows in v1. `tp` returns an error if a hook is configured and executed on `GOOS=windows`.

## Execution model

- Each list entry is rendered as a Go template, then passed to `sh -c <rendered>`.
- Commands run with the working directory inherited from the `tp` process (usually the directory you invoked `tp` from).
- Environment variables are inherited from the `tp` process.
- Standard output and error from hook commands are not currently captured or displayed; post-hook failures appear as `warning: post hook <event> failed: <error>` in `tp` output.

## Example use cases

### Bootstrap a new agent worktree

Run dependency install and generate a seed task file when a new worktree is created. The agent opens to a ready environment.

```toml
[[hooks.post_new]]
command = "cd {{.WorktreePath}} && go mod download"

[[hooks.post_new]]
command = "cp .env.example {{.WorktreePath}}/.env"

[[hooks.post_new]]
command = "echo '# Task: {{.Branch}}\n\nSee CLAUDE.md for context.' > {{.WorktreePath}}/TASK.md"
```

### Refuse removal of dirty worktrees

Block `tp remove` if the worktree has uncommitted changes, preventing accidental data loss.

```toml
[[hooks.pre_remove]]
command = "git -C {{.WorktreePath}} diff --exit-code HEAD"
```

If the diff exits non-zero (dirty), `tp remove` aborts with a hook error before touching anything.

### Auto-approve direnv per worktree

Allow the new worktree's `.envrc` after sync so environment variables are live before the agent opens.

```toml
[[hooks.post_sync]]
command = "direnv allow {{.WorktreePath}}"
```

Because `post_sync` fires per-worktree, every synced directory gets approved.

### Log agent activity to a shared file

Append to a repo-level activity log on every worktree creation.

```toml
[[hooks.post_new]]
command = "echo \"$(date -u +%FT%TZ) created {{.Branch}}\" >> .treepad/activity.log"

[[hooks.post_remove]]
command = "echo \"$(date -u +%FT%TZ) removed {{.Branch}}\" >> .treepad/activity.log"
```

### Run linters before sync overwrites

Guard against syncing a broken config into all worktrees.

```toml
[[hooks.pre_sync]]
command = "./scripts/validate-vscode-settings.sh"
```

### Run different setup per branch type

Install dependencies only on feature branches; run integration tests only on fix branches.

```toml
[[hooks.post_new]]
command = "cd {{.WorktreePath}} && go mod download"
only = ["feat/**"]

[[hooks.post_new]]
command = "cd {{.WorktreePath}} && go test ./..."
only = ["fix/**", "hotfix/**"]
```

## Limitations and roadmap

| Limitation | Notes |
|---|---|
| Repo-level only | Global `~/.config/treepad/config.toml` hooks are not merged in v1 |
| No approval flow | All configured hooks execute unconditionally (no first-run approval gate) |
| No concurrent hooks | Commands within a list run sequentially; parallel execution is not supported |
| Windows not supported | `sh -c` execution path; `cmd /C` fallback is deferred |
| No per-branch `vars` | Branch-specific variable scratchpad is part of the shared-state feature (`.treepad/state.toml`), not hooks |
