# Batch orchestration

`tp batch sync` reconciles a scoped collection of Tickets — declared once, up front, in a
Manifest — into a fleet of stacked worktrees, respecting blocking relationships between them.
It replaces the retired `tp from-spec-bulk`: a Batch of single-Ticket Chains is what that
command used to do.

## Contents

- Vocabulary
- The Manifest
- `tp batch list`
- `tp batch sync`
- The `gh` requirement
- Chain depth guidance
- The sharp edge: link is additive only
- Launching agents
- External dependency: `to-tickets`

## Vocabulary

Use these terms — they are the repo's own (see [CONTEXT.md](../../../CONTEXT.md)):

| Term | Meaning |
| --- | --- |
| **Batch** | A collection of Tickets treepad orchestrates as one unit, declared by a Manifest |
| **Manifest** | The uncommitted local file declaring one Batch's Chains — never written by treepad, never by hand in real use |
| **Chain** | An ordered run of Tickets within a Batch, each worktree branched from the one before it |
| **Stack** | GitHub's linked pull requests. A Chain *becomes* a Stack once its pull requests are linked |
| **Launcher** | The configured command treepad renders and runs to start an agent in a materialised worktree |
| **Activity file** | The per-worktree file whose mtime is the only evidence treepad has that an agent is working |

_Avoid_: "project", "epic", "bulk", "task", "job", "worker" — a Batch is none of those.

## The Manifest

A TOML file under `<git-common-dir>/treepad/batches/*.toml`. Treepad reads every file there and
unions them; a repo with no Manifests is normal.

```toml
name          = "silent-refresh"
branch_prefix = "feat/"
base          = "main"

[[chain]]
tickets = ["ENG-12", "ENG-13"]

[[chain]]
tickets = ["ENG-14"]
```

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | filename stem | The Batch's name |
| `branch_prefix` | string | `feat/` | Prepended to each member's slugified Ref |
| `base` | string | `main` | What Chain position 0 branches from — every later position branches from the member before it |
| `[[chain]]` | table array | — | One entry per Chain; `tickets` is ordered |

A Chain of one Ticket never becomes a Stack. Chains within a Batch have no ordering between
them and run in parallel — put independent work in separate Chains, not the same one, or the
top Ticket can't merge until the bottom one does.

## `tp batch list [--json]`

Lists every Batch, its Chains, and each member's Ticket, Ref, branch, and base. Reads Manifests
only — creates nothing, calls `gh` for nothing.

## `tp batch sync [options]`

| Flag | Description |
| --- | --- |
| `--json` / `-j` | Emit the Report as JSON instead of a table |
| `--dry-run` / `-n` | Print what would be created without touching anything |
| `--batch` | Narrow to one Manifest by name |
| `--offline` | Skip the `gh` call; report last-known pull request state, marked stale |
| `--launch` | Spawn `[batch] launch` for each materialised member with no Activity file yet |

Each run materialises the next creatable worktree per Chain, links ready pull requests into a
Stack, repairs safe divergence, and marks a member `removable` once its pull request merges.
Every row in the Report carries an `action`: `created`, `skipped`, `would-create` (`--dry-run`),
`blocked` (parent has no open pull request yet), `gh-required` (parent's pull request state is
unknown), `error`, or `launched`.

Partial failures are per-Chain, not per-Batch: a member that fails to materialise stops only its
own Chain; other Chains keep going. Exit code is 1 if any member's materialisation errored this
tick — read the Report for which.

## The `gh` requirement

Linking Chains into Stacks and checking whether a Chain's parent branch has an open pull request
both go through the `gh` CLI (`gh auth status`, `gh pr list`, `gh stack link`). Without `gh`
installed and authenticated, only each Chain's first member materialises; every later member
reports `gh-required` instead of `blocked` or `created` — it degrades rather than failing the
whole run. `--offline` opts into this deliberately.

## Chain depth guidance

**Chain depth is a review-latency multiplier.** Layer five cannot land until four reviews
complete below it, and every merge below rewrites the base under the agents above. Deep Chains
optimise writing throughput and pessimise review throughput, which is the opposite of the point.
**Shallow and wide beats deep and narrow.** Weigh this when writing or reviewing a Manifest.

## The sharp edge: link is additive only

`gh stack link` only ever adds — nothing in this codebase calls `gh pr edit --base`, and treepad
stores no stack identity to undo. Practically: **a Manifest edited after its Chain has been
linked leaves a Stack on GitHub that no treepad command can correct.** If a Chain's Tickets
change after linking, fix the Stack by hand on github.com; do not expect `tp batch sync` to
reorder or unlink anything.

## Launching agents

`[batch] launch` in `.treepad.toml` configures the Launcher — a Go template command, same
template family as `[from_spec] agent_command`, plus Batch-specific variables (`.Batch`,
`.Chain`, `.Position`, `.Ref`, `.ActivityFile`). `tp batch sync --launch` spawns it for every
member that has a worktree but no Activity file yet — that file's existence is the guard against
double-launching. Without `--launch`, or with `[batch] launch` unconfigured, `tp batch sync`
only reports how many members are ready.

`tp ui` offers the same launch via the `l` (one member) and `L` (all pending) keys, and opens a
member's Activity file with `v` — both behind a confirmation prompt.

## External dependency: `to-tickets`

The Manifest is meant to be authored by whatever cuts your Tickets, not by hand. `to-tickets` (a
separate tool/repo) must learn to emit it; until it does, Manifests are hand-authored fixtures.
Treepad never writes a Manifest and never reads the Tracker to check one against reality.
