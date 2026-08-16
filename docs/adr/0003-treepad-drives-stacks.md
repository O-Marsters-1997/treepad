# 3. Treepad drives Stacks, and restacks worktrees itself

Date: 2026-08-16

## Status

Accepted

## Context

[ADR 0001](./0001-treepad-does-not-read-trackers.md) removed `gh` as a dependency of
`from-spec`, on the grounds that treepad should not be a second, worse client for data the
agent can fetch for itself. Batch orchestration puts `gh` back.

GitHub shipped stacked pull requests to public preview on 2026-07-30. A **Stack** is a
strictly linear series of pull requests in one repository, each targeting the branch below
it, and it is created only through the `gh stack` CLI extension or the web UI. There is no
other mechanism. A **Chain** — treepad's ordered run of Tickets, each worktree branched from
the one before it — is the work-side shape that produces exactly the branch topology a Stack
requires. Nothing connects the two without `gh`.

GitHub sanctions this exact integration. `gh stack link` is documented as "designed for people
who manage branches with other tools locally, such as Jujutsu, Sapling, or git-town, and want
to open a stack of pull requests", and it "does not create or modify any local tracking state".
Treepad is that category of tool.

Two things force the division of labour.

**`gh stack` assumes a single checkout.** `gh stack init` and `gh stack add` create a branch
at HEAD *and check it out*; `gh stack modify` requires a clean working tree with the active
stack checked out. The documented workflow is one developer moving upward through the stack
one branch at a time. A fleet is the opposite: every branch in the Chain is already checked
out, in its own worktree, with an agent in it.

**`gh stack link` deliberately leaves no local tracking.** That is the point of it — and the
consequence is that `gh stack rebase` and `gh stack sync`, which need the tracking state
`gh stack init` would have created, are not available to a tool that links. Restacking is the
external tool's job by design, not by workaround. Adopting branches with `gh stack init` to
regain those commands is the documented alternative, and is the thing a fleet cannot do.

**And `gh stack rebase` and `sync` fail silently against a fleet anyway.** Git refuses to check out a
branch that is checked out in another worktree, so the cascading rebase cannot start —
and gh-stack reports success anyway ([#35](https://github.com/github/gh-stack/issues/35),
open). `gh stack sync` has a matching report ([#87](https://github.com/github/gh-stack/issues/87),
open). Local stack state lives in `.git/gh-stack`, resolved from `--git-dir`, which in a
linked worktree is `.git/worktrees/<name>/` — so a worktree may not see the stack at all.
The post-merge repair GitHub documents, `gh stack sync --prune`, is therefore precisely the
command a worktree fleet cannot run.

Repair is unavoidable, because merging a lower pull request rewrites every branch above it
**server-side**. The moment a reviewer merges the bottom of a Chain, every worktree above it
holds a local branch pointing at commits that no longer exist upstream — possibly with an
agent mid-edit inside it.

## Decision

Treepad drives Stacks through `gh`, and owns restacking itself.

**`gh` is a hard dependency of Batch orchestration**, and this does not contradict ADR 0001.
A Stack is keyed by the **git remote** treepad already knows, not by a Tracker. There is no
per-provider code path, no credential storage, and no API client — only an installed `gh`.
ADR 0001 protects treepad from knowing about Trackers; it does not protect treepad from
knowing about git remotes, which it has always known about.

Treepad calls exactly two things:

- `gh pr list` — batched for the whole repo on a slow tick, never per branch, to learn which
  branches have pull requests
- `gh stack link <branch> <branch> …` — arguments in stack order, bottom to top, no checkout
  required and no local state written

Nothing corrects pull request bases, because `link` already does: *"Existing pull requests
whose base branch does not match the expected chain are corrected automatically."* A `gh pr
edit --base` call would be redundant.

Treepad passes **only branches that already have a pull request**. This is a real constraint,
not a preference: `link` pushes every branch argument to the remote and *creates* pull
requests for any that lack one. Passing a Chain member whose agent is still working would push
unfinished work and open a pull request for it.

Treepad re-lists the whole Chain-so-far on every link rather than tracking stack numbers.
`link` is additive and idempotent — arguments already in the stack are skipped, and existing
pull requests are never removed — so there is no stack identity for treepad to persist. The
corollary is that a Chain cannot be un-linked or reordered through `link`; that is a human
job on github.com.

Treepad **never** calls `gh stack init`, `add`, `submit`, `modify`, `rebase`, or `sync`.
The first four assume a single checkout; the last two need tracking state `link` does not
create, and fail silently against a fleet even where it exists.

**Treepad restacks in-worktree.** When a merge rewrites a branch upstream, treepad repairs it
inside its own worktree, where the branch is legitimately checked out and plain `git` works.
Repair is automatic only when the worktree is clean and has nothing unpushed — a
fast-forward to `origin`, which cannot lose work. Anything dirty or ahead is reported as
`stack-stale` and waits for a human. Treepad never stashes on an agent's behalf.

**Treepad does not merge.** Its responsibility ends at a linked Stack with correct bases and
coherent worktrees. Reviewers merge on github.com or with `gh stack merge`; treepad's tick
notices, repairs the branches above, and marks the merged worktree removable.

## Consequences

**Good.** Treepad does the one thing no tool in the category can: it holds the whole fleet in
one process, so it can restack every branch in place rather than fighting git's
one-checkout-per-branch rule. The `gh` surface is two commands, both keyed by the remote, so
there is still no auth code, no HTTP client, and no provider enum. Declining
to merge keeps treepad free of any irreversible remote mutation, and it serves the actual
goal — small chunks reviewed and merged one at a time, which whole-stack atomic merge works
against.

**Bad.** `gh` is now required for a headline feature, seven months after ADR 0001 removed it,
and a user without `gh` gets a Batch that provisions and launches but never links. Stacked
pull requests are in public preview and subject to change, so `gh stack link`'s behaviour is
a moving target — in particular its automatic base correction, which treepad relies on rather
than enforcing itself. And because `link` is additive only, treepad can build a Chain into a
Stack but can never take one apart: a Manifest edited after linking leaves a Stack on GitHub
that no treepad command can correct.

**Also.** The filter on "branches that already have a pull request" is load-bearing and easy to
lose. A refactor that passes the whole Chain to `link` for tidiness would silently push every
agent's work-in-progress and open pull requests for it. Whatever builds the argument list needs
a test that a Chain member without a pull request is excluded.

**And.** Because a Chain's pull requests must merge bottom-up, Chain depth is a review-latency
multiplier: layer five cannot land until four reviews complete below it, and every merge
below rewrites the base under the agents above. Deep Chains optimise writing throughput and
pessimise review throughput, which is the opposite of the point. Shallow and wide beats deep
and narrow — guidance for whatever writes the Manifest, and a candidate `doctor` warning.

## Alternatives considered

**Delegate restacking to `gh stack sync --prune`.** The documented post-merge repair, and it
would leave treepad with no rebase logic at all. Rejected twice: `link` writes no local
tracking, so `sync` has nothing to work from — and even after `gh stack init`, against a fleet
it cannot move a single branch and exits zero while failing. A wrong answer reported as
success is worse than no answer.

**Adopt the branches with `gh stack init`, then submit.** The documented "full integration"
path for externally-managed branches, and it would buy the entire `gh stack` command suite.
Rejected because `init`/`add` create a branch at HEAD and check it out, and `modify` requires
the active stack checked out with a clean tree — all incompatible with one worktree per branch
by construction. Treepad would have to abandon worktrees to use it, which is abandoning
treepad.

**Avoid `gh` entirely; gate on branches being pushed instead of pull requests existing.**
Pure git, no dependency, and true earlier. Rejected because a Stack cannot be created without
`gh` at all, so the dependency is unavoidable for the feature — and a pushed branch is not a
promise the work is reviewable, so an agent pushing a work-in-progress commit at minute two
would unblock the layer above against nothing.

**Merge from the fleet view.** One keystroke to land a Chain, and the fleet view is the only
place that knows the whole shape. Rejected twice over: it would be the first thing treepad
does that cannot be undone, and `gh stack merge` lands every layer up to the chosen one
atomically — the opposite of small chunks reviewed independently.

**Track agent processes by PID to drive the fleet view.** Exact liveness and exit codes.
Rejected because the Launcher is a config template: a tmux or terminal-opening Launcher exits
within milliseconds while the agent runs for an hour, and a Conductor-launched agent was never
treepad's child. Liveness comes from an Activity file's mtime instead, which is true
regardless of who started the work.
