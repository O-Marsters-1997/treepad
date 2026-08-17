# Ticket-driven worktrees

`tp new --ticket` seeds a worktree from a ticket so an agent can start immediately, and
`tp from-spec-bulk` fans several tickets out at once.

## Contents

- Vocabulary
- What `tp new --ticket` actually does
- Ticket resolution
- Agent handoff
- Playbooks
- Bulk fan-out
- Configuration

## Vocabulary

Use these terms — they are the repo's own, and mixing in "issue" or "card" makes
conversations about the config ambiguous.

| Term | Meaning |
| --- | --- |
| **Ticket** | A unit of tracked work a worktree is created to deliver |
| **Spec** | The body text a Ticket carries |
| **Tracker** | The system hosting Tickets — Linear, GitHub, whatever the repo declares |
| **Ticket URL** | The canonical web address, from which Tracker and Ref both follow |
| **Ref** | The Tracker-local identifier — `42` on GitHub, `ENG-123` on Linear |
| **Playbook** | Prose at `.claude/playbooks/<name>.md` saying which Skills a shape of work should use, and why |

"Prompt" is retired: ADR 0002 deleted `PROMPT.md` rendering. Treepad authors no prose.

## What `tp new --ticket` actually does

```
tp new [options] <branch> --ticket <url-or-ref>
```

1. Resolves `--ticket` to a Ticket URL. This happens **before** anything is created, so an
   unresolvable ticket leaves no orphan worktree behind.
2. Creates the worktree exactly as a plain `tp new` does — config sync, artifact, `pre_new`,
   `pre_sync`/`post_sync`, `post_new`.
3. Runs `[from_spec] agent_command` in the worktree, with stdio passed through.
4. Emits the cd directive unless `--current` was passed.

**`tp` never reads the Tracker.** It has no API token, no `gh` call, no knowledge of Linear
or GitHub. It hands the agent the Ticket URL and the agent fetches the Spec itself. That is a
deliberate design decision (ADR 0001), not a gap — reading the cited URL is the agent's job.

**`tp` writes no prompt.** It composes no text and interpolates nothing (ADR 0002).

| Flag | Description |
| --- | --- |
| `--ticket` / `-t` | Ticket URL, or a bare Ref when `ticket_url` is configured |
| `--base` / `-b` | Ref to branch from (default `main`) |
| `--current` / `-c` | Do not emit the cd directive |

`--ticket` cannot be combined with `--open`.

## Ticket resolution

Input starting with `http://` or `https://` is used verbatim, and its Ref is the last path
segment. Anything else is treated as a Ref and rendered through the `[from_spec] ticket_url`
template with `{{.Ref}}`:

```toml
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
```

With that set, `--ticket ENG-123` becomes `https://linear.app/acme/issue/ENG-123`. Without
it, a bare ref fails:

```
no ticket_url configured: cannot resolve "ENG-123".
  set [from_spec] ticket_url in .treepad.toml, or pass the full ticket URL.
```

A full URL always works and always overrides the configured Tracker, which is how a repo
pulls a one-off ticket from somewhere else.

## Agent handoff

```toml
[from_spec]
agent_command = ["claude", "{{.TicketURL}}"]
```

Each element is a Go template with access to `.TicketURL`, `.Branch`, `.Slug` and
`.WorktreePath` — and nothing else. The command runs with the worktree as its working
directory, and `tp new --ticket` returns the agent's exit code.

A config still referencing `{{.PromptPath}}` or `{{.Skills}}` fails loudly with
`execute agent_command template`. There is no fallback; update the config.

With no `agent_command`, `tp new --ticket` creates the worktree, logs where it is, and exits.

Launching an interactive agent from inside another agent's shell is rarely what you want.
Prefer creating the worktree and leaving it:

```bash
tp new feat/x --ticket ENG-123 --current
```

## Playbooks

Skill routing is a Playbook's job. Write one with the body on stdin:

```bash
tp playbook new task-dashboard < playbook.md
```

The body is written **verbatim** to `.claude/playbooks/task-dashboard.md` in the main
worktree — treepad composes nothing, interpolates nothing, appends nothing. `--force` / `-f`
overwrites an existing Playbook; an empty body is an error.

Name it on the Ticket (`Playbook: task-dashboard`). The agent reads the Ticket anyway, so it
picks the name up for free, and the designation survives a re-run and works for bulk fan-out
without per-ticket flags. Playbooks propagate through `[sync]`; the default `.claude/`
pattern already covers them, and a narrowed config needs `".claude/playbooks/**"`.

Treepad never reads a Playbook. A Ticket naming one that does not exist fails inside the
agent, not as a `tp` error.

## Bulk fan-out

```
tp from-spec-bulk --tickets ENG-12,ENG-14,ENG-19 --branch-prefix feat/
```

One worktree per ticket. It never launches an agent and never emits a cd directive — there is
no single destination. Branch names are `--branch-prefix` plus the slugified Ref.

Partial failures are non-fatal: a bad ticket is recorded in the summary and the batch
continues. Exit code 1 means at least one ticket failed; read the table rather than assuming
total failure.

```
[STEP] RESULTS
[OK]     ENG-12  feat/eng-12   /Users/olly/code/repo-feat-eng-12
[WARN]   ENG-14  no ticket_url configured: cannot resolve "ENG-14"
[OK]     ENG-19  feat/eng-19   /Users/olly/code/repo-feat-eng-19
[INFO] 2 succeeded, 1 failed
```

Afterwards, each worktree is opened in its own terminal and its agent started there — that
separation is the point, since the whole purpose is parallel sessions that do not share a
context window.

## Configuration

```toml
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
agent_command = ["claude", "{{.TicketURL}}"]
```

| Field | Effect |
| --- | --- |
| `ticket_url` | Template expanding a bare Ref into a Ticket URL; empty means URLs only |
| `agent_command` | Argv run once the worktree exists; empty means create-and-exit |
