# Approach

_Last updated: 2026-08-19_

## Background Research

Synthesis of ten sources, applied to the **Command Centre** design in
`.claude/handoffs/treepad__command-centre-architecture.md`. Written to be handed to a
staff-level reviewer alongside that document.

### Use-case frame

We are building a single-machine Go control plane that takes a dependency-ordered batch of
tickets and drives them to finished stacked pull requests: one `claude -p` agent per ticket in
its own `treepad` worktree, every result gated behind mechanical verification, and descendants
re-verified whenever a parent lands and rewrites their base. `treepad` supplies worktree
lifecycle and the pure scheduling predicates; the app owns all execution. The question this
research answers is **what the established literature says a system of this shape should look
like**, so the reviewer can judge the design against prior art rather than intuition.

### Why the Kubernetes sources were on the list

They were included for one property, not for distribution: **level-triggered reconciliation**.
A controller is triggered by an event but must never *consume* the event — it re-reads current
state and drives toward desired state, so it is correct after missed events, restarts, and
partial failures. That is orthogonal to whether the system is local or clustered, and it is the
only widely battle-tested example of a loop that owns resources it did not create and repairs
them continuously.

It matters here because **the near-fatal bug in our own design is an edge-triggered mistake**:
"a run marked `running` with no event newer than the heartbeat timeout is a crashed agent" reads
a *transition* out of an absent event, so a laptop sleep is indistinguishable from mass crash.
A level-triggered version asks a different question — *is this process alive right now, and does
this worktree contain work?* — and is immune to the whole class. `tp doctor`, `tp sync` and
`tp prune` are already level-triggered reconcilers over filesystem drift, so this is a discipline
the codebase has, extended over run state. The reviewer does not need the Kubernetes sources
themselves; they need to check that the Supervisor never branches on why it woke up.

### The mental model

1. **Level-triggered reconciliation** — events say *when* to look, never *what to do*. Every
   tick re-derives the world. Consequence for us: no state transition may be inferred from the
   absence of an event.
2. **Event log as source of truth, state as projection** — append-only history; current state is
   a fold over it. Buys crash recovery and after-the-fact answers. But see the crash window in
   *Watch out*: the log is not the only truth on disk.
3. **Idempotency by construction** — replay is normal, not exceptional. Sources achieve it with
   unique constraints and step-completion records, not with careful sequencing.
4. **Supervision, not repair** — a supervisor restarts and escalates; it never rescues a failing
   worker, and it never repairs state. Our "Supervisor never deletes" rule is this principle
   arrived at independently.
5. **Restart intensity** — a supervisor tolerates N failures per window; exceed it and the
   supervisor itself escalates rather than looping forever. **We have no equivalent policy.**
6. **Error kernel** — stable, critical things near the root; volatile things at the leaves. State
   and scheduler are the kernel; agent processes are leaves and are expected to die.
7. **Process group as the unit of termination** — an agent is a *tree* of processes, so kill the
   group (`Setpgid` at spawn, signal the negative PID), never the PID.
8. **Conflict as package/scope overlap, not git conflict** — SubmitQueue decides two changes are
   independent when they touch disjoint build targets. All changes it handles already merge
   cleanly; textual mergeability is not the question.
9. **Speculation and invalidation** — validating a change against a predicted base is sound iff
   *every* path in the speculation tree agrees. This is the formal version of our "`verified` is
   perishable" problem.
10. **Single writer** — one actor mutates a given artifact; extra agents may add intelligence,
    never concurrent actions. Our one-agent-per-branch rule complies; the stack dimension may not
    (see *Tensions*).

### Consensus view

The settled positions across these sources, all of which our design should be measured against:

- **Durability is a schema problem, not a framework problem.** Both durable-execution sources
  converge on the same minimal shape: an instance/flow row, an append-only event or step log
  keyed `(instance, step)`, status and attempt counters, and serialised inputs/results. Recovery
  is "replay the log, skip completed steps". No orchestration framework is required at our scale.
- **Persist the input before doing the work, and acknowledge only after the transaction commits.**
  This single ordering rule is what makes at-least-once processing survivable.
- **Idempotency comes from constraints.** A uniqueness violation on replay is normal operation to
  be swallowed, not an error to be reported.
- **SQLite is a correct choice for a single-writer local control plane**, with mechanics that are
  not optional: WAL journal mode, a `busy_timeout`, and a connection pool capped at one writer.
  Both sources name the single-writer limit as the reason it works *and* the reason it does not
  scale past one process.
- **Restarting works only for transient failures.** Deterministic failures are unaffected by
  restart, so a supervision strategy must distinguish them or it becomes an infinite loop that
  masks a broken product.
- **Killing a child does not kill its descendants.** Set the process group at spawn; signal the
  group; treat "no such process" as success. `exec.CommandContext`'s default behaviour is
  insufficient on both counts — it signals one PID and it pre-empts the child's own cleanup.
- **Stacks must merge bottom-up, and every merge below forces a rebase above.** This is inherent,
  not a tooling defect. Tooling automates ~80% of it; conflicts still surface to a human.
- **Review, not generation, is the bottleneck** — the reason stacks exist, and the reason a fleet
  that generates faster does not automatically ship faster.
- **Gating agent output requires layered constraints, not one check.** Feedforward constraints
  (rules, types, linters) shrink the solution space before generation; corrective feedback lets
  the agent self-repair; enforcement gates block the merge. A gate alone is the most expensive
  and least effective of the three.
- **Measure churn, verification cost and escape rate, not volume.** Lines or PRs produced measure
  throughput, not reliability.

### Tensions and trade-offs

**1. Serialise or speculate — the readiness gate.** Our design gates a task on its dependencies
being `merged`. SubmitQueue shows the alternative: validate against a *predicted* base and land
out of order, provided all speculation paths agree. Their 74% p95 wait-time improvement came
purely from queueing logic with no build-time change, which is direct evidence that scheduler
policy is where the throughput is. The cost is combinatorial verification runs and real
complexity. `merged` is the correct v1; the reviewer should confirm we have not accidentally
foreclosed speculation, and that we know what it would cost.

**2. Restart or escalate.** Erlang's supervision economics rest on production bugs being
overwhelmingly transient — one repeatable bug in 132 observed. **Agent failures are probably the
opposite distribution:** a vague ticket, a missing dependency or an unreachable acceptance
criterion fails identically on every attempt. If so, automatic relaunch is close to worthless and
the correct policy is a low restart intensity plus fast escalation to `awaiting_human`. This is
the sharpest unresolved question in the design and no source settles it for our domain.

**3. Event log as truth vs. git as truth.** The durable-execution literature treats the log as
authoritative, and one source names the exact hole: a crash *after* a step executes but *before*
its result is recorded replays that step. Translated: an agent that has committed real work and
died before its completion event was written looks unstarted, so a naive scheduler relaunches an
agent into a worktree that already contains the work. Our system has a second, stronger durable
record the sources do not — **the git worktree** — which argues for treating commits and pushed
branches as completion evidence and the event log as an index over them.

**4. Single writer vs. the point of the product.** Cognition's argument is that parallel agents
fail not from write conflicts but from **conflicting implicit decisions** made without visibility
into each other's traces. We comply on a single branch. We do **not** obviously comply along a
chain: a descendant agent inherits the parent's *code* but never the parent's *trace* — its
rejected approaches, its assumptions, its reasons. Cognition's first principle is "share full
agent traces, not just individual messages." **We hold those traces in the event log and
currently plan to use them only as telemetry.** Feeding a compressed parent decision-summary into
each descendant's context is the highest-value unbuilt idea surfaced by this reading, and it
turns the event log from an observability feature into a correctness feature.

**5. Prevention vs. detection for scope containment.** Our verifier *detects* out-of-scope edits
after the fact. The harness-engineering position is that feedforward constraints are cheaper and
more effective than gates. Both are available; the reviewer should say whether detection-only is
the right v1.

**6. Depth.** Practitioners report stacks of 5–20, but typical examples are 3, and the explicit
advice for agent-driven stacking is to start at two layers before going deeper. Our "shallow and
wide beats deep and narrow" guidance matches, and is currently unenforced by anything.

### Practical patterns worth adopting

- **Schema:** `tasks` (declared work, scope, dependencies) · `runs` (one attempt, with attempt
  counter and status) · `events` (append-only, keyed `(run_id, seq)`, holding the parsed
  `stream-json`). Store the run's *inputs* before spawning. One transaction per handled event.
  Unique constraints on `(task_id, attempt)` and on any externally-visible action.
- **SQLite:** `?journal_mode=WAL&busy_timeout=…`, `db.SetMaxOpenConns(1)`, claims via
  `BEGIN IMMEDIATE`. Test recovery deliberately with chaos middleware that fails handlers and
  cancels contexts mid-flight — cheap, and the only way this code path is ever exercised.
- **Termination:** `SysProcAttr{Setpgid: true}` at spawn; on cancel, `syscall.Kill(-pgid, SIGTERM)`
  then `SIGKILL` after a grace period; treat `ESRCH` as done; `cmd.Wait()` in its own goroutine
  and race it against the context so cleanup and output flush actually happen.
- **Supervision policy:** per-task restart intensity (e.g. 2 attempts per hour) → escalate to
  `awaiting_human` rather than looping. Reap (kill + mark) and reclaim (delete tree) stay separate.
- **Reconcile:** derive everything from current state each tick — process liveness, worktree
  contents, branch ahead/behind, PR state. Never branch on which event woke the loop. Compare
  elapsed wall-clock against the requested interval and treat a jump as "look again next tick",
  not as evidence.
- **Conflict model:** reuse the ticket's declared file scope as the conflict oracle (SubmitQueue's
  disjoint-package test). The same declaration then serves three jobs: scheduling independence,
  the verifier's containment check, and the plan-alignment check.
- **Agent-side rules for stacking:** each PR body describes only that layer's net diff; agents are
  forbidden from mutating stack structure (no rebase, no base edits, no force-push of siblings);
  agents must push and open a PR, since that is the signal the stack machinery consumes.
- **Harness layers before gates:** scope rules in `CLAUDE.md`/`AGENTS.md` (feedforward), lint
  messages that carry the remediation and cannot be inline-disabled (corrective), then the verifier
  (enforcement).

### What to watch out for

- **The completion crash window** (see Tension 3) — the single most likely source of duplicated or
  lost agent work.
- **`exec.CommandContext`'s default kill** signals one process and pre-empts cleanup. `claude -p`
  spawns tool subprocesses; without the group, they orphan to init and keep holding the worktree.
- **Unbounded relaunch.** With no restart intensity, one badly-specified ticket burns the fleet's
  entire rate-limit window.
- **Squash merge.** Stacked-PR support for squash merge is explicitly unconfirmed in the tooling,
  and squash is what makes descendants diverge non-patch-equivalently — i.e. what pushes them into
  `stack-stale` and onto a human. Verify this against the real `gh stack` before assuming
  auto-restack is hands-off.
- **`gh stack link` is additive only** — a Chain can be built but never reordered or unlinked.
- **Scope declarations are load-bearing.** If tickets do not name their files precisely, the
  conflict oracle, the containment gate and the plan-alignment check all degrade to theatre
  simultaneously.
- **Metrics trap:** tickets-completed-per-day will look excellent while churn and review cost
  quietly absorb the gain.

### Gaps in this reading list

- **Nothing addresses suspend/resume detection.** The laptop-sleep failure mode is ours to solve;
  the Kubernetes discipline gives the shape of the fix but no source discusses it directly.
- **Nothing treats rate limits as a scheduling input.** Every source assumes compute is available
  when the scheduler asks. Our global concurrency governor reading `api_retry` events has no prior
  art here.
- **Nothing covers fan-in against strictly linear stacks.** SubmitQueue assumes independent changes
  landing on one trunk; stacked-diff sources assume a single author's linear stack. A DAG whose
  nodes are stack layers is unaddressed.
- **No empirical data on the transient-vs-deterministic split for agent failures**, which is
  precisely what Tension 2 needs to be settled properly. Our own event log is the instrument that
  would answer it — worth designing for that from day one.
- **No source covers verification of work whose base is rewritten mid-flight.** SubmitQueue is the
  closest analogue and it cancels-and-revalidates rather than repairing.

### Recommended next steps

1. **Settle the readiness gate** — `merged` for v1, with SubmitQueue's all-paths-agree rule written
   down as the correctness bar any future speculation must clear.
2. **Add a restart-intensity policy and an escalation path** before writing the Supervisor, and
   instrument failures well enough to learn the transient/deterministic split empirically.
3. **Make git the completion evidence and the event log the index** — closes the crash window and
   makes "relaunch" safe by construction.
4. **Decide whether descendant agents receive the parent's compressed trace.** This is the one
   architectural question this reading raised that the design does not currently answer.
5. **Build the feedforward harness first** (scope rules, remediating lint) so the verifier is the
   last line of defence rather than the only one.

<!-- Alignment content (Problem, Goals, Features, etc.) to be added via chat-to-approach -->
