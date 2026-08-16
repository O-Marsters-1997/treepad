# tp command reference

Every command, its flags, and what it actually emits. `tp <command> --help` wins over this
file if they ever disagree.

## Contents

- Global flags
- Creating: `new`, `from-spec`, `from-spec-bulk`
- Navigating: `cd`, `base`, `shell-init`
- Inspecting: `status`, `doctor`, `diff`, `ui`
- Operating: `exec`, `sync`
- Removing: `remove`, `prune`
- Setup: `config`, `skill`
- Exit codes

## Global flags

| Flag | Effect |
| --- | --- |
| `--verbose` / `-v` | Debug logging to stderr. First thing to add when a command misbehaves. |
| `--profile` | Per-stage timing breakdown to stderr after the command finishes. |

All human-facing output (`[OK]`, `[STEP]`, `[INFO]`, `[WARN]`, `[ERR]`) goes to stderr.
Stdout carries only machine payloads: `--json` output, the `__TREEPAD_CD__` directive,
`shell-init`'s function body, and passed-through child process output from `exec`.

## Creating

### `tp new [options] <branch>`

Creates the worktree, syncs configs from the main worktree into it, writes the artifact
file, and emits a cd directive. Hooks: `pre_new`, `pre_sync`/`post_sync`, `post_new`.

| Flag | Description |
| --- | --- |
| `--base` / `-b` | Ref to branch from (default `main`) |
| `--open` / `-o` | Open the artifact via `[open] command` after creation |
| `--current` / `-c` | Do not emit the cd directive |

Worktree paths are siblings of the main worktree, named `<repo-slug>-<slugified-branch>`.
Capture the exact path rather than reconstructing it:

```bash
WT=$(TREEPAD_CD_FD=3 tp new feat/x 3>&1 1>&2)
```

### `tp from-spec [options] <branch>`

Same creation path as `new`, plus: resolves a ticket to its spec, writes `PROMPT.md` into
the worktree, and runs `[from_spec] agent_command`. If `PROMPT.md` already exists it is
reused as-is. See [from-spec.md](from-spec.md).

| Flag | Description |
| --- | --- |
| `--ticket` / `-t` | Ticket URL, or a bare ref when `[from_spec] ticket_url` is configured (required) |
| `--base` / `-b` | Ref to branch from (default `main`) |
| `--current` / `-c` | Do not emit the cd directive |
| `--prompt` / `-p` | Instructions replacing the default closing "Implement the ticket." |

### `tp from-spec-bulk [options]`

One worktree per ticket, each with its own `PROMPT.md`. Never launches an agent and never
emits a cd directive — there is no single destination. Branch names are the slugified ref
with `--branch-prefix` prepended; a collision gets the ref appended.

| Flag | Description |
| --- | --- |
| `--tickets` / `-t` | Comma-separated ticket URLs or refs, e.g. `"ENG-12,ENG-14"` (required) |
| `--branch-prefix` | Prepended to each derived branch name, e.g. `feat/` |
| `--base` / `-b` | Ref every worktree branches from (default `main`) |
| `--prompt` / `-p` | Instructions appended to every prompt body |

Partial failures are non-fatal: bad tickets are reported in the summary table and the rest
of the batch continues. Exit code is 1 if any ticket failed.

## Navigating

### `tp cd <branch>` / `tp cd -`

Resolves the branch to its worktree path and emits the cd directive. `tp cd -` toggles to
the previous worktree using `$TP_PREV_WORKTREE`, which only the shell wrapper maintains —
it has no meaning in a non-interactive shell.

### `tp base`

Emits the cd directive for the main worktree. Errors with "already on the default worktree"
if you are there. `tp status --json | jq -r '.[] | select(.is_main) | .path'` gets the same
answer without the directive.

### `tp shell-init`

Prints the shell wrapper function to stdout. Users add `eval "$(tp shell-init)"` to
`~/.zshrc` or `~/.bashrc`. Nothing to run in an agent shell; suggest it when a user reports
that `tp new` is not changing their directory.

## Inspecting

### `tp status [--json]`

The fleet inventory. JSON objects carry:

```json
{
  "branch": "feat/x",
  "path": "/abs/path/repo-feat-x",
  "is_main": false,
  "dirty": true,
  "ahead": 2,
  "behind": 0,
  "has_upstream": true,
  "last_commit": {"sha": "abc1234", "subject": "...", "committed": "2026-04-15T15:07:51+01:00"},
  "artifact_path": "/abs/path/repo-workspaces/repo-feat-x.code-workspace",
  "last_touched": "2026-04-13T20:07:27.882Z"
}
```

`last_touched` is the artifact file's mtime — a proxy for recent editor or agent activity,
not for commits. A zero value (`0001-01-01T00:00:00Z`) means no artifact exists.

Useful queries:

```bash
tp status --json | jq -r '.[] | select(.dirty) | .branch'                 # uncommitted work
tp status --json | jq -r '.[] | select(.is_main) | .path'                 # main worktree
tp status --json | jq -r '.[] | select(.branch=="feat/x") | .path'        # one branch
tp status --json | jq -r '.[] | select(.behind > 0) | "\(.branch) ↓\(.behind)"'
```

### `tp doctor [options]`

Health findings across the fleet. Each finding is `{branch, path, kind, detail}`.

| Kind | Meaning |
| --- | --- |
| `stale` | No commit within `--stale-days` |
| `dirty-old` | Uncommitted changes *and* no recent commit |
| `merged-present` | Branch merged into base but the worktree still exists |
| `remote-gone` | Branch no longer on the remote (needs network) |
| `artifact-missing` | Configured artifact file absent |
| `config-drift` | Worktree's `.treepad.toml` differs from the main worktree's |
| `prunable` | Git considers the worktree metadata stale |

| Flag | Description |
| --- | --- |
| `--json` / `-j` | JSON array instead of a table |
| `--stale-days` | Threshold in days (default 30) |
| `--base` / `-b` | Branch to check merges against (default `main`) |
| `--offline` | Skip remote checks — faster, and required with no network |
| `--strict` | Exit non-zero when any finding is reported |

`doctor` reports; it never changes anything. Act on `merged-present` with `tp prune`, on
`artifact-missing` or `config-drift` with `tp sync`.

### `tp diff [options] <branch> [-- <git-diff-args>...]`

`git diff <base>...HEAD` inside the target worktree — three-dot, so it shows what the branch
added since diverging, matching the GitHub PR view. Diffs the committed tip; uncommitted
changes in that worktree are invisible to it.

| Flag | Description |
| --- | --- |
| `--base` / `-b` | Ref to diff against (default `[diff] base` in config, else `origin/main`) |
| `--output` / `-o` | Write an uncolored patch to a file instead of the terminal |

Everything after `--` is forwarded to `git diff`: `-- --stat`, `-- --word-diff`,
`-- -- src/` to scope by path. The default base is `origin/main`, so in a repo with no
remote pass `--base main` explicitly.

### `tp ui`

Interactive TUI over the same data as `status`, with inline sync/diff/remove/prune actions.
Requires a TTY and exits 2 without one. Recommend it to the user; do not invoke it.

## Operating

### `tp exec [--] <branch> [command] [args...]`

Runs a command in the named worktree with full stdio passthrough, returning the child's exit
code. With no command, prints the detected runner and its enumerated scripts.

The runner (`just`, `npm`/`pnpm`/`yarn`/`bun`, `make`, `poetry`/`uv`) is detected from
marker files, or forced with `[exec] runner` in `.treepad.toml`. If the command matches an
enumerated script it routes through the runner; otherwise it runs directly in the worktree
root.

Two things reliably trip this command up:

- **Flags get eaten.** `tp exec feat-x git status --short` fails with
  `flag provided but not defined: -short`. Write `tp exec feat-x -- git status --short`.
- **No runner, no exec.** With no `justfile`/`package.json`/`Makefile`/`pyproject.toml` in
  the worktree it errors out before running anything, even for a raw command. Use
  `git -C "$WT" ...` or `cd "$WT" && ...` in such repos.

### `tp sync [options] [source-path]`

Copies the configured files from a source worktree into every other worktree and regenerates
artifacts. Source precedence: explicit `source-path` argument, then `--current`, then the
auto-detected main worktree. Hooks: `pre_sync`/`post_sync` per worktree.

| Flag | Description |
| --- | --- |
| `--current` / `-c` | Use the current directory as the source (alias `--use-current`) |
| `--sync-only` | Copy files only; skip artifact generation |
| `--output-dir` / `-o` | Artifact output directory (default `~/<repo-slug>-workspaces/`) |
| `--include` | Extra glob, repeatable; appended to `[sync] include` |

Reach for it after editing a local config that other worktrees need, or to repair
`artifact-missing` / `config-drift` findings from `doctor`.

## Removing

### `tp remove <branch>`

Removes the worktree, deletes its artifact file, deletes the local branch. Hooks:
`pre_remove`, `post_remove`. Refuses to remove the main worktree, and refuses to remove the
worktree you are currently inside.

### `tp prune [options]`

| Flag | Description |
| --- | --- |
| `--base` / `-b` | Ref to check merges against (default `main`) |
| `--dry-run` / `-n` | Print what would be removed and exit |
| `--all` / `-a` | Force-remove every non-main worktree regardless of merge status |
| `--yes` / `-y` | Skip the confirmation prompt |

Default mode removes only branches merged into the base, and skips the main worktree,
detached-HEAD worktrees, dirty worktrees, and the worktree you are in. `--all` keeps only
the main worktree, removes detached-HEAD and dirty worktrees too, must be run from the main
worktree, and prompts for confirmation. `--all --yes` is unrecoverable — get explicit
agreement before running it.

## Setup

### `tp config init [options]` / `tp config show`

`init` writes a documented `.treepad.toml` to the main worktree root; `show` prints the
resolved config and which sources produced it. Hook: `post_config_init` (not fired with
`--global`).

| Flag | Description |
| --- | --- |
| `--global` / `-g` | Write to the global config path instead of the repo |
| `--inherit` / `-i` | Seed from the global config rather than built-in defaults |
| `--hooks-only` / `-H` | With `--inherit`, take only `[hooks]` from the global config |
| `--force` / `-f` | Overwrite an existing config file |

### `tp skill install [name...] [--local] [--force]` / `tp skill list`

Installs the skills `tp` embeds into `~/.agents/skills`, the shared location Codex, Cursor,
Gemini CLI, Copilot CLI, and opencode read directly. Claude Code does not read it yet, so
when a `.claude` directory sits alongside the target, `tp` also symlinks
`.claude/skills/<name>` at the canonical copy.

`--local` installs into the repo instead of `$HOME`, so the skill is committed and shared;
`.agents/` is a default sync pattern, so a local install propagates into every worktree
`tp new` creates afterwards.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Command failed; `[ERR]` line on stderr explains why |
| 1 | `from-spec-bulk` with at least one failed ticket (the rest still succeeded) |
| 2 | `tp ui` with no TTY |
| child | `tp exec` returns the child process's exit code unchanged |
