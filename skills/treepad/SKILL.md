---
name: treepad
description: Manages git worktrees with the treepad CLI (`tp`) — creating branch workspaces, running commands and diffs inside them, checking fleet health, and tearing them down. Use this skill whenever a task touches git worktrees, the `tp` command, treepad, `.treepad.toml`, or several agent sessions working one repo in parallel, and whenever the user asks to spin up a branch workspace, work a ticket in its own directory, run or inspect something in another branch's checkout, see which worktrees are dirty or stale, or clean up finished branches — even if they never say "treepad" or "worktree". Prefer `tp` over raw `git worktree` commands in any repo where `tp` is on PATH, because `tp` also syncs local configs, writes the editor artifact, and fires lifecycle hooks that bare git skips.
---

# treepad (`tp`)

`tp` wraps `git worktree` so a new worktree arrives fully provisioned: gitignored local
configs copied in (`.env`, `.claude/settings.local.json`, `.agents/skills/`), an editor
artifact written, and lifecycle hooks fired. A worktree made with bare `git worktree add`
is missing all of that, which is why it looks fine and then fails to build.

Check availability once with `command -v tp`. If it is absent, fall back to `git worktree`
and say so rather than silently changing tools.

## You are not an interactive shell — read this first

`tp new`, `tp cd`, and `tp base` cannot change your directory. They hand a path to a shell
wrapper the user installed with `eval "$(tp shell-init)"`. You run each command in a fresh
non-interactive shell, so that wrapper never runs for you, and a bare `cd` in one tool call
does not survive into the next.

Three consequences shape almost every `tp` command you will write:

**1. Capture the path instead of expecting a cd.** Set `TREEPAD_CD_FD=3` and redirect fd 3
into the capture; `tp`'s progress output stays on stderr where you can still read it:

```bash
WT=$(TREEPAD_CD_FD=3 tp new feat-x 3>&1 1>&2)
```

`$WT` is now the absolute worktree path. Use it explicitly from then on — `cd "$WT" && ...`
inside a single command, or `git -C "$WT" ...`. Without `TREEPAD_CD_FD`, `tp` falls back to
printing a `__TREEPAD_CD__<tab><path>` line on stdout, which is parseable but noisier.

**2. Look up existing worktrees from JSON, not from a table.** `tp status --json` is the
machine-readable inventory: `branch`, `path`, `is_main`, `dirty`, `ahead`, `behind`,
`last_commit`, `last_touched`.

```bash
WT=$(tp status --json | jq -r '.[] | select(.branch=="feat-x") | .path')
```

Prefer this over `tp cd feat-x`, which only emits the directive. Never scrape the aligned
table from plain `tp status` — it is formatted for humans and its columns shift.

**3. Reach into a worktree without moving.** `tp exec <branch> <command>` runs in that
worktree and passes stdio straight through. If the command takes flags of its own, put `--`
after the branch or `tp` will try to parse them:

```bash
tp exec feat-x test              # routes through the detected runner (just/npm/make/uv)
tp exec feat-x -- git log --oneline -5   # raw command; -- protects the flags
```

`tp exec` needs a task-runner marker file (`justfile`, `package.json`, `Makefile`,
`pyproject.toml`) in the target worktree; in a repo with none it errors before running
anything, so use `git -C "$WT"` or `cd "$WT" && ...` there instead.

## Which command

| Intent | Command |
| --- | --- |
| Start work on a new branch | `tp new <branch>` (`--base <ref>` to branch off something other than `main`) |
| Start work from a ticket, with `PROMPT.md` written and an agent launched | `tp from-spec <branch> --ticket <url-or-ref>` |
| Fan several tickets out into their own worktrees at once | `tp from-spec-bulk --tickets ENG-12,ENG-14 --branch-prefix feat/` |
| Find where a branch's worktree lives | `tp status --json` + `jq` |
| Run a build/test/command in another worktree | `tp exec <branch> [--] <command>` |
| Review what a branch changed | `tp diff <branch>` (three-dot vs `origin/main`, matches the PR view) |
| See the whole fleet at a glance | `tp status` (human) / `tp status --json` (scripted) |
| Find stale, merged, drifted, or orphaned worktrees | `tp doctor` (`--json`, `--offline`, `--strict` for CI) |
| Copy configs into worktrees that predate a change | `tp sync` (`--sync-only` to skip artifact generation) |
| Finish one branch | `tp remove <branch>` |
| Clean up everything already merged | `tp prune` (`--dry-run` first) |
| Set up a repo for `tp` | `tp config init`, then `tp config show` to confirm what resolved |
| Give this repo's skills to an agent | `tp skill install` (`--local` to commit them into the repo) |

`tp ui` opens an interactive TUI. It needs a real TTY and exits 2 without one, so it is for
the user to run, not for you — suggest it, never invoke it.

## Working a branch end to end

```bash
WT=$(TREEPAD_CD_FD=3 tp new feat/retry-backoff --base main 3>&1 1>&2)
cd "$WT" && <make the change> && git add -A && git commit -m "..."
tp diff feat/retry-backoff --base main -- --stat   # confirm the shape of the change
```

Then, from anywhere except that worktree:

```bash
tp remove feat/retry-backoff
```

`tp remove` refuses to remove the main worktree, and refuses to remove the worktree you are
standing in — `cd` to the main worktree first (`tp status --json | jq -r '.[] | select(.is_main) | .path'`).
It removes the worktree, deletes its artifact file, and deletes the local branch, so only
run it when the work is merged or genuinely unwanted.

## Destructive commands

`tp remove`, `tp prune`, and `tp prune --all` delete branches and working directories.
`prune` without `--all` is fairly safe — it only touches branches already merged into the
base, and skips the main worktree, detached-HEAD worktrees, dirty worktrees, and the one you
are in. Even so, run `tp prune --dry-run` first and show the user what it would delete.

`tp prune --all` force-removes every non-main worktree regardless of merge status, and
`--yes` skips the confirmation prompt. Never combine them on your own initiative; that pair
discards unmerged, uncommitted work with nothing to recover it from. Ask first.

Because interactive prompts have no terminal in your shell, a confirmation-gated command
will hang or fail rather than proceed. If the user has clearly asked for the destructive
action, pass `--yes` deliberately and say that you did.

## Reference material

Read the one that matches the task rather than all of them:

- **[references/commands.md](references/commands.md)** — every command, every flag, exact
  output shapes and exit codes. Go here when you need a flag this page did not mention.
- **[references/configuration.md](references/configuration.md)** — `.treepad.toml` schema,
  resolution order, artifact templates, and lifecycle hooks. Go here when changing what
  gets synced, what artifact is generated, or what runs at a lifecycle event.
- **[references/from-spec.md](references/from-spec.md)** — ticket-driven worktrees: ticket
  URL resolution, `PROMPT.md` structure, agent handoff, bulk fan-out. Go here whenever a
  ticket, issue, or spec is the starting point.
- **[references/troubleshooting.md](references/troubleshooting.md)** — error messages mapped
  to cause and fix. Go here the moment a `tp` command fails instead of guessing.

`tp <command> --help` is the authoritative flag list and costs one cheap call; trust it over
any document, including this one, if they disagree.
