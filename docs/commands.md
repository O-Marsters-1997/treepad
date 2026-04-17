# Commands

## sync

Syncs editor configs and generates artifact files across all git worktrees. By default, generates VS Code `.code-workspace` files.

```
tp sync [options] [source-path]
```

By default, uses the main worktree (the one with a `.git` directory) as the config source. Configs from `.vscode/`, `.claude/`, and `.env` files are copied to every other worktree. The artifact file generated is controlled by `.treepad.toml` and can be customized for any editor.

**Hooks fired:** `pre_sync`/`post_sync` around each worktree's file sync. See [hooks.md](hooks.md).

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
| `--include` | | Additional file patterns to sync (appended to `sync.include` in `.treepad.toml`) |

### Examples

```bash
# Generate artifact files and sync configs from the main worktree
tp sync

# Sync configs only (no artifact files generated)
tp sync --sync-only

# Use the current directory as the config source
tp sync --use-current

# Write artifact files to a custom directory
tp sync --output-dir ~/my-workspaces

# Use an explicit repo path as the config source
tp sync /path/to/repo

# Include extra file patterns in the sync
tp sync --include ".prettierrc" --include "*.md"
```

### Configuration

See [configuration.md](configuration.md) for the full schema, defaults, and examples.

## new

Create a new git worktree, sync configs from the main worktree, and generate an artifact file for it.

```
tp new [options] <branch>
```

Creates a new worktree branched from a specified ref (default: `main`), syncs editor configs from the main worktree, and generates an artifact file as configured in `.treepad.toml`. By default, cd's into the new worktree directory when invoked via the shell wrapper (see [Shell integration](#shell-integration) below).

**Hooks fired:** `pre_new` (before `git worktree add`), `pre_sync`/`post_sync` (around file sync), `post_new` (after artifact write). See [hooks.md](hooks.md).

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--base` | | Ref to branch the new worktree from (default: `main`) |
| `--open` | `-o` | Open the generated artifact file (using the command specified in `[open].command`) |
| `--current` | `-c` | Stay in the current directory instead of cd-ing into the new worktree |

### Examples

```bash
# Create a new worktree for branch 'feature-x' branched from main
tp new feature-x

# Create a worktree from a different base ref
tp new bugfix-y --base develop

# Create a worktree and open the generated artifact file
tp new feature-z --open

# Stay in the current directory instead of cd-ing in
tp new my-branch -c
```

## from-spec

Create a worktree from a spec (GitHub issue or file), render a prompt, and hand off to an agent.

```
tp from-spec [options] <branch>
```

Creates a new worktree, loads a spec from either a GitHub issue or a local markdown file, generates a prompt for an agent to work on the spec, and hands off execution. The spec is parsed into a structured format that agents can use to understand the requirements. By default, cd's into the new worktree when invoked via the shell wrapper.

**Spec source resolution:**
- `--issue`: GitHub issue number (requires internet access and GitHub permissions)
- `--file`: Local markdown file path

One of `--issue` or `--file` is required; they are mutually exclusive.

**Hooks fired:** Same as `new` command: `pre_new` (before `git worktree add`), `pre_sync`/`post_sync` (around file sync), `post_new` (after artifact write). See [hooks.md](hooks.md).

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--issue` | `-i` | GitHub issue `number` to use as the spec (mutually exclusive with `--file`) |
| `--file` | `-f` | Path to a local markdown spec `file` (mutually exclusive with `--issue`) |
| `--base` | | Ref to branch the new worktree from (default: `main`) |
| `--current` | `-c` | Stay in the current directory instead of cd-ing into the new worktree |

### Examples

```bash
# Create a worktree from a GitHub issue spec
tp from-spec feature-x --issue 42

# Create a worktree from a local markdown spec file
tp from-spec feature-y --file ./spec.md

# Create a worktree from a different base ref
tp from-spec bugfix-z --issue 10 --base develop

# Stay in current directory after creation
tp from-spec feature-a --issue 99 --current
```

### Spec Format

Specs are expected to be in markdown format. When provided via `--issue`, the GitHub issue body is used directly. When using `--file`, the markdown file should contain the spec content.

The spec is parsed and made available to agents as structured input, enabling them to understand and implement the requirements.

## cd

cd into an existing worktree by branch name.

```
tp cd <branch>
```

Looks up the worktree registered under `<branch>` from `git worktree list` and emits a `__TREEPAD_CD__` directive. The shell wrapper installed by `shell-init` intercepts it and changes the current directory. No flags — positional branch argument only.

If the branch has no associated worktree, an error is returned with a suggestion to use `tp new <branch>`.

### Setup

Requires the shell wrapper (same as `new`):

```sh
eval "$(tp shell-init)"
```

### Examples

```bash
# cd into an existing worktree
tp cd feature-x

# Run the binary directly to inspect the directive
command tp cd feature-x
# => __TREEPAD_CD__	/path/to/repo-feature-x
```

## shell-init

Print a shell wrapper function that enables `tp new` to cd into the new worktree automatically.

```
tp shell-init
```

Because a child process cannot change the parent shell's working directory, `tp new` emits a `__TREEPAD_CD__` directive in its output. The shell wrapper function intercepts this directive, strips it from the visible output, and cd's into the path.

### Setup

Add to your `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(tp shell-init)"
```

After sourcing, `tp new <branch>` will automatically cd into the new worktree. Pass `-c` / `--current` to skip the cd.

## config

Manage tp configuration files.

```
tp config <subcommand>
```

### config init

Write a config file with default values.

```
tp config init [--global]
```

By default, writes `.treepad.toml` to the main worktree root (the directory containing `.git`). Use the `--global` flag to write to the global config path instead.

#### Flags

| Flag | Description |
|------|-------------|
| `--global` | Write to the global config path instead of `.treepad.toml` in the main worktree |

#### Examples

```bash
# Write default config to the main worktree root
tp config init

# Write default config to the global config path
tp config init --global
```

### config show

Print the resolved config and which sources contributed.

```
tp config show
```

Displays the final configuration that would be used, along with information about which source(s) contributed to it. Resolution order is:
1. Local `.treepad.toml` in the main worktree
2. Global config file (from `$TREEPAD_CONFIG`, `$XDG_CONFIG_HOME/treepad/config.toml`, or `~/.config/treepad/config.toml`)
3. Built-in defaults

#### Examples

```bash
# Show the resolved config and its sources
tp config show
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
tp remove <branch>
```

Removes the worktree for the specified branch, cleans up its associated artifact file (if any), and deletes the branch locally. Includes pre-flight safety guards to prevent accidental data loss.

**Hooks fired:** `pre_remove` (before `git worktree remove`), `post_remove` (after `git branch -d`). See [hooks.md](hooks.md).

### Pre-flight guards

- Refuses to remove the main worktree
- Refuses to remove a worktree if you are currently inside it (must `cd` elsewhere first)

### Examples

```bash
# Remove a completed feature branch
tp remove feature-x

# Remove after switching out of the worktree
cd ../main-repo  # or any other location
tp remove feature-x
```

### Errors

Attempting to remove the main worktree or the worktree you're currently in will return an error:

```
cannot remove the main worktree
cannot remove the worktree you are currently in; cd elsewhere first
```

## prune

Remove all worktrees whose branches are already merged into a base branch, or force-remove all non-main worktrees. Useful for batch-cleaning completed work.

```
tp prune [options]
```

Automatically identifies and removes worktrees whose branches have been merged into a base branch (default: `main`). Executes removals directly; pass `--dry-run` to preview without making changes. Use `--all` to force-remove all non-main worktrees (with confirmation prompt).

### Flags

| Flag | Description |
|------|-------------|
| `--base` | Ref to check merges against (default: `main`) |
| `--dry-run` | Preview removals without executing |
| `--all` | Force-remove all non-main worktrees regardless of merge status (must be run from main worktree, requires confirmation) |

### Filtering

When not using `--all`:
- The main worktree is automatically skipped
- Detached HEAD worktrees are skipped
- The worktree you are currently in is skipped (continues to next rather than failing)

When using `--all`:
- Only the main worktree is preserved
- Detached HEAD worktrees are still removed
- Must be invoked from the main worktree (guards against removal by accident)
- Requires interactive confirmation before proceeding

### Examples

```bash
# Remove all worktrees merged into main
tp prune

# Preview without executing
tp prune --dry-run

# Check merges against a different base branch
tp prune --base develop

# Preview against a different base
tp prune --base develop --dry-run

# Force-remove all non-main worktrees (with confirmation)
tp prune --all

# Preview force-removal without executing
tp prune --all --dry-run
```

### Output Examples

**Execution output (default, merge-based):**

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

**Force-remove all (`--all`) execution:**

```
the following worktrees will be force-removed:
  feature-x  /path/to/repo/repo-feature-x
  feature-y  /path/to/repo/repo-feature-y
continue? [y/N]: y
removed worktree: /path/to/repo/repo-feature-x
removed artifact: /home/user/repo-workspaces/repo-feature-x.code-workspace
deleted branch: feature-x
removed worktree: /path/to/repo/repo-feature-y
removed artifact: /home/user/repo-workspaces/repo-feature-y.code-workspace
deleted branch: feature-y
```

**Force-remove all (`--all`) aborted:**

```
the following worktrees will be force-removed:
  feature-x  /path/to/repo/repo-feature-x
  feature-y  /path/to/repo/repo-feature-y
continue? [y/N]: n
aborted
```

### Skipping current worktree

If you're currently inside a merged worktree (merge-based mode), prune skips it and continues with the rest:

```
skipping feature-x: currently in this worktree
removed worktree: /path/to/repo/repo-feature-y
removed artifact: /home/user/repo-workspaces/repo-feature-y.code-workspace
deleted branch: feature-y
```

## status

List all worktrees in the repo with their branch, dirty state, ahead/behind count vs upstream, last commit, and last-touched time (from artifact file mtime).

```
tp status [options]
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
tp status

# Emit JSON for scripting or dashboards
tp status --json

# Combine with standard tools
tp status | grep dirty
tp status --json | jq '.[] | select(.dirty == true)'
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

## diff

Show the diff of a worktree against a base branch using three-dot merge-base semantics.

```
tp diff [options] <branch> [-- <git-diff-args>...]
```

Displays the diff between the target worktree's branch and a base ref (default: `main`) using `git diff <base>...HEAD` semantics, which matches the diff view in GitHub pull requests. The diff is shown in the terminal with color and paging inherited from the target worktree's git configuration (respects `delta`, `diff-so-fancy`, or other configured tools). Optionally writes a plain (uncolored) patch to a file with `--output`.

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--base` | `-b` | Ref to diff against (default: `main`) |
| `--output` | `-o` | Write uncolored patch to `file` instead of terminal; outputs `[OK]` to stderr on success |

### Argument Forwarding

Everything after `--` is forwarded directly to `git diff`:

```bash
# Show only changed files (using git diff --stat)
tp diff feature-x -- --stat

# Limit diff to a specific subdirectory
tp diff feature-x -- -- src/

# Show word-level diffs
tp diff feature-x -- --word-diff
```

### Semantics

- **Three-dot merge-base** — Uses `<base>...HEAD` which includes commits on the target branch since it diverged from base, matching GitHub PR diff behavior
- **Ref-based** — Diffs the committed tip; uncommitted changes in the worktree are ignored
- **Inherited git config** — Color, pager, and diff algorithm are sourced from the target worktree's git configuration

### Examples

```bash
# Show diff of feature-x against main (colored, paged)
tp diff feature-x

# Diff against a different base branch
tp diff feature-x --base develop

# Write a plain patch to a file (useful for email, review, archival)
tp diff feature-x -o ~/my-feature.patch

# Show file change summary
tp diff feature-x -- --stat

# Show only files matching a pattern
tp diff feature-x -- -- src/components/

# Advanced: show word-level diffs for detailed review
tp diff feature-x -- --word-diff
```

### Error Cases

**Worktree not found:**
```
no worktree found for branch 'unknown'; run `tp sync` to list worktrees
```

**Prunable target:**
```
worktree for 'feature-x' is prunable (branch is merged into main); run `tp prune`
```

### Git Config Inheritance

The `diff` command executes `git diff` inside the target worktree. This means it inherits all git configuration from that worktree, including:

- Pager settings (`core.pager`)
- Custom diff tools (`diff.tool`, `difftool.cmd`)
- Color settings (`color.diff`)
- Diff algorithms (`diff.algorithm`)

If the target worktree has `delta` or `diff-so-fancy` configured, `tp diff` will use it automatically.
