# Ticket-driven worktrees

`tp from-spec` seeds a worktree from a ticket so an agent can start immediately, and
`tp from-spec-bulk` fans several tickets out at once.

## Contents

- Vocabulary
- What `from-spec` actually does
- Ticket resolution
- The generated `PROMPT.md`
- Agent handoff
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
| **Prompt** | The `PROMPT.md` a worktree is seeded with |

## What `from-spec` actually does

```
tp from-spec [options] <branch> --ticket <url-or-ref>
```

1. Resolves `--ticket` to a Ticket URL. This happens **before** anything is created, so an
   unresolvable ticket leaves no orphan worktree behind.
2. Creates the worktree exactly as `tp new` does — config sync, artifact, `pre_new`,
   `pre_sync`/`post_sync`, `post_new`.
3. Writes `PROMPT.md` into the worktree root.
4. Runs `[from_spec] agent_command` in the worktree, with stdio passed through.
5. Emits the cd directive unless `--current` was passed.

**`tp` never reads the Tracker.** It has no API token, no `gh` call, no knowledge of Linear
or GitHub. It writes the Ticket URL into the prompt and the agent fetches the Spec itself.
That is a deliberate design decision (ADR 0001), not a gap — so if a `PROMPT.md` looks
empty of requirements, that is expected, and reading the cited URL is the agent's job.

| Flag | Description |
| --- | --- |
| `--ticket` / `-t` | Ticket URL, or a bare Ref when `ticket_url` is configured (required) |
| `--base` / `-b` | Ref to branch from (default `main`) |
| `--current` / `-c` | Do not emit the cd directive |
| `--prompt` / `-p` | Instructions replacing the default closing "Implement the ticket." |

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

## The generated `PROMPT.md`

```markdown
# <branch>

## Spec
Read the ticket at:
<ticket-url>

## Skills
- /tdd
- /code-review

Implement the ticket.
```

The `## Skills` block appears only when `[from_spec] skills` is non-empty. `--prompt "..."`
replaces the closing line with:

```
Implement the ticket according to the following instructions:

<your text>
```

If `PROMPT.md` already exists in the worktree it is reused untouched and no new prompt is
generated — which is how you hand-edit a prompt and re-run without losing your edits.

## Agent handoff

```toml
[from_spec]
agent_command = ["claude", "{{.PromptPath}}"]
```

Each element is a Go template with access to `.Spec`, `.Skills`, `.Branch`, `.Slug`,
`.WorktreePath`, `.PromptPath`, and `.Prompt` (the rendered prompt body). The command runs
with the worktree as its working directory, and `from-spec` returns the agent's exit code.

With no `agent_command`, `from-spec` writes the prompt, logs where it went, and exits —
useful when you want to review the prompt before launching anything.

Launching an interactive agent from inside another agent's shell is rarely what you want.
Prefer previewing:

```bash
tp from-spec feat/x --ticket ENG-123 --current
cat <worktree>/PROMPT.md
```

## Bulk fan-out

```
tp from-spec-bulk --tickets ENG-12,ENG-14,ENG-19 --branch-prefix feat/
```

One worktree and one `PROMPT.md` per ticket. It never launches an agent and never emits a cd
directive — there is no single destination. Branch names are `--branch-prefix` plus the
slugified Ref, with the Ref appended if two would collide.

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
skills = ["tdd", "code-review"]
agent_command = ["claude", "{{.PromptPath}}"]
```

| Field | Effect |
| --- | --- |
| `ticket_url` | Template expanding a bare Ref into a Ticket URL; empty means URLs only |
| `skills` | Skill names listed in every generated prompt's `## Skills` block |
| `agent_command` | Argv run after the prompt is written; empty means write-and-exit |
