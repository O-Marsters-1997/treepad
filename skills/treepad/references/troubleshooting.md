# When `tp` fails

Error text is on stderr behind an `[ERR]` marker. Add `--verbose` for the git commands `tp`
actually ran before theorising about a cause.

## Contents

- Directory did not change
- Worktree not found
- Removal refused
- `exec` failures
- `diff` failures
- Creation failures
- Ticket failures
- Config and skills

## Directory did not change

**`tp new` printed `__TREEPAD_CD__<tab><path>` and stayed put.** Expected in a
non-interactive shell — no wrapper is listening. Capture the path instead:

```bash
WT=$(TREEPAD_CD_FD=3 tp new feat-x 3>&1 1>&2)
```

If a *user* reports this in their own terminal, they are missing
`eval "$(tp shell-init)"` in `~/.zshrc` or `~/.bashrc`, or have not re-sourced it.

**`stale shell wrapper detected — re-run: eval "$(tp shell-init)"`.** Their wrapper predates
the fd-3 protocol. Re-running `eval "$(tp shell-init)"` fixes it; the command still works
meanwhile.

**`tp cd -` says `no previous worktree`.** `$TP_PREV_WORKTREE` is maintained only by the
shell wrapper, so it is always empty in an agent shell. Look the path up with
`tp status --json`.

**`already on the default worktree`** — `tp base` was run from the main worktree. Not an
error worth reporting; you are already where you asked to be.

## Worktree not found

```
no worktree found for branch "X"; run `tp sync` to list worktrees
```

The branch has no registered worktree. Confirm the exact name — worktree lookup is by branch
name, not by directory name or slug:

```bash
tp status --json | jq -r '.[].branch'
```

Create it with `tp new X` if it should exist. If the branch exists but the worktree was
deleted by hand, `git worktree prune` clears the stale metadata.

## Removal refused

| Message | Fix |
| --- | --- |
| `cannot remove the main worktree` | Nothing to do — that is the repo. Remove a branch worktree instead. |
| `cannot remove the worktree you are currently in; cd elsewhere first` | `cd` to the main worktree path from `tp status --json \| jq -r '.[] \| select(.is_main) \| .path'`, then re-run in the same command. |
| `branch not found` | The worktree is gone but the branch name is wrong — check `git branch --list`. |

`tp prune` silently skipping a branch is usually correct: it skips the main worktree,
detached-HEAD worktrees, dirty worktrees, and the one you are in. `--dry-run` prints exactly
which and why.

## `exec` failures

**`flag provided but not defined: -short`** (or any flag of the command you meant to run) —
`tp` parsed the flag as its own. Put `--` after the branch:

```bash
tp exec feat-x -- git status --short
```

**`no task runner detected; check that a justfile, package.json, Makefile, or pyproject.toml
is present`** — `tp exec` resolves a runner before running anything, even for a raw command,
so a repo with no marker file cannot use it at all. Work in the worktree directly:

```bash
WT=$(tp status --json | jq -r '.[] | select(.branch=="feat-x") | .path')
git -C "$WT" status --short
```

**A non-zero exit with no `tp` error** — `tp exec` returns the child's exit code unchanged.
The failure is the command's, not treepad's.

## `diff` failures

**`fatal: ambiguous argument 'origin/main...HEAD': unknown revision`** — the default base is
`origin/main`, and this repo has no remote or has not fetched it. Pass a local base:

```bash
tp diff feat-x --base main
```

Set `[diff] base` in `.treepad.toml` to make it permanent.

**`worktree for "X" is prunable (...); run tp prune`** — git considers that worktree's
metadata stale. `tp prune` clears it.

**The diff is empty but the worktree has changes.** `tp diff` compares committed tips using
`<base>...HEAD`. Uncommitted work is invisible to it — use `git -C "$WT" diff` for that.

## Creation failures

**`branch already exists`** — a branch of that name exists without a worktree. Either pick a
different name, or check out the existing branch in a worktree with
`git worktree add <path> <branch>` and re-run `tp sync` to provision it.

**`git worktree add: ...`** — the underlying git error is the real message. Common causes: an
existing directory at the target path, or a `--base` ref that does not exist locally
(`git fetch` first).

**`could not find main worktree (no .git directory found)`** — the command was run outside
any git repo, or inside a worktree whose main repo has moved. `cd` into the repo first.

**A hook aborted it.** A failing `pre_new`/`pre_sync`/`pre_remove` hook stops the operation
and its error surfaces as `<event> hook: <error>`. Look at `[hooks]` in `tp config show`;
hook output is discarded unless the entry sets `interactive = true`, so re-run the hook
command by hand to see why it failed.

## Ticket failures

**`no ticket_url configured: cannot resolve "ENG-123"`** — a bare Ref needs
`[from_spec] ticket_url` in `.treepad.toml`. Pass the full ticket URL instead, or add the
template.

**`--open is not supported with --ticket`** — there is no artifact-open path for a
ticket-driven run; drop one of the two flags.

**`execute agent_command template`** — the config references a field that no longer exists.
`{{.PromptPath}}`, `{{.Skills}}`, `{{.Spec}}` and `{{.Prompt}}` were deleted by ADR 0002.
The available data is `.TicketURL`, `.Branch`, `.Slug`, `.WorktreePath`.

**`tp batch sync` exited 1 but some worktrees exist.** That is the designed behaviour:
a member's materialisation error stops only its own Chain, and the exit code only reports that
at least one member errored this tick. Read the Report for which. See [batch.md](batch.md).

## Config and skills

**Config edits had no effect.** Run `tp config show` — a global config or a different
`.treepad.toml` may be winning, and setting `[sync] include` replaces the defaults rather
than extending them.

**`<path> already exists; pass --force to overwrite`** — from `tp config init` or
`tp skill install`. Confirm the existing file is safe to lose before adding `--force`.

**`unknown skill "X"; available: [...]`** — `tp skill install` only installs skills embedded
in the binary. `tp skill list` shows what exists.

**`tp ui requires an interactive terminal`** (exit 2) — `tp ui` cannot run in an agent shell.
Use `tp status` / `tp status --json`, and suggest `tp ui` to the user for interactive work.
