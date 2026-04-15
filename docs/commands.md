# Commands

## workspace

Syncs editor configs and generates artifact files across all git worktrees. By default, generates VS Code `.code-workspace` files.

```
treepad workspace [options] [source-path]
```

By default, uses the main worktree (the one with a `.git` directory) as the config source. Configs from `.vscode/`, `.claude/`, and `.env` files are copied to every other worktree. The artifact file generated is controlled by `.treepad.toml` and can be customized for any editor.

**Source resolution precedence:**
1. Explicit `source-path` argument
2. `--use-current` flag (current directory)
3. Auto-detected main worktree

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--use-current` | `-c` | Use current directory as config source instead of the main worktree |
| `--sync-only` | | Sync configs only; skip artifact file generation |
| `--output-dir` | `-o` | Directory for generated artifact files (default: `~/<repo-slug>-workspaces/`) |
| `--include` | | Additional file patterns to sync (appended to `sync.files` in `.treepad.toml`) |

### Examples

```bash
# Generate artifact files and sync configs from the main worktree
treepad workspace

# Sync configs only (no artifact files generated)
treepad workspace --sync-only

# Use the current directory as the config source
treepad workspace --use-current

# Write artifact files to a custom directory
treepad workspace --output-dir ~/my-workspaces

# Use an explicit repo path as the config source
treepad workspace /path/to/repo

# Include extra file patterns in the sync
treepad workspace --include ".prettierrc" --include "*.md"
```

### Configuration

See [configuration.md](configuration.md) for the full schema, defaults, and examples.

## new

Create a new git worktree, sync configs from the main worktree, and generate an artifact file for it.

```
treepad new [options] <branch>
```

Creates a new worktree branched from a specified ref (default: `main`), syncs editor configs from the main worktree, and generates an artifact file as configured in `.treepad.toml`. By default, cd's into the new worktree directory when invoked via the shell wrapper (see [Shell integration](#shell-integration) below).

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--base` | | Ref to branch the new worktree from (default: `main`) |
| `--open` | `-o` | Open the generated artifact file (using the command specified in `[open].command`) |
| `--current` | `-c` | Stay in the current directory instead of cd-ing into the new worktree |

### Examples

```bash
# Create a new worktree for branch 'feature-x' branched from main
treepad new feature-x

# Create a worktree from a different base ref
treepad new bugfix-y --base develop

# Create a worktree and open the generated artifact file
treepad new feature-z --open

# Stay in the current directory instead of cd-ing in
treepad new my-branch -c
```

## shell-init

Print a shell wrapper function that enables `treepad new` to cd into the new worktree automatically.

```
treepad shell-init
```

Because a child process cannot change the parent shell's working directory, `treepad new` emits a `__TREEPAD_CD__` directive in its output. The shell wrapper function intercepts this directive, strips it from the visible output, and cd's into the path.

### Setup

Add to your `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(treepad shell-init)"
```

After sourcing, `treepad new <branch>` will automatically cd into the new worktree. Pass `-c` / `--current` to skip the cd.

## config

Manage treepad configuration files.

```
treepad config <subcommand>
```

### config init

Write a config file with default values.

```
treepad config init [--global]
```

By default, writes `.treepad.toml` to the main worktree root (the directory containing `.git`). Use the `--global` flag to write to the global config path instead.

#### Flags

| Flag | Description |
|------|-------------|
| `--global` | Write to the global config path instead of `.treepad.toml` in the main worktree |

#### Examples

```bash
# Write default config to the main worktree root
treepad config init

# Write default config to the global config path
treepad config init --global
```

### config show

Print the resolved config and which sources contributed.

```
treepad config show
```

Displays the final configuration that would be used, along with information about which source(s) contributed to it. Resolution order is:
1. Local `.treepad.toml` in the main worktree
2. Global config file (from `$TREEPAD_CONFIG`, `$XDG_CONFIG_HOME/treepad/config.toml`, or `~/.config/treepad/config.toml`)
3. Built-in defaults

#### Examples

```bash
# Show the resolved config and its sources
treepad config show
```

This will output something like:

```
Sources:
  local:  /path/to/repo/.treepad.toml

Config:
[sync]
files = [".claude/settings.local.json", ".env"]

[artifact]
filename = "myrepo-{{.Branch}}.code-workspace"
content = "..."
```

See [configuration.md](configuration.md) for details on the configuration schema and defaults.

## remove

Remove a git worktree, delete its artifact file, and delete the local branch.

```
treepad remove <branch>
```

Removes the worktree for the specified branch, cleans up its associated artifact file (if any), and deletes the branch locally. Includes pre-flight safety guards to prevent accidental data loss.

### Pre-flight guards

- Refuses to remove the main worktree
- Refuses to remove a worktree if you are currently inside it (must `cd` elsewhere first)

### Examples

```bash
# Remove a completed feature branch
treepad remove feature-x

# Remove after switching out of the worktree
cd ../main-repo  # or any other location
treepad remove feature-x
```

### Errors

Attempting to remove the main worktree or the worktree you're currently in will return an error:

```
cannot remove the main worktree
cannot remove the worktree you are currently in; cd elsewhere first
```

## prune

Remove all worktrees whose branches are already merged into a base branch. Useful for batch-cleaning completed work.

```
treepad prune [options]
```

Automatically identifies and removes worktrees whose branches have been merged into a base branch (default: `main`). Executes removals directly; pass `--dry-run` to preview without making changes.

### Flags

| Flag | Description |
|------|-------------|
| `--base` | Ref to check merges against (default: `main`) |
| `--dry-run` | Preview removals without executing |

### Filtering

Prune automatically skips:
- The main worktree
- Detached HEAD worktrees
- The worktree you are currently in (continues to next rather than failing)

### Examples

```bash
# Remove all worktrees merged into main
treepad prune

# Preview without executing
treepad prune --dry-run

# Check merges against a different base branch
treepad prune --base develop

# Preview against a different base
treepad prune --base develop --dry-run
```

### Output Examples

**Execution output (default):**

```
removed worktree: /path/to/repo/repo-feature-x
removed artifact: /home/user/repo-workspaces/repo-feature-x.code-workspace
deleted branch: feature-x
removed worktree: /path/to/repo/repo-feature-y
removed artifact: /home/user/repo-workspaces/repo-feature-y.code-workspace
deleted branch: feature-y
```

**Dry-run output (`--dry-run`):**

```
would remove: feature-x (/path/to/repo/repo-feature-x)
would remove: feature-y (/path/to/repo/repo-feature-y)
```

**No merged worktrees:**

```
no merged worktrees to remove
```

### Skipping current worktree

If you're currently inside a merged worktree, prune skips it and continues with the rest:

```
skipping feature-x: currently in this worktree
removed worktree: /path/to/repo/repo-feature-y
removed artifact: /home/user/repo-workspaces/repo-feature-y.code-workspace
deleted branch: feature-y
```

## status

List all worktrees in the repo with their branch, dirty state, ahead/behind count vs upstream, last commit, and last-touched time (from artifact file mtime).

```
treepad status [options]
```

Provides a repo-wide snapshot of all active worktrees, showing which ones have uncommitted changes, how they diverge from their upstream branches, and when they were last modified by agents or editors.

### Flags

| Flag | Description |
|------|-------------|
| `--json` | Emit JSON array instead of an aligned table |

### Output Columns (Table Format)

| Column | Meaning |
|--------|---------|
| `BRANCH` | Branch name, with `*` suffix if main worktree |
| `STATUS` | `clean` or `dirty` (has uncommitted changes) |
| `AHEAD/BEHIND` | `↑N ↓M` vs upstream, or `—` if no upstream configured |
| `LAST COMMIT` | Short SHA, subject, and relative time (e.g. `abc1234 fix thing · 3m`) |
| `TOUCHED` | Relative time since artifact file was last modified, or `—` if no artifact |
| `PATH` | Absolute path (collapsed to `~/...` when under home directory) |

### Examples

```bash
# Show status of all worktrees in a table
treepad status

# Emit JSON for scripting or dashboards
treepad status --json

# Combine with standard tools
treepad status | grep dirty
treepad status --json | jq '.[] | select(.dirty == true)'
```

### Output Examples

**Table output:**

```
BRANCH                   STATUS  AHEAD/BEHIND  LAST COMMIT                            TOUCHED  PATH
main *                   dirty   ↑0 ↓0         ea69222 Merge PR #33 · 1h             1d       ~/treepad
feat/status              clean   —             ea69222 Merge PR #33 · 1h             18m      ~/treepad-feat-status
task/remove-guards       clean   ↑0 ↓6         8305b88 add pre-flight guards · 6h    —        ~/treepad-remove-guards
```

**JSON output (pretty-printed):**

```json
[
  {
    "branch": "main",
    "path": "/Users/user/treepad",
    "is_main": true,
    "dirty": true,
    "ahead": 0,
    "behind": 0,
    "has_upstream": true,
    "last_commit": {
      "sha": "ea69222",
      "subject": "Merge pull request #33",
      "committed": "2026-04-15T15:07:51+01:00"
    },
    "artifact_path": "/Users/user/treepad-workspaces/treepad-main.code-workspace",
    "last_touched": "2026-04-13T20:07:27.882Z"
  }
]
```
