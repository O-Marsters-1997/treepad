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
2. `--current` flag (current directory)
3. Auto-detected main worktree

### Flags

| Flag           | Short | Description                                                                      |
| -------------- | ----- | -------------------------------------------------------------------------------- |
| `--current`    | `-c`  | Use current directory as config source instead of the main worktree              |
| `--sync-only`  |       | Sync configs only; skip artifact file generation                                 |
| `--output-dir` | `-o`  | Directory for generated artifact files (default: `~/<repo-slug>-workspaces/`)    |
| `--include`    |       | Additional file patterns to sync (appended to `sync.include` in `.treepad.toml`) |

**Note:** `--use-current` is accepted as a backwards-compatible alias for `--current`.

### Examples

```bash
# Generate artifact files and sync configs from the main worktree
tp sync

# Sync configs only (no artifact files generated)
tp sync --sync-only

# Use the current directory as the config source
tp sync --current

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

| Flag        | Short | Description                                                                        |
| ----------- | ----- | ---------------------------------------------------------------------------------- |
| `--base`    | `-b`  | Ref to branch the new worktree from (default: `main`)                              |
| `--open`    | `-o`  | Open the generated artifact file (using the command specified in `[open].command`) |
| `--current` | `-c`  | Stay in the current directory instead of cd-ing into the new worktree              |
| `--ticket`  | `-t`  | Ticket URL, or a bare Ref when `[from_spec] ticket_url` is configured              |

### Ticket-driven creation

With `--ticket`, `tp new` resolves the Ticket to a Ticket URL and hands that URL to the configured `[from_spec] agent_command` once the worktree exists. Treepad writes no prompt: it passes the URL and the agent retrieves the Spec itself. See [ADR 0001](adr/0001-treepad-does-not-read-trackers.md) and [ADR 0002](adr/0002-treepad-writes-playbooks-not-prompts.md).

Resolution happens *before* the worktree is created, so an unresolvable Ticket leaves no worktree behind:

- Input starting with `http://` or `https://` is used verbatim as the Ticket URL
- Anything else is a Ref, rendered through `[from_spec] ticket_url` with `{{.Ref}}`
- A bare Ref with no `ticket_url` configured is an error naming both fixes

`--ticket` cannot be combined with `--open`.

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

# Create a worktree from a Ticket, using a bare Ref against the configured ticket_url
tp new feature-a --ticket ENG-42

# Pass a full Ticket URL — no ticket_url needed, and it overrides one if set
tp new feature-b --ticket https://github.com/acme/api/issues/42
```

## batch orchestration

Turn a scoped collection of Tickets into a fleet of provisioned worktrees that respects blocking
relationships: unblocked work runs in parallel, dependent work is stacked rather than serialised.
Vocabulary follows [CONTEXT.md](../CONTEXT.md): a **Batch** is a collection of Tickets declared by a
**Manifest**; a **Chain** is an ordered run of Tickets within a Batch, each worktree branched from
the one before it; a **Stack** is GitHub's linked pull requests, which a Chain *becomes* once linked;
the **Launcher** starts one agent in one worktree; the **Activity file** is the sole evidence treepad
has that an agent is working in it.

A Batch of single-Ticket Chains replaces the bulk worktree-creation verb this repo used to ship —
see [Retired commands](#retired-commands).

### The Manifest

A Manifest is an uncommitted local TOML file under `<git-common-dir>/treepad/batches/*.toml`,
declaring one Batch's Chains. Treepad reads every Manifest under that directory and never writes one
itself — a Manifest is authored by an agent that read the Tracker, or by hand for testing.

```toml
name          = "silent-refresh"
branch_prefix = "feat/"
base          = "main"

[[chain]]
tickets = ["ENG-12", "ENG-13"]

[[chain]]
tickets = ["ENG-14"]
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | The Batch's name. Defaults to the filename stem when omitted. |
| `branch_prefix` | string | Prefix prepended to each member's slugified Ref (default: `feat/`). |
| `base` | string | Ref the first member of every Chain branches from (default: `main`). |
| `[[chain]]` | table array | One entry per Chain. Each Chain's `tickets` array is ordered — position 0 branches from `base`, every later position branches from the member before it. |

A Chain of one Ticket never becomes a Stack. Chains within a Batch have no ordering between them and
run in parallel.

> **Chain depth is a review-latency multiplier.** Layer five cannot land until four reviews complete
> below it, and every merge below rewrites the base under the agents above. Deep Chains optimise
> writing throughput and pessimise review throughput, which is the opposite of the point. **Shallow
> and wide beats deep and narrow.**

**External dependency:** the Manifest is meant to be machine-written by whatever cuts your Tickets.
`to-tickets` (a separate tool/repo) must learn to emit it — until it does, Manifests are hand-authored
fixtures, and the feature is only as useful as that hand-authoring effort.

### `tp batch list`

List every Batch, its Chains, and each member's Ticket, Ref, branch, and base. Reads Manifests only —
creates no worktrees and calls `gh` for nothing.

```
tp batch list [--json]
```

### `tp batch sync`

Reconcile every Batch against the current worktrees and GitHub state: create the next materialisable
worktree in each Chain, link ready pull requests into a Stack with `gh stack link`, repair
divergence, and retire worktrees whose pull request merged.

```
tp batch sync [options]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | `-j` | Emit the Report as JSON instead of a table |
| `--dry-run` | `-n` | Print what would be created without touching anything |
| `--batch` | | Narrow to one Manifest by name |
| `--offline` | | Skip the `gh` call; report last-known pull request state |
| `--launch` | | Spawn `[batch] launch` for each materialised member with no Activity file yet |

**Requires `gh`.** Linking Chains into Stacks, and checking whether a Chain's parent branch has an
open pull request, both go through the `gh` CLI (`gh auth status`, `gh pr list`, `gh stack link`).
Without `gh` installed and authenticated, `tp batch sync` still materialises each Chain's first
member, but every later member reports `gh-required` instead of `blocked` or `created` — it degrades
gracefully rather than failing the whole run. `--offline` chooses this degraded mode deliberately,
reporting cached pull request state marked stale.

Each output row carries an `action`: `created`, `skipped` (branch already exists), `would-create`
(`--dry-run`), `blocked` (parent has no open pull request yet), `gh-required` (parent's pull request
state is unknown), `error`, or `launched`.

**`gh stack link` is additive only.** Treepad can build a Chain into a Stack, but it can **never take
one apart** — nothing here ever calls `gh pr edit --base`, and treepad stores no stack identity to
undo. Practically: **a Manifest edited after its Chain has been linked leaves a Stack on GitHub that
no treepad command can correct.** Fix a wrong Stack by hand on github.com.

Starting an agent is opt-in via `--launch`, and only fires for a member with a worktree but no
Activity file yet — see `[batch] launch` in [configuration.md](configuration.md#batch-section).
Without `--launch`, or with `[batch] launch` unconfigured, `tp batch sync` reports how many members
are ready to launch and starts nothing.

#### Examples

```bash
# See what a Batch would do without touching anything
tp batch sync --dry-run

# Reconcile every Batch, spawning agents for newly-ready members
tp batch sync --launch

# Reconcile one Batch by name, emitting JSON for scripting
tp batch sync --batch silent-refresh --json

# Skip the gh call entirely (e.g. offline, or gh not installed)
tp batch sync --offline
```

### Retired commands

`tp from-spec-bulk` is retired: a Batch of single-Ticket Chains replaces it. Invoking it still
resolves as a command, but its Action always fails, naming the Manifest as the replacement:

```
$ tp from-spec-bulk
tp from-spec-bulk is retired: a Batch of single-Ticket Chains replaces it — write a Manifest and run `tp batch sync` instead
```

## cd

cd into an existing worktree by branch name, or toggle back to the previous worktree.

```
tp cd <branch>
tp cd -
```

Looks up the worktree registered under `<branch>` from `git worktree list` and emits a `__TREEPAD_CD__` directive. The shell wrapper installed by `shell-init` intercepts it and changes the current directory. No flags — positional branch argument only.

Use `tp cd -` to go back to the previous worktree (like `cd -` in bash). The shell wrapper tracks the last visited worktree in the `$TP_PREV_WORKTREE` environment variable. If no previous worktree is set, an error is returned.

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

# Toggle back to the previous worktree
tp cd -

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

## doctor

Report cross-worktree health issues across the fleet.

```
tp doctor [options]
```

Scans all worktrees and emits a table (or JSON with `--json`) of findings. Each finding has a `KIND`, `BRANCH`, `DETAIL`, and `PATH` column. Exits 0 by default; use `--strict` to exit non-zero when any findings are present.

### Finding kinds

| Kind | Description |
|---|---|
| `stale` | No commit in more than `--stale-days` days (default: 30) |
| `dirty-old` | Uncommitted changes and no commit in more than `--stale-days` days |
| `merged-present` | Branch is merged into base but the worktree still exists |
| `remote-gone` | Branch no longer exists on the remote (requires network; skip with `--offline`) |
| `artifact-missing` | Expected artifact file is absent for a configured `[artifact]` section |
| `config-drift` | Worktree's `.treepad.toml` differs from the main worktree's config |
| `prunable` | Git marks the worktree as prunable (stale metadata) |

### Flags

| Flag | Short | Description |
|---|---|---|
| `--json` | `-j` | Emit JSON array instead of a table |
| `--stale-days` | | Threshold in days for stale/dirty-old checks (default: 30) |
| `--base` | `-b` | Branch to check merges against (default: `main`) |
| `--offline` | | Skip remote branch existence checks |
| `--strict` | | Exit non-zero if any findings are reported |

### Examples

```bash
# Show all health findings for the fleet
tp doctor

# Use a shorter stale threshold
tp doctor --stale-days 14

# Check merges against a different base
tp doctor --base develop

# Skip remote checks (faster, no network required)
tp doctor --offline

# Emit JSON for scripting
tp doctor --json

# Fail in CI if any issues are found
tp doctor --strict
```

### Output Example

```
KIND              BRANCH           DETAIL                                  PATH
stale             feat/old-work    last commit 45 days ago                 ~/myrepo-feat-old-work
merged-present    feat/done        branch already merged into main         ~/myrepo-feat-done
config-drift      feat/diverged    differs in: sync                        ~/myrepo-feat-diverged
```

## base

Return to the main worktree from any branch worktree.

```
tp base
```

Emits a `__TREEPAD_CD__` directive for the main worktree path (the one containing `.git`). Requires the shell integration wrapper; returns an error if you are already in the main worktree.

### Setup

Requires the same shell wrapper as `new` and `cd`:

```sh
eval "$(tp shell-init)"
```

### Examples

```bash
# Return to the main worktree from wherever you are
tp base
```

## config

Manage tp configuration files.

```
tp config <subcommand>
```

### config init

Write a config file with default values.

```
tp config init [--global] [--inherit] [--hooks-only] [--force]
```

By default, writes `.treepad.toml` to the main worktree root (the directory containing `.git`) populated with documented defaults. Use the `--global` flag to write to the global config path instead. Refuses to overwrite an existing config file unless `--force` is passed.

Use `--inherit` to seed the repo config from the global config instead — useful for onboarding a repo with your standard hooks, sync includes, etc. already in place. Combine with `--hooks-only` to keep the built-in defaults and lift only the `[hooks]` section from the global config.

**Hooks fired**: `post_config_init` (after the file is written; not fired with `--global`, since there is no repo to set up). See [hooks.md](hooks.md).

#### Flags

| Flag           | Short | Description                                                                              |
| -------------- | ----- | ----------------------------------------------------------------------------------------- |
| `--global`     | `-g`  | Write to the global config path instead of `.treepad.toml` in the main worktree           |
| `--inherit`    | `-i`  | Seed the config from the global config instead of the built-in defaults                   |
| `--hooks-only` | `-H`  | With `--inherit`, keep the built-in defaults and lift only `[hooks]` from the global config |
| `--force`      | `-f`  | Overwrite the config file if it already exists                                            |

#### Examples

```bash
# Write default config to the main worktree root
tp config init

# Write default config to the global config path
tp config init --global

# Onboard a repo with your global hooks (e.g. Claude settings, npx skills add)
tp config init --inherit

# Same, but keep this repo's built-in defaults for sync/artifact/open
tp config init --inherit --hooks-only
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
tp remove <branch> [options]
```

Removes the worktree for the specified branch, cleans up its associated artifact file (if any), and deletes the branch locally. Includes pre-flight safety guards to prevent accidental data loss.

### Flags

| Flag      | Short | Description                                                                        |
| --------- | ----- | ---------------------------------------------------------------------------------- |
| `--force` | `-f`  | Remove a worktree with uncommitted changes and delete the branch even if unmerged |

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

# Discard uncommitted changes and delete an unmerged branch
tp remove --force feature-x
```

### Errors

Attempting to remove the main worktree or the worktree you're currently in will return an error:

```
cannot remove the main worktree
cannot remove the worktree you are currently in; cd elsewhere first
```

A dirty worktree or an unmerged branch also fails; pass `--force` to override:

```
git worktree remove: ... contains modified or untracked files, use --force to delete it
git branch -d: ... the branch 'feature-x' is not fully merged
```

## prune

Remove all worktrees whose branches are already merged into a base branch, or force-remove all non-main worktrees. Useful for batch-cleaning completed work.

```
tp prune [options]
```

Automatically identifies and removes worktrees whose branches have been merged into a base branch (default: `main`). Executes removals directly; pass `--dry-run` to preview without making changes. Use `--all` to force-remove all non-main worktrees (with confirmation prompt).

### Flags

| Flag        | Short | Description                                                                                                            |
| ----------- | ----- | ---------------------------------------------------------------------------------------------------------------------- |
| `--base`    | `-b`  | Ref to check merges against (default: `main`)                                                                          |
| `--dry-run` | `-n`  | Preview removals without executing                                                                                     |
| `--all`     | `-a`  | Force-remove all non-main worktrees regardless of merge status (must be run from main worktree, requires confirmation) |
| `--yes`     | `-y`  | Skip confirmation prompt (use with caution)                                                                            |

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

| Flag     | Short | Description                                 |
| -------- | ----- | ------------------------------------------- |
| `--json` | `-j`  | Emit JSON array instead of an aligned table |

### Output Columns (Table Format)

| Column         | Meaning                                                                    |
| -------------- | -------------------------------------------------------------------------- |
| `BRANCH`       | Branch name, with `*` suffix if main worktree                              |
| `STATUS`       | `clean` or `dirty` (has uncommitted changes)                               |
| `AHEAD/BEHIND` | `↑N ↓M` vs upstream, or `—` if no upstream configured                      |
| `LAST COMMIT`  | Short SHA, subject, and relative time (e.g. `abc1234 fix thing · 3m`)      |
| `TOUCHED`      | Relative time since artifact file was last modified, or `—` if no artifact |
| `PATH`         | Absolute path (collapsed to `~/...` when under home directory)             |

### Examples

```bash
# Show status of all worktrees in a table (snapshot)
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

## ui

Open an interactive live fleet view in the terminal using a BubbleTea TUI. Requires a TTY; exits with code 2 if stdout is not a terminal.

```
tp ui
```

Renders a full-screen alt-screen display that auto-refreshes every 5 seconds. Shows the same worktree data as `tp status` plus a cursor for navigation and inline actions. When you navigate to a worktree and press Enter, `tp ui` exits and cd's your shell into that directory (requires shell integration).

Rows belonging to a Batch group by Batch then Chain, ordered by position within the Chain, main
first; a worktree with no Manifest entry still renders, ungrouped. Batch members show run state
(`pending`/`working`/`idle`, derived from the Activity file) and pull request state alongside the
usual columns; without `gh`, pull request columns show last-known state plus a staleness marker
rather than going blank. See [batch orchestration](#batch-orchestration) for the underlying model.

### Key Bindings

| Key            | Action                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------- |
| `0`–`9`        | Jump cursor to row N (row 0 is always the main worktree); shows an out-of-range notice if N ≥ number of rows |
| `↑` / `k`      | Move cursor up                                                                                                |
| `↓` / `j`      | Move cursor down                                                                                              |
| `Enter`        | Exit and cd into selected worktree                                                                            |
| `s`            | Sync selected worktree configs                                                                                |
| `S`            | Sync all worktrees (fleet sync)                                                                               |
| `o`            | Open artifact file for selected worktree                                                                      |
| `d`            | Diff selected worktree against base (default from config or `origin/main`)                                    |
| `e`            | Open an interactive shell (`$SHELL`, falling back to `/bin/sh`) in selected worktree — TUI suspends, shell runs full-screen, TUI resumes on exit (prompts for confirmation) |
| `l`            | Launch an agent for the selected Batch member via `[batch] launch` (prompts for confirmation; only enabled while the member's run state is `pending`) |
| `L`            | Launch every `pending` Batch member across the fleet (prompts for confirmation)                               |
| `v`            | Open the selected Batch member's Activity file in the pager — TUI suspends, pager runs full-screen, TUI resumes on exit |
| `y`            | Yank (copy) path to clipboard via OSC-52                                                                      |
| `r`            | Remove selected worktree (prompts for confirmation)                                                           |
| `R`            | Force-remove selected worktree — discards uncommitted changes and unmerged commits (prompts for confirmation) |
| `p`            | Prune merged worktrees (prompts for confirmation)                                                             |
| `/`            | Enter filter mode — type to fuzzy-match by branch or path basename                                            |
| `Esc`          | Clear active filter (from normal mode)                                                                        |
| `Enter`        | Commit filter query and return to normal mode (from filter mode)                                              |
| `Esc`          | Cancel filter and clear query (from filter mode)                                                              |
| `?`            | Toggle key binding help overlay                                                                               |
| `q` / `Ctrl-C` | Quit without cd                                                                                               |

### Notes

- Requires `eval "$(tp shell-init)"` for `Enter`→cd to take effect in the shell
- `r`, `R`, and `p` show an inline confirmation prompt; any key other than `y` cancels; `R` uses `git worktree remove --force` and `git branch -D` so it succeeds even on dirty or unmerged worktrees
- While a sync, remove, or prune action is in flight the cursor is locked and a spinner is shown; auto-refresh is paused until the action completes
- `y` writes the path to the system clipboard via the OSC-52 terminal escape sequence; supported by most modern terminal emulators

## diff

Show the diff of a worktree against a base branch using three-dot merge-base semantics.

```
tp diff [options] <branch> [-- <git-diff-args>...]
```

Displays the diff between the target worktree's branch and a base ref (default: `origin/main`, or the value of `[diff] base` in `.treepad.toml`) using `git diff <base>...HEAD` semantics, which matches the diff view in GitHub pull requests. The diff is shown in the terminal with color and paging inherited from the target worktree's git configuration (respects `delta`, `diff-so-fancy`, or other configured tools). Optionally writes a plain (uncolored) patch to a file with `--output`.

### Flags

| Flag       | Short | Description                                                                                                                |
| ---------- | ----- | -------------------------------------------------------------------------------------------------------------------------- |
| `--base`   | `-b`  | Ref to diff against (default: from config `[diff] base` or `origin/main`; not set via CLI flag unless explicitly provided) |
| `--output` | `-o`  | Write uncolored patch to `file` instead of terminal; outputs `[OK]` to stderr on success                                   |

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

## skill

Manage the agent skills treepad ships.

```
tp skill <subcommand>
```

### skill install

Install treepad's agent skills onto disk.

```
tp skill install [name...] [--local] [--force]
```

By default, installs every skill treepad embeds into `~/.agents/skills` — the shared, per-user location read natively by Codex, Cursor, Gemini CLI, Copilot CLI, and opencode (see [agentskills.io](https://agentskills.io)). Pass one or more names to install only those. Refuses to overwrite an existing skill directory unless `--force` is passed.

Claude Code doesn't read `~/.agents/skills` yet, so when a `.claude` directory is detected alongside the install target, `tp` also creates a symlink at `.claude/skills/<name>` pointing back at the canonical copy. No other harness needs this — they read `.agents/skills` directly.

Use `--local` to install into the main worktree instead of the user's home directory — useful when the skill should be committed and shared with the rest of the team. `.agents/` is a default `[sync] include` pattern, so a local install (and any `.claude` compat symlink) is carried into every worktree `tp new` creates.

#### Flags

| Flag        | Short | Description                                                                |
| ----------- | ----- | --------------------------------------------------------------------------- |
| `--local`   | `-l`  | Install into the repo instead of the user's home directory                  |
| `--force`   | `-f`  | Overwrite a skill directory (or compat symlink) that already exists         |

#### Examples

```bash
# Install every skill to ~/.agents/skills (and link ~/.claude/skills if present)
tp skill install

# Install into the repo instead, so it's committed and shared
tp skill install --local

# Reinstall to pick up an updated skill
tp skill install --force
```

### skill list

List the agent skills treepad ships.

```
tp skill list
```

## playbook

Manage the repo's Playbooks.

```
tp playbook <subcommand>
```

### playbook new

Write a Playbook to `.claude/playbooks/<name>.md` in the main worktree.

```
tp playbook new <name> [--force] < body.md
```

The body comes from stdin and is written **verbatim** — treepad composes nothing, interpolates nothing, and appends nothing. A Playbook is prose saying which Skills a recurring shape of work should use and why; the Ticket names it, and the agent picks the name up when it reads the Ticket. See [ADR 0002](adr/0002-treepad-writes-playbooks-not-prompts.md) for the design rationale, and [playbooks.md](playbooks.md) for best practices on what to write.

Refuses to overwrite an existing Playbook unless `--force` is passed. An empty body is an error. Add `".claude/playbooks/**"` to `[sync] include` so Playbooks reach every worktree — the built-in default already covers `.claude/`.

#### Flags

| Flag      | Short | Description                                    |
| --------- | ----- | ---------------------------------------------- |
| `--force` | `-f`  | Overwrite a Playbook that already exists       |

#### Examples

```bash
# Write a Playbook from a file
tp playbook new task-dashboard < playbook.md

# Or from a heredoc
tp playbook new task-dashboard <<'EOF'
Use /impeccable for the visual layer — this is dashboard work.
EOF

# Replace an existing Playbook
tp playbook new task-dashboard --force < playbook.md
```
