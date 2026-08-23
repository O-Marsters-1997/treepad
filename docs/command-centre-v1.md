# Command Centre — v1 design

**Date:** 2026-08-20 · **Revision 2** · **Status:** designed, not built

**Supersedes:** `.claude/handoffs/treepad__command-centre-architecture.md`, and revision 1 of
this document. Revision 1 was reviewed in
`docs/architecture-reviews/command-centre-v1-review.md`, which found four false premises
and a set of real defects; this revision is the response. §14 records what changed and why.

A local app that takes a DAG of tickets across several repos and drives them to reviewable
pull requests using agents, one worktree per ticket. The user writes specs and reviews PRs;
everything between those two acts is the app's job.

It does not decide *what* to build (planning skills do) and it does not perform git or
GitHub mechanics (`treepad` does). It launches agents, watches them, pushes what they
produced, reads CI, and shows one page.

**Non-goals.** It never merges, never rebases, never runs tests, and never decides a ticket
is done. CI decides whether work is sound; the user decides done by merging.

---

## 1 · Division of labour

| Layer | Singularly responsible for | Must never |
|---|---|---|
| `to-plan` → `to-seams` → `to-tickets` | Above-ticket knowledge: which repos, which edges, which integration seams | Execute anything |
| **App** | Launching agents, liveness, pushing, PR draft state, CI reading, one status page | Merge, rebase, run tests, hold tracker or domain knowledge |
| `tp` (treepad) | One repo's worktrees, branch bases, PR stack linking, in-worktree repair | Know that a workspace or a second repo exists |
| CI | Whether the work is sound | — |
| GitHub | Review, merge | — |

The app calls `tp` as a subprocess, once per repo per tick, and never imports treepad's
`batch/` package — separate repos make that structural rather than a rule.

The app's readiness gate for a chain descendant is deliberately stated as **OPEN or
MERGED**, matching `batch.ReadyToMaterialise` (`batch/ready.go:14-22`). Revision 1 claimed
the app's gate was OPEN only and that this coincided with treepad's; it does not, and the
divergence stranded descendants whose parent merged before they got a concurrency slot —
on the happy path, whenever review is faster than the launch.

## 2 · Data flow

```
spec.md
  │  to-plan          (one small feature; flags cross-repo seams)
  ▼
plans/<feature>.md
  │  to-seams         (only if >1 repo; halts for approval)
  ▼
plain/.claude/seams/*                        ← private, uncommitted
  │  to-tickets
  ▼
per-repo manifest TOML          +   app task table
  .git/treepad/batches/*.toml       (ticket_url, repo, blocked_by[], seams[])
  │
  │  app tick, every ~15s, per repo:
  ▼
git fetch origin
tp batch sync --json      → worktrees, stacked bases, PR links, repair
gh pr list --json …statusCheckRollup   → PR state + CI state
  │
  ▼
launch agent per ready worktree (up to the global cap)
  │  agent runs /implement → runs the repo's tests → commits → exits
  ▼
pid dead? → commits since this run's baseline? → denylist check → push, open PR
  │
  ▼
required checks green → "review me"
only the cross-repo compat check red → "waiting on producer deploy"
anything else red → "needs you"
  │
  │  USER reviews and merges
  ▼
merge → seams marked landed; consumers flip to "waiting on producer deploy"
```

## 3 · The tick

One reconcile loop, level-triggered — it never branches on why it woke. Per repo, in order:

1. **`git -C <repo> fetch origin`.** The only `git fetch` anywhere in treepad is
   `restack.go:87`, which fetches a member's own branch; `git worktree add` uses the local
   ref (`internal/treepad/lifecycle/lifecycle.go:69`). Without this step every worktree is
   cut from whatever `main` was when the user last pulled.
2. **`tp batch sync --json`** — materialises worktrees, assigns stacked bases, links
   open-PR prefixes into a Stack, repairs branches whose upstream was rewritten, marks
   merged members removable. Never `--launch`. Safe to run against a live fleet only
   because of the treepad change in §10: `link` and `restack` skip any member whose
   worktree holds a live Activity file.
3. **`gh pr list --json number,headRefName,baseRefName,state,statusCheckRollup`** — one
   call per repo. treepad's own `PRList` does not select check status
   (`internal/gh/gh.go:51`), so this is the app's own call. `pr_stale` from the sync report
   is read explicitly; its zero value must not be taken to mean "no PR exists".
4. **Liveness.** For each `running` task, `kill(-pgid, 0)` plus a process-start-time
   identity check. Alive → running, whatever the log says. Dead → delete the run's Activity
   file, then classify from artifacts: commits **since that run's baseline SHA** → push;
   none → `failed`. Never from timing. Sleep and wake need no special case.
5. **Push.** Refuse outright if the diff touches `.github/`, a lockfile or `package.json`
   → `needs you`, nothing pushed. Otherwise push the branch and `gh pr create --fill`
   (`--draft` if the ticket consumes a seam whose producer has not merged) → `checking`.
   A push or create failure → `push failed`, which is a state with a verb, not a retry
   every 15 seconds forever.
6. **CI verdict.** Computed only from a rollup whose `head_oid` matches the tip just
   pushed, over the repo's **named required checks**. An empty, absent or stale rollup is
   never green.
   - all required green → `review me`
   - only the cross-repo compatibility check red, every other required check green →
     `waiting on producer deploy`
   - anything else red → `needs you`

   There is no retry. Revision 1 re-ran the agent with the failing log; that races the
   repo's own check retries, cancels the run it is reacting to by pushing, spends a shared
   capacity-limited resource, and can produce a worse branch presented as `review me`.
7. **Draft state**, re-derived — never latched. A consumer PR is un-drafted when its CI
   verdict is green and every seam it consumes has a merged producer.
8. **Merges and closures.** MERGED → `merged`; seams whose producer merged are marked
   landed and their consumers flip to `waiting on producer deploy`. CLOSED unmerged →
   `pr closed unmerged`. A `StackStale` report from `tp` → `stack-stale`.
9. **Seam drift.** Recompose each task's prompt and compare `hash(composed)` against the
   hash stored with its last run. Mismatch → the row is flagged `seam changed`. Nothing is
   copied anywhere, so there is no propagation step to build.
10. **Top up.** Launch agents into ready worktrees up to the global cap. Before launching a
    chain root whose worktree was materialised before its blockers merged, and which holds
    no commits and is not dirty: `tp remove`, `git fetch origin`, `tp new <branch> --base
    main`, then launch (§4).

Goroutines: one for the loop, one per running agent (waits on the process, records exit),
one for HTTP. Not five actors — five *sections* of one loop, which is treepad's own proven
`Reconcile` shape.

## 4 · Readiness

```
ready(task) =
  position > 0   →  parent branch has an OPEN or MERGED pull request
  position == 0  →  every blocker (if any) is MERGED
```

Chain descendants stack on unlanded work — that is what stacking means, and it is what
keeps the fleet from idling on review latency. The cost is accepted in §13.4.

**Fan-in is a chain root.** A ticket with more than one same-repo blocker is chain
position 0 with base `main`, gated on all blockers merged. `to-tickets` must never continue
a chain *through* a fan-in node: `Chain.Tickets` is strictly linear and `batch.Resolve`
sets base = previous member's branch (`batch/resolve.go:52`), so a fan-in node appended to
one blocker's chain would sit in a worktree that does not contain the other blocker's code.

**The stale-root problem, and the fix.** `ReadyToMaterialise` passes position 0
**unconditionally** (`batch/ready.go:15`), so `tp batch sync` creates a fan-in root's
worktree on tick 1 — hours or days before the app is willing to launch into it — and
nothing ever advances that base. Fetching before materialising does not help, because
materialisation already happened. So immediately before launching such a root, the app
removes and recreates the worktree off a freshly fetched `main`. This is legal because
invariant 4 forbids deleting a worktree **that has commits or is dirty**, and an unlaunched
root has neither. Revision 1's invariant 14 passed in letter while the agent wrote code
against a base missing its own dependency.

**Cross-repo dependencies never stack** — no shared git history, so there is no base to
stack onto. Producer and consumer of a seam run in parallel; the edge between them is a
PR-draft edge, not a scheduling edge (§6).

## 5 · State machine

```
waiting ──► running ──► checking ──► review me ──► merged
              │            │
              │            ├──► waiting on producer deploy ──► running
              │            └──► needs you
              ├──► failed ──► running (attempt 2) ──► needs you
              └──► push failed

any state ──► stack-stale          (tp reports divergence it will not repair)
any state ──► pr closed unmerged   (PR closed without merging)
```

| State | Means | Verbs |
|---|---|---|
| `waiting` | not ready, or capped | — |
| `running` | agent process alive | kill |
| `checking` | pushed, PR open, CI running or not yet reporting | kill, re-run |
| `review me` | PR open, every required check green | — |
| `waiting on producer deploy` | every required check green except the cross-repo compatibility one; a consumed seam is not live yet | re-run |
| `needs you` | a required check other than the compat one is red, or attempts exhausted, or a denylisted diff | re-run, kill |
| `failed` | dead run with no commits since its baseline | re-run |
| `push failed` | push or `gh pr create` failed | re-run |
| `stack-stale` | `tp` reports the worktree diverged in a way it will not auto-repair | re-run, remove |
| `pr closed unmerged` | someone closed the PR | re-run, remove |
| `merged` | PR merged | remove worktree |

Every state has an entry, an exit and at least one verb. `review me` and the CI verdict are
re-derived every tick, not latched. `checking` exits to `needs you` if no rollup matching
the pushed tip appears within a bounded wait.

**A wedged agent is not a state the app can compute.** Liveness is a pid check by design
(invariant 7), so an agent stuck inside a 40-minute tool call is indistinguishable from one
working. Revision 1's `needs you` claimed to cover "a wedged agent"; it cannot. Instead the
page shows each running row's elapsed time and pgid, and `kill` is one click. Judging when
elapsed time has become suspicious is the user's, not a timing rule's.

Verbs: `re-run` (relaunch in the same worktree, incremental on the existing branch, handed
the composed-spec diff when a seam changed), `kill` (`-pgid` SIGTERM → SIGKILL), `remove
worktree` (`tp remove --force` when the app holds MERGED PR state). Worktree path, pgid and
elapsed time render as plain text so a terminal session is a copy-paste away.

## 6 · The seam mechanism

A **seam** is a named integration point between repos. It exists *only* because cross-repo
work has no shared git history to read. Single-repo features need none, and seam count is a
free complexity signal.

Seams live in `plain/.claude/seams/` — private, uncommitted, workspace-level. They must not
be committed to team repos: that would impose one person's orchestration on colleagues.

A seam is **a file, pasted whole into a prompt**. No symbol addressing, no marker comments,
no parsers. **It has exactly three jobs and nothing verifies it.**

1. **Prompt context.** Prompts are composed at launch, never embedded:
   `/implement <ticket-url>` plus the current contents of every seam the ticket consumes.
   `hash(composed)` identifies what that run ran against. Because nothing is copied,
   amending a seam simply stops every dependent's hash matching — there is no propagation
   step to build. The app stores each run's composed prompt so a re-run can be handed a
   mechanically computed before/after diff.
2. **Draft gate.** A consumer PR opens as a draft and is un-drafted only when its CI
   verdict is green and every seam it consumes has a merged producer. This uses GitHub's
   own affordance, so the constraint is visible to teammates who might otherwise merge it.
3. **Retirement pointer.** Each seam declares `lands_at` — one or more paths in the
   producing repo. Once the producer merges, later consumers are pasted the real file from
   `main`, so the private seam file is never read again and cannot go stale.

A producer-side assertion was available and was declined: `services` does hold a single
committed SDL file (`packages/core-graphql/src/schema.ts`, a 15,569-line `gql` template
literal), so a seam could be a verbatim excerpt and a check could be a grep. It is not
built, because the seam mechanism's job here is coordination, not enforcement, and a
partial check on the producer side buys little when the consumer side cannot be checked at
all (below). Recorded as a deferral in §12, not as an oversight.

### What a cross-repo consumer actually experiences

This is the honest statement revision 1 got backwards, and it constrains the design more
than anything else in it.

- `support-app` has **no committed copy of the schema.** `schema.graphql` is untracked and
  absent from the working tree; `scripts/gen.sh` `rm -f`s and re-`curl`s it from a running
  server, and skips the whole GraphQL block when `CI` is set.
- `.github/workflows/graphql-prod-compat.yml` regenerates types from **production** and
  typechecks against them, and `.mergify.yml:20` makes it a **required** check.

So a consumer implementing an agreed-but-unlanded seam field fails a required check
**deterministically until the producer deploys** — not merges. There is nothing to patch
locally and no way to make it green early. Revision 1 asserted the opposite and built a
mitigation on it.

The app's answer is naming rather than mechanism. `waiting on producer deploy` is entered
only when the compat check is the *sole* red required check and Lint, Typecheck and Tests
are all green — which distinguishes "the seam is not live yet" from "this consumer is
broken" using check names `.mergify.yml` already enumerates. The state exits on the user's
`re-run` once the producer's change is live, which regenerates types against the deployed
schema.

Seam amendment is a user-invoked skill, not an automatic loop. Most PR comments are code
standards or implementation disagreements and do not touch the seam.

A stuck producer therefore leaves its consumers parked indefinitely, by design. The
escalation is that the producer's own row shows `needs you`.

## 7 · What "done enough to push" means

The app runs no tests. `/implement` already does — "run the full test suite once at the
end" (`~/.agents/skills/implement/SKILL.md:11`) — so local testing exists; the app simply
does not check the result. That is the deliberate position: CI is the correctness gate, and
the app's contribution is to read it precisely (§3.6) rather than to duplicate it.

Two things make an unconditional push unacceptable, and both are handled by a path
denylist rather than a test run:

- On `services`, a push drives roughly 72 jobs and an OIDC-federated AWS deploy.
- An agent editing `.github/`, a lockfile or `package.json` is changing the machinery that
  judges it.

So: **no diff touching `.github/`, a lockfile or `package.json` is ever pushed.** That row
goes `needs you` with the offending paths listed. It is about twenty lines and it blocks
the only push whose consequences are not confined to a branch.

There is no verify command and no per-repo verify config. `quarantined` does not exist and
`to-tickets` needs no `files` field.

## 8 · Config and on-disk layout

```
ai-development/
  treepad/                            two small changes (§10)
  command-centre/
    cmd/cc/main.go                    flock → SQLite → ticker + HTTP
    store.go  loop.go  runner.go  http.go

plain/.claude/command-centre.toml     app config
plain/.claude/command-centre.db       SQLite; flock target
plain/.claude/seams/                  seam files
plain/.claude/runs/<run-id>.jsonl     agent stdout, one file per run
```

```toml
max_agents = 4
port       = 7777

[[repo]]
name            = "services"
path            = "services"
required_checks = ["Lint", "Typecheck", "Tests", "Generated files"]

[[repo]]
name            = "support-app"
path            = "support-app"
required_checks = ["Lint", "Typecheck", "Tests", "Generated files"]
compat_check    = "GraphQL production compatibility"
```

`required_checks` and `compat_check` are the only per-repo values, and both come straight
out of `.mergify.yml`. Without them a CI verdict is a guess: a bare rollup conflates
required with advisory checks and reads empty as green.

Agent stdout is **redirected to a file**, never piped. An app crash leaves the fleet alive;
a restart re-reads pids from SQLite, re-checks liveness with `kill(-pgid, 0)`, deletes
Activity files whose process is gone, and re-tails the files. Spawn with `Setpgid`; cancel
with a custom `Cancel` signalling `-pgid` SIGTERM then SIGKILL, because
`exec.CommandContext`'s default signals one pid and `claude -p` spawns tool subprocesses
that would otherwise orphan holding the worktree.

Truth is DB ∪ worktrees ∪ `gh pr list`, re-derived each tick. `tasks.status` is the
scheduling truth; `events` is an append-only audit table. No event-sourcing projection.

Intake is the manifest directory plus the task table — both already exist, and `batch.Load`
already reads the former.

## 9 · Invariants

Numbered so each can become a test.

1. Nothing is pushed until a run is dead **and** has commits made since that run's recorded
   baseline SHA.
2. No diff touching `.github/`, a lockfile or `package.json` is ever pushed.
3. The app never merges a pull request.
4. The app never deletes a worktree that has commits or is dirty.
5. The app never calls `tp batch sync --launch`, nor `gh stack
   init/add/submit/modify/rebase/sync`.
6. A run's Activity file exists exactly while its process is alive: written at spawn,
   deleted when the process is found dead, and reconciled against live pids at startup.
7. Liveness is `kill(-pgid, 0)` plus a start-time identity check. No timing rule ever marks
   a run dead.
8. A dead run's disposition comes from artifacts, never from the absence of events.
9. `link` and `restack` never touch a worktree with a live Activity file (enforced in
   treepad — §10).
10. One app instance per workspace, enforced by flock on the DB path.
11. A CI verdict is computed only from a rollup whose `head_oid` matches the pushed tip, and
    only over that repo's configured required checks. An empty or absent rollup is never
    green.
12. `waiting on producer deploy` is entered only when the configured compat check is the
    sole red required check and every other required check is green.
13. A consumer pull request is un-drafted only when its CI verdict is green and every seam
    it consumes has a merged producer.
14. Draft state, CI verdict and readiness are re-derived every tick, never latched.
15. At most two attempts per task, counted per task, before `needs you`.
16. A chain descendant launches only when its parent branch has an OPEN or MERGED pull
    request. A chain root with blockers launches only when every blocker is MERGED and its
    worktree was cut from a base fetched after the last of them merged.
17. No run inherits `ANTHROPIC_API_KEY`; no run uses `--bare`.
18. The HTTP surface binds `127.0.0.1` and rejects any request whose `Origin` or `Host`
    does not match, because `kill` and `remove worktree` sit behind it on a machine that
    also runs a browser.

## 10 · Required upstream changes

**treepad — two, both small.** Revision 1 said "none"; that was wrong.

1. **`link` and `restack` skip members whose worktree holds a live Activity file.**
   `Reconcile` runs both unconditionally on every tick with no liveness input
   (`internal/treepad/batch.go:189-190`), and `RestackFastForward` fires on a *clean*
   worktree — the normal state of an agent that has committed and is thinking. With the app
   ticking every 15 seconds, treepad would fast-forward under live agents and hand
   `gh stack link` branches with work in progress on them, which it pushes. The Activity
   file is already the fleet's busy marker (ADR 0003); `restack` simply does not read it.
   This also fixes the same hazard for a hand-run `tp batch sync` and for `tp ui`.
2. **Expose `tp remove --force`.** The plumbing exists — `lifecycle.go:243` builds
   `git worktree remove --force` and `git branch -D` — but no CLI flag reaches it, so
   `tp remove` uses `git branch -d` and refuses a squash-merged branch. `.mergify.yml:85`
   sets `merge_method: squash`, so that is every merged ticket. The app passes `--force`
   only when it holds MERGED PR state for the branch, making the evidence a merged pull
   request rather than git ancestry — the distinction `tp prune` gets wrong (§13.7).

**Skills** — contracts specified here, written separately:

| Skill | Change |
|---|---|
| `to-plan` | Repoint the dead `design-an-interface` branch (that skill exists in neither `~/.claude/skills/` nor `~/.agents/skills/`) at `to-seams`. Recommend and prompt when a plan touches >1 repo; do not gate. |
| `to-seams` | **New.** Writes `plain/.claude/seams/<name>`; declares producer repo, consumer repos, and `lands_at`. Halts for approval — the seam files are the one artifact worth real attention. |
| `to-tickets` | Emit per ticket: `repo`, `blocked_by[]`, `seams[]` into the app's task table. **A fan-in ticket must be a chain root, never mid-chain.** |

## 11 · Verified treepad behaviour this design relies on

- `tp batch sync --json` encodes the full report including `WorktreePath`
  (`internal/treepad/batch.go:497`), and has no flag to select steps: materialise → launch
  (opt-in) → link → restack → retire always run together (`docs/commands.md:70-76`).
- `retire` is flag-only and non-destructive (`internal/treepad/batch.go:384`).
- `LinkArgs` takes the longest prefix of OPEN-PR branches and breaks at the first miss
  (`batch/link.go:18`), so it cannot open a PR for a branch that has none. ADR 0003 calls
  this filter load-bearing.
- `ReadyToMaterialise` accepts OPEN **or** MERGED and passes position 0 unconditionally
  (`batch/ready.go:14-22`).
- `batch.Resolve` sets each member's base to the previous member's branch
  (`batch/resolve.go:52`).
- The only `git fetch` in treepad is `restack.go:87`, for a member's own branch.
- `tp new` accepts `--base` (`docs/commands.md`); `tp remove` currently takes no flags.
- `MergedBranches` uses `git for-each-ref --merged` (`internal/worktree/worktree.go:130`).

## 12 · Deferred, with re-entry triggers

| Deferred | Re-enter when |
|---|---|
| Local verify (a real pre-push test gate) | you find work reaching `review me` that `/implement`'s own suite should have caught. `.treepad.toml` is gitignored, so it is a ready-made private per-repo home for a `[verify]` key the app reads directly |
| Producer-side seam assertion (grep the seam against `lands_at`) | producer drift wastes consumer work more than once. `services`' single committed SDL file makes this ~10 lines |
| CI retry with the failing log | you click `re-run` on CI-red rows often enough to want it automated — and then only gated on a *terminal* failure: no pending re-dispatch, a conclusion outside {CANCELLED, TIMED_OUT, STARTUP_FAILURE, STALE}, a capped log delivered as a file, and a refusal to auto-push a denylisted diff |
| Detecting producer deploys automatically | `waiting on producer deploy` rows sit unnoticed. The mechanism is a `curl` of the deployed schema, at the cost of a production dependency in the tick |
| Scope containment / `quarantined` | you reject PRs for drive-by edits often enough to notice; then `to-tickets` gains a `files` field and the tick gains one diff check |
| Automatic repair of descendants after a merge | `stack-stale` rows become routine. Note `restack` has no moved-base predicate at all (§13.5), so this is new logic, not a flag |
| Adaptive rate-limit governor | you hit rate limits often enough to notice. Until then `max_agents` is the knob |
| `POST /tasks` | you want to add a ticket without re-running `to-tickets` |
| Slack / OTel / Datadog egress | a localhost page is not enough. Silence detection is a tick-age field on the page |
| MCP permission-prompt loop | denied permissions become a common failure mode; today they surface as `failed` → `needs you` |
| TUI | the HTTP page is demonstrably the wrong shape |
| Chain depth cap or warning | rebase churn from deep chains shows up in practice. No threshold is asserted; the page shows depth as information |
| Parent decision-trace into descendant prompts | descendants re-litigate decisions their parent already made (`docs/approach.md` Tension 4) |
| Multi-machine | never — different product, different auth |

## 13 · Known limits accepted

1. **Nothing anywhere validates a consumer against a seam.** Cross-repo consumers are red
   until the producer deploys, and the app's only response is to name the state correctly.
2. **The app cannot tell a wedged agent from a working one** (§5). The page shows elapsed
   time; the judgement is the user's.
3. **`.env` is copied wholesale into every worktree.** treepad does the copying via
   `[sync] include`, which is an allowlist in a config file the app does not own
   (`internal/sync/sync.go`), so there is no denylist for the app to apply. `.env` is not
   gitignored in `support-app`. Revision 1 claimed a denylist mechanism that does not
   exist.
4. **Stacking on open PRs means a red parent strands its descendants.** A fix pushed to the
   parent leaves descendants on a stale base. Deliberate throughput trade.
5. **`restack` has no moved-base predicate.** It compares a branch against its own upstream
   only, so the condition in the previous item produces no `stack-stale` signal — the
   signal fires for post-merge server-side rewrites, not for a base that moved. Revision 1
   relied on a mitigation that does not trigger.
6. **Squash-merge makes descendants non-patch-equivalent**, so where a signal does fire it
   lands in `stack-stale` and waits for a human rather than auto-repairing.
7. **`tp prune` is unusable for this workflow** and the app does not call it: it iterates
   every worktree in the repo with no batch or branch filter
   (`internal/treepad/lifecycle/lifecycle.go:326`), and its merge evidence is ancestry, so
   a squash-merged branch is never a candidate. Recorded as a treepad issue; the app uses
   `tp remove --force` with PR-state evidence instead.
8. **`gh stack link` is additive only** — a linked chain can never be reordered or unlinked
   by any treepad command (ADR 0003) — and it pushes every branch it is given. Safety rests
   entirely on `LinkArgs`' open-PR prefix plus the Activity-file veto.
9. **Mergify's merge queue batches up to four PRs** (`.mergify.yml`). Its interaction with
   stacked PRs is unexamined by this design.
10. **Ticket precision is inside the correctness surface** and nothing checks it.
11. **Rate limits, not money, are the wall** on a Claude subscription, and `max_agents` is
    the only control.

## 14 · Corrected from revision 1

Four premises in revision 1 were false. Each is stated here because the conclusions drawn
from them were reasonable *given* the premise, and a future reader should not re-derive
them.

| Revision 1 claimed | Actually | What it had been used to justify |
|---|---|---|
| `.treepad.toml` is committed in both repos | Globally ignored (`~/.gitignore-plain:1`); untracked in both. It appears in `[sync] include` *because* git will not carry it | Rejecting a `[verify]` key in treepad config; part of the case for no local gate |
| `services` has no single schema file — the schema is code-first across ~100 module dirs | One committed file, `packages/core-graphql/src/schema.ts`, a 15,569-line `gql` template literal | Relaxing "a seam is a file, verbatim"; deleting the producer-side `lands_at` assertion |
| A `support-app` consumer can patch its own committed schema copy and go green pre-merge | `schema.graphql` is untracked *and absent*; `gen.sh` deletes and re-curls it; `graphql-prod-compat` is a required check that regenerates from **production** | The entire consumer mitigation, and invariant 13's "merged producer" formulation |
| The app runs no tests, therefore no tests run locally | `/implement` runs the full suite at the end (`SKILL.md:11`) | Framing "CI is the only gate" as removing local testing rather than leaving it unchecked |

Design changes beyond those corrections:

| Revision 1 | Now | Why |
|---|---|---|
| `tp batch sync --json` is safe to call every tick | Requires the treepad Activity-file veto first | `link` and `restack` run unconditionally with no liveness input; `RestackFastForward` fires on the clean worktree of a thinking agent, and `link` pushes what it is given |
| Descendant gate: parent has an OPEN PR | OPEN **or** MERGED | a parent merging before its descendant got a slot stranded it permanently in `waiting`, on the happy path |
| CI red → re-run the agent with the failing log | CI red → `needs you` | the retry races the repo's own check retries, cancels the run it reads by pushing, spends shared CI capacity, and can present a worse branch as `review me` |
| Any dead run with commits is pushed | plus a `.github/`/lockfile/`package.json` denylist | a push drives ~72 jobs and an AWS deploy; an agent editing CI config edits its own judge |
| "commits present" | commits since the run's baseline SHA | attempt 1's commits made attempt 2 look successful |
| CI green = the rollup is green | rollup `head_oid` must match the pushed tip, over configured required checks | an empty or stale rollup read as green and un-drafted untested work |
| Nine states | thirteen, adding `stack-stale`, `pr closed unmerged`, `push failed`, `waiting on producer deploy` | each was a reachable condition with no state, no exit and no verb |
| `needs you` covers "a wedged agent" | it does not; the page shows elapsed time and `kill` | pid-only liveness cannot compute wedged, so the state named something undecidable |
| Invariant: never deletes a worktree | never deletes a worktree **that has commits or is dirty** | recreating an unlaunched fan-in root off fresh `main` is the fix for the stale-base bug, and the invariant was protecting work that is not there |
| Invariant: `.env` through a denylist | dropped; recorded as an accepted limit | treepad's `[sync] include` is an allowlist the app does not own |
| Fan-in needs no treepad change | still no schema change, but needs the recreate step | position 0 is materialised unconditionally on tick 1 and never advanced |

Unchanged and re-affirmed under review: the layer split, the single level-triggered loop,
pid liveness with no timing rule, artifact-first classification, file-redirected agent
output, `tasks.status` as truth with `events` as audit, never merging, stacking retained
with mechanics delegated to `tp`, and seams as prompt context only.
