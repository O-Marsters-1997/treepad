# Architecture Review — Command Centre (design-stage)

_Read-only assessment. Date: 2026-08-19. Scope: the design document
`.claude/handoffs/treepad__command-centre-architecture.md` (unbuilt L3 app), verified
against the real treepad code it depends on (`batch/`, `internal/treepad/`,
`internal/launcher/`, `internal/gh/`, ADR 0003) and read alongside
`docs/approach.md` §Background Research. No source files were modified. Because L3 does
not exist, design-doc findings cite the doc's numbered invariants, decisions, and quoted
claims; treepad findings cite file:line. Each finding states whether the design is
**wrong** or **underspecified**._

## Verdict

The layer model is genuinely good — the per-layer prohibitions, the division-of-labour
table, and the refusal to merge are the right skeleton, and the treepad code really does
enforce the boundaries the doc claims (the `batch/` import guard and the `internal/`
launcher note are real). But the design as written is **not internally coherent and should
not be implemented as-is**, for one central reason and two structural ones. Central: the
`merged` readiness gate and the stacked-PR machinery are mutually exclusive — under
`merged` gating no two PRs in a chain are ever open together, so `gh stack link`, the
restack path, and the perishable-`verified` cycle are dead code, while `batch.Resolve`'s
stacked base assignment becomes actively wrong (children branch off stale, already-merged
parent branches). The document describes two different systems smeared together and pays
for both. Structural: (1) the Runner/Supervisor process model (stdout pipe as heartbeat,
stale-stream ⇒ crashed) couples every agent's life to the app's life and re-commits the
edge-triggered mistake the author's own research document names as near-fatal and
prescribes the fix for; (2) `batch/` exports treepad's *predicates* but not its
*mechanisms*, so the app as designed re-implements the dangerous half (git fact-gathering,
`gh` calls, path derivation) from scratch. All three are fixable on paper, cheaply, before
any code exists — which is exactly why this review should block implementation until they
are.

## Executive summary

**Risk 1 — the design contradicts itself on its core scheduling rule.** "Ready = deps
`merged`" (L3.2, Decision table row 4) collapses each Chain into a strictly serial
pipeline in which stacking never occurs, yet Parts 2–5 are largely machinery for stacking
(LinkArgs, RestackDecision, `verified → rebasing → verified`, invariants 1–2, 11–12).
Either gate on `verified` and keep the stack machinery, or gate on `merged` and delete it
— the current text funds both and delivers neither.

**Risk 2 — the process model does not survive the failure it is designed for.** The app
parses each agent's stdout pipe as event log *and* heartbeat (L3.3). If the app crashes,
every agent's next write hits a broken pipe (SIGPIPE/EPIPE) — the app crash kills or
silences the fleet, defeating "survives crashes… without losing work" (What it is, item 6).
And "no event newer than the heartbeat timeout ⇒ crashed" reaps healthy agents that are
quietly inside a long tool call. `docs/approach.md` diagnoses this exact class ("the
near-fatal bug in our own design is an edge-triggered mistake") and prescribes the fix
(direct process liveness + level-triggered reconciliation); the design doc kept the broken
rule and patched it with sleep-jump detection (invariant 8) instead.

**Risk 3 — the library boundary is real but half-deep.** `batch/` is verified pure and
import-guarded, but it exports decision functions whose *inputs* (dirty, ahead/behind,
patch-equivalence via `git cherry`, PR lists) are computed only in `internal/`. The app
must re-implement `internal/treepad/restack.go` and `internal/gh/gh.go` wholesale, and
worktree-path derivation would exist in a third place (it is already duplicated once, by
treepad's own admission at `internal/treepad/batch.go:464-469`).

**Genuine strengths, material to the verdict:** the five-layer prohibition model; the
reap/reclaim split (L3.5); never merging (invariant 4); `quarantined` as distinct from
`failed`; per-task auth as a field (Decision table); the honest single-machine scope. These
survive review and should be kept through any redesign.

## System map

*(as proposed by the design doc; L1/L2/L4 verified against real code)*

- **Entry points** — `POST /tasks` on the L3 HTTP surface; Manifests written by
  `to-tickets` to `<git-common-dir>/treepad/batches/*.toml` (`batch.Load`,
  `batch/manifest.go:64`); a reconcile ticker inside the app.
- **Modules** — L0 tracker + ticket-cutting skills (declares the DAG); L1 `batch/` (pure
  predicates: `Resolve`, `ReadyToMaterialise`, `LinkArgs`, `RestackDecision`); L2 `tp`
  CLI/TUI (human worktree lifecycle; own orchestration path `Reconcile`,
  `internal/treepad/batch.go:152`, which the app must bypass); L3 Command Centre — State
  (SQLite, event log + projection), Scheduler (topological ready-set, cap), Runner (one
  `claude -p` per worktree, `Setpgid`, stream-json → event log), Verifier
  (build/test/lint + scope diff), Supervisor (reap / repair / link / govern / top-up),
  Surface (HTTP + Slack + OTel/Datadog + MCP permission server); L4 GitHub (`gh stack
  link` only; humans merge).
- **Boundaries & data flow** — Manifest → `batch.Resolve` gives (ticket, branch, base) →
  scheduler claims (`BEGIN IMMEDIATE`) → Runner shells `tp new <branch>` → agent runs →
  verifier gates → supervisor links open-PR prefixes → human merges → supervisor rebases
  descendants in-worktree and re-verifies.
- **State & persistence** — SQLite (`tasks`, `runs`, `events`), single writer,
  `SetMaxOpenConns(1)`; git worktrees hold the actual work; GitHub holds PRs/Stacks;
  treepad's own PR cache and Activity files exist beside it but are declared unused by L3.
- **External dependencies** — `claude` CLI (subscription auth, volatile policy), `gh` +
  `gh stack` (public preview, additive-only `link`), `tp` as subprocess, git.
- **Runtime shape** — one Go process on one developer machine; agents as detached child
  process groups; no server dependencies.

## Findings by dimension

All seven lenses were assessed directly by this reviewer (nested fan-out was not used; the
review target is a single 380-line document plus ~1.5 kLoC of already-verified Go, and the
lenses below share one evidence base). No measurement tooling applies to an unbuilt
system; treepad claims were verified by reading source, not estimated.

### Simplicity & understandability

**Verdict: the layer model is the clearest part of the design; the L3 interior carries
accidental complexity that a 12-task laptop tool does not need, and the one load-bearing
concept (readiness) has two contradictory definitions living in the same document.**

1. **Two incompatible operating modes, presented as one system.** *(Design is wrong, not
   underspecified.)* Trace the doc's own lifecycle (Part 4) under its own readiness rule
   (L3.2: "a task is ready when all its dependencies are `merged`"):
   - T1 (position 0) runs, verifies, opens PR. T2 is not ready — T1 is not merged.
   - A human merges T1. Only now does T2 claim, and the Runner creates its worktree "off
     the stack parent" — which `batch.Resolve` defines as *T1's branch*
     (`batch/resolve.go:31,52`: `base = branch` of the previous member). T1's branch is
     merged (with squash, its content is not even reachable from `main` as those commits)
     and possibly deleted. T2 is branched off a corpse.
   - At no point do two PRs of the chain coexist as OPEN. `batch.LinkArgs`
     (`batch/link.go:18`) returns nil below two open-PR members — **`gh stack link` never
     fires**. No descendant ever exists above an open PR, so "merging a lower PR rewrites
     every branch above it" never happens within a chain — **the repair path (Supervisor
     step 2, `RestackDecision`) never fires for its stated purpose**. The
     `verified → rebasing → verified` cycle survives only in the weaker form "main moved
     under a verified-but-unmerged task", which is ordinary merge-queue rebasing and needs
     none of the stack machinery.
   Part 3's table then forbids the only escape: "Branch/base derivation: L1
   `batch.Resolve` — L3 must not re-derive." Under `merged` gating that instruction
   produces wrong bases; the correct base for a post-merge child is `main`. The document
   is simultaneously a serial merge-queue design and a stacked-pipeline design, and every
   reader must hold both to understand any part. Direction: pick one (see Positions,
   Q-A) and delete roughly half of Part 2–5 accordingly.

2. **Event-sourcing ceremony at 12 rows.** *(Design costs more than it buys.)* "Current
   state is a projection over the log" (L3.1) plus invariant 7 ("state always derivable
   from the event log alone") imports replay/projection machinery for a task table that
   will hold ~a dozen rows. Worse, invariant 7 is false by the design's *own* stance:
   Part 8 / `approach.md` Tension 3 argue git commits and PRs are completion evidence the
   log can miss (crash window), so truth is log ∪ worktree ∪ GitHub, and the tick must
   reconcile from artifacts anyway. Once you reconcile from artifacts, the projection is
   redundant. Direction: `tasks.status` columns as the scheduling truth, an append-only
   `events` table as *audit*, and drop invariant 7 in favour of "state is re-derivable
   each tick from DB + worktrees + PR list" — which is also treepad's own proven shape
   (`Reconcile` derives everything per tick and stores almost nothing,
   `internal/treepad/batch.go:152`).

3. **Two logs conflated.** The agent's `stream-json` transcript (high-volume telemetry,
   MBs per run) and the orchestration event log (a handful of state transitions per task)
   are both "the event log" (L3.1 vs L3.3). A projection over a log dominated by tool-call
   noise is the wrong read model, and it drags the transcript into SQLite for no
   consumer. Direction: transcript → one file per run on disk (which is also what makes
   Risk 2's fix work); state transitions → SQLite. Note treepad already landed on exactly
   this split: the Activity file *is* a per-run transcript file
   (`internal/launcher/launcher.go:84-109`).

4. **Surface scope (L3.6) is a small platform bolted to a laptop tool.** HTTP view +
   Slack egress + OTel → Datadog + a Datadog silence monitor + an MCP
   permission-prompt server, for one user, v1. The doc itself concedes "an HTTP handler…
   is ~90% of the value". Direction: ship the 90%; the silence monitor is a `tickAge`
   field on the HTTP page, not a Datadog monitor.

### Maintainability

**Verdict: the design creates three sets of duplicated knowledge across L2/L3 and hangs a
correctness requirement on an untracked cross-repo contract. Each is cheap to fix now and
expensive after.**

1. **The restack *mechanism* will be written twice.** `batch.RestackDecision`
   (`batch/restack.go:25`) is pure — but its four inputs are produced by
   `internal/treepad/restack.go:42-110` (fetch, `worktree.Dirty`, `worktree.AheadBehind`
   with the `hasUpstream` subtlety, and `patchEquivalentToOrigin` at
   `internal/treepad/restack.go:111` — the `git cherry` "+"-line parse). None of that is
   exported. The app must re-implement all of it, and the subtle bugs live in the
   fact-gathering, not the predicate (e.g. skipping the `hasUpstream` check, or misreading
   `git cherry` output, silently turns `RestackStale` into `RestackReset` — a work-destroying
   difference). Same story for `gh`: `internal/gh/gh.go:38,49` (`StackLink`, `PRList`
   with its JSON field selection) are internal, so the app re-implements the `gh` surface
   ADR 0003 spent a page constraining. **This answers the brief's key question: `batch/`
   is a genuine but insufficient library surface — the predicates are exported, the
   dangerous mechanisms are still trapped in `internal/`.** Direction: before writing the
   app, promote a thin mechanism layer into `batch/` (or a new public package):
   `GatherRestackFacts(runner, path, branch)`, `PRList`, `StackLink`. The
   `batch.Runner` interface (`batch/manifest.go:41-44`) already exists precisely so
   exported functions can take a command runner — use it.

2. **Worktree-path derivation, third copy.** `internal/treepad/batch.go:467`
   (`worktreePathFor`) already "mirrors lifecycle.CreateWorktreeWithSync's path
   derivation" by its own comment — knowledge duplicated once inside treepad. The app,
   shelling `tp new` (L3.3), gets no machine-readable path back (`lifecycle/new.go`
   emits a `__TREEPAD_CD__` directive and human logs; there is no `--json`), so it either
   parses that directive or derives the path a third time. Direction: `tp new --json`
   printing `{worktree_path}` — one small L2 change that makes the subprocess seam clean —
   or promote path derivation into `batch/`.

3. **The `/implement` skill contract is load-bearing and untracked.** Part 4 requires
   "agent commits, pushes, opens PR"; the doc's own References admit the skill "currently
   ends at 'commit to the current branch' and does not push or open a PR." The whole PR
   pipeline rests on a behaviour change in a skill in a different repo, with no version
   pin and no ticket. (My position — see Positions, Q-B — is that the skill should *not*
   gain push rights; the Runner should push after verification. Either way, decide and
   record it; today the design depends on a contract nobody owns.)

4. **Two readiness rules held in two layers by design** (L3.2 divergence note) is
   defensible — but only if L1's predicate is parameterised rather than shadowed, else the
   next person to touch `ReadyToMaterialise` (`batch/ready.go:12`) has no way to know a
   second consumer deliberately disagrees with it. See Positions, Q-F.

### Extensibility

**Verdict: the design is extensible along the axes it names (auth policy, readiness
policy) and blocked along the axis it claims as its main advantage — the true DAG —
because no component is specified to supply the edges.**

1. **The fan-in story begs its own question.** *(Underspecified.)* Part 7.1: "the app can
   read the true DAG from its own task table and need not inherit the approximation." But
   the task table only holds what something wrote there, and nothing in the design writes
   dependency edges: the Manifest schema has none (`batch/manifest.go:27-38` — name,
   prefix, base, `chain.tickets` only; the dropped fan-in edge is "reported" by
   `to-tickets`, not recorded), `POST /tasks` is not specified to carry edges, and L3
   reading the Tracker is a new boundary the doc never grants. Direction: extend the
   Manifest schema with an optional `blocked_by` per ticket — `to-tickets` already holds
   the full graph at authoring time, the change is a few lines in L1, and it keeps the
   no-Tracker rule intact for every layer.

2. **Fan-in placement as designed produces wrong worktrees.** *(Design is wrong.)*
   Concrete sequence: T3 is blocked by T1 (its chain parent) and T2 (another chain).
   `to-tickets` appends T3 to T1's chain, so T3's base is T1's branch, which forked from
   `main` before T2 landed. Whatever the readiness gate:
   - under `verified` gating, T3 starts while T2's code exists only on T2's branch — T3's
     worktree simply *does not contain* the dependency it declared, so the agent either
     fails or reinvents T2;
   - under `merged` gating, T2 is in `main` but T3's base (T1's branch) still predates it
     — same absence, because a chain's position-0 fork point never advances until the
     chain itself merges (nothing in the design rebases an unmerged chain onto moving
     `main`; restack only follows server-side rewrites after merges *below in the same
     chain*, `internal/treepad/restack.go`).
   The only placement whose worktree provably contains all dependencies is: **a fan-in
   node starts a new chain (position 0, base `main`), gated on all its cross-chain deps
   being merged.** A chain must never continue *through* a fan-in node. That is a crisp
   rule the app can enforce and `to-tickets` should adopt; the current
   "append to the longest blocker's chain" is precisely the wrong one.

3. **No exit from `quarantined`.** The state machine (Part 4) has no transition out of
   `quarantined` — the state exists to "preserve work for a human decision", but no
   operation (re-scope, split, discard, promote) is specified, so the first quarantine
   strands a worktree and a branch forever. Underspecified; a `quarantined → pending
   (re-scoped)` and `quarantined → abandoned` pair is enough.

4. Genuinely good: auth as a per-task field (Decision table) against a policy that "has
   changed three times in four months", and the explicit deferral of multi-machine. These
   are the right open/closed choices.

### Security

**Verdict: acceptable only under an unstated assumption — loopback-only, single-trusted-
operator — that the design never states. Three concrete gaps, one of which (push before
verification) also undermines invariant 1.**

1. **Out-of-scope work is public before the gate sees it.** Part 4 has the agent push and
   open a PR *before* the Verifier runs. `quarantined` therefore does not contain
   anything: the drive-by edit is already on GitHub under the operator's identity, and
   `gh stack link` — which "pushes every branch it is given" (ADR 0003; quoted in Part 2
   L4) — can compound it. The containment gate gates a stable door. Direction: agents
   work and commit locally only; the Runner pushes and opens the PR *after* `verified`.
   This one ordering change makes invariant 1 true, makes quarantine real containment,
   and removes the need to give the `/implement` skill push behaviour at all.

2. **`POST /tasks` is an unauthenticated execute-arbitrary-agent endpoint.**
   *(Underspecified.)* A task is a prompt that will be run with `--permission-mode
   acceptEdits` and the operator's GitHub credentials. The doc specifies no bind address
   and no auth. On loopback it is fine; the doc must say so (invariant material:
   "the Surface binds to 127.0.0.1 only").

3. **`.env` fan-out vs invariant 10.** L3.3 copies `.env` wholesale into every worktree;
   invariant 10 forbids inheriting `ANTHROPIC_API_KEY`. A `.env` that contains that key
   (common) violates the invariant through the front door, and quarantined/failed
   worktrees retain secret copies indefinitely because the Supervisor never deletes
   (invariant 5 — right call, but it makes worktrees a secret-retention surface).
   Direction: copy `.env` through a denylist filter; state it in invariant 10.

4. **The Slack permission loop is an approval channel with no authorisation model.**
   L3.6: `--permission-prompt-tool` → MCP → Slack → blocked agent proceeds on reply.
   Whoever can post in that channel can approve a permission escalation for a process
   holding the operator's credentials. Fine for a personal DM; must be stated.

### Performance & scalability

**Verdict: the honest single-machine ceiling is fine; the two real problems are an
ungoverned re-verification storm and a cap that counts the wrong thing. (No measurements
possible — unbuilt system; treepad-side claims verified from source.)**

1. **The concurrency cap does not govern the Verifier.** Every merge at the bottom of a
   depth-D chain triggers up to D-1 rebases and D-1 *re-verifications* — each a full
   build+test+lint. Nothing in L3.2/L3.4/L3.5 subjects verifier runs to the cap, so a
   human merging three stacked PRs in a review session detonates O(D²) build jobs on the
   laptop that is also running the agents. Direction: verifier runs draw from the same
   (or a sibling) semaphore, and a rebase cancels any in-flight verification of the same
   task (verification results must be keyed by base SHA or they can be recorded against a
   base that no longer exists — the doc never says this; it is the difference between the
   `verified` cycle terminating cleanly and thrashing).

2. **Termination of `verified → rebasing → verified`** (author's challenge): it
   terminates, with a bound — a task re-verifies at most once per ancestor merge plus
   once per `main` movement *if* "base moved" includes `main` for position-0 tasks. The
   doc leaves that definition open, and the open reading is a livelock in any active
   repo: every unrelated push to `main` un-verifies every position-0 task. Direction:
   define "base" as the tracked stack parent only (position 0: the fork point recorded at
   claim time, not live `main`), and let staleness-vs-main be GitHub/CI's problem, as it
   is for every human PR.

3. **`SetMaxOpenConns(1)` serialises reads behind stream ingest.** WAL exists precisely
   so readers don't queue behind the writer; one pooled connection for everything gives
   up that property, so the HTTP surface stalls whenever event ingest is busy. Idiomatic
   shape: one dedicated writer `*sql.DB` (max 1 conn) + one read pool. Two lines, not a
   framework. (Moot for transcript volume if finding S3 lands and the transcript stays
   out of SQLite.)

4. **Invariant 9 counts tasks, not processes.** An `awaiting_human` agent is a live
   `claude` process holding memory and a session; freeing its "slot" means the fleet's
   real process count is cap + M. On rate limits (the stated wall) and a laptop's RAM,
   the resource is processes. Direction: cap OS processes; `awaiting_human` merely stops
   *counting toward scheduling priority*, or is explicitly suspended.

5. The rate-limit governor (Supervisor step 4) lowers the cap and never specifies raising
   it — a one-way ratchet ends the batch at concurrency 1. Underspecified; a decay/
   recovery rule is one line. (Honestly, v1 could ship with a manual cap knob and no
   governor at all.)

### Modularity (deep modules + coupling & cohesion)

**Verdict: the L1 boundary is real and verified — and exactly one layer too shallow for
this embedder. Inside L3, five components hide a simpler truth: this is one reconcile
loop.**

1. **Verified:** `batch/api_test.go:26` (`TestNoInternalImports`) does what the doc
   claims — parses imports of every non-test file, allowlists only `internal/slug`.
   `batch.PR` (`batch/pr.go`) exists so exported predicates can take PR state. The
   launcher's embedder note (`internal/launcher/launcher.go:1-5`) says what the doc says.
   The doc's characterisation of treepad is accurate; this discipline is rare and worth
   naming as a strength.

2. **But the module is deep for the CLI and shallow for an embedder.** A deep module
   hides the dangerous implementation behind a small interface; for the Command Centre,
   `batch/` hides only the *decisions* while the dangerous *doing* (Maintainability 1)
   leaks back out as "reimplement `internal/` yourself." The interface the app actually
   needs — facts + predicate + a couple of `gh` verbs — is nearly designed already; it
   just isn't exported. Fix in treepad first; do not start the app against the current
   surface.

3. **Scheduler vs Supervisor is one component wearing two names.** Scheduler "decides
   what starts next" (L3.2); Supervisor step 5 "tops up the fleet to the cap" (L3.5) —
   the same decision, owned twice, with the claim transaction in a third place (State).
   The natural Go shape — and treepad's own proven shape (`Reconcile`,
   `internal/treepad/batch.go:152`: one tick, fixed steps, pure predicates from L1) — is
   **one reconcile goroutine** that each tick: reads facts (DB, process liveness,
   worktrees, `gh pr list`), computes decisions via pure functions, executes side
   effects; plus one goroutine per running agent doing nothing but pumping that agent's
   output and exit status into the store. Channels between five stateful components would
   be un-Go-like ceremony here; the design flirts with it by naming five owners.
   Direction: keep the five *responsibilities* as sections of one loop + pure functions,
   not five concurrent actors.

4. **`BEGIN IMMEDIATE` claims defend against a second scheduler the design says cannot
   exist** — and would not save you if it did (two instances would still concurrently
   `gh stack link`, `git reset --hard`, and double-spawn agents; the claim protects one
   row, not the world). The actual requirement is one instance per repo. Direction: an
   exclusive lock (flock on the DB path) at startup; keep `BEGIN IMMEDIATE` merely as the
   correct SQLite write mode, and drop the two-schedulers rationale from L3.1/invariant 6.

5. **The L2/L3 coexistence hole.** *(Design is wrong by omission.)* The doc rules that
   the app must not use L2's orchestration path — but nothing stops the *human* from
   using it while the app runs. Concrete interleaving: the app launches an agent into
   `feat/x` (the app writes no Activity file — that is an L2-only concept per Part 2).
   The human, out of habit, runs `tp batch sync --launch` or presses `l` in `tp ui`.
   `readyToLaunch` (`internal/treepad/batch.go:276`) sees the branch materialised
   (`ActionSkipped`) and **no Activity file** → treepad launches a second agent into the
   same worktree. Two writers, one branch — the exact failure class the whole one-agent-
   per-worktree rule exists to prevent, reachable by muscle memory. Direction (pick one,
   cheapest first): the app writes the Activity file too (it is a path convention,
   `launcher.ActivityPath`, not a process dependency — using the guard is not "using the
   Launcher"); or `tp` learns a `managed` marker file that makes launch refuse; or accept
   an operational rule and print it, which history says fails.

### Deployability (operability on one laptop)

**Verdict: this is the weakest dimension. The stated crash/sleep story does not work as
designed; restart, reattach, and retry are unspecified; and the design rejects the one
mechanism (file-redirected output) that would make all three trivial — a mechanism its
own L2 already uses.**

1. **App crash ≠ fleet survival, as designed.** L3.3 pipes each agent's stdout into the
   app ("the stream is the heartbeat"). App dies → pipes close → each agent's next write
   gets EPIPE/SIGPIPE and the agent dies or wedges mid-ticket. The design's headline
   promise ("survives crashes, restarts and laptop sleep without losing work") is
   structurally unmet by its own Runner. Direction — and this fixes four findings at
   once: **agents write `stream-json` to a per-run file; the app tails the file.** Then
   app crash leaves agents running; restart = re-open files + re-check liveness;
   sleep/wake needs no special-casing; and the transcript stays out of SQLite (S3). This
   is treepad's Activity-file pattern (`internal/launcher/launcher.go:84-109`) with a
   structured payload — the design discarded the right mechanism along with the L2
   process model it was right to discard.

2. **The reap rule is the edge-triggered mistake, kept.** "Stale heartbeat ⇒ crashed"
   (L3.5 step 1) plus the wall-clock-jump patch (invariant 8) still fails two concrete
   sequences: (a) a healthy agent inside a silent 10-minute tool call (a build it
   spawned) emits no events past the timeout → killed mid-work, marked failed —
   false-positive with no clock jump at all; (b) laptop sleeps, the app is restarted
   after wake (or crashed during sleep) → the *new* process observes no interval jump,
   sees uniformly stale heartbeats, and reaps every live agent on its first tick —
   invariant 8 is unfalsifiable protection because the process holding the "requested
   interval" is gone. The level-triggered rule is simpler than the patched edge rule:
   each tick, `kill(-pgid, 0)` + start-time identity check (PID-reuse guard). Process
   alive → not crashed, whatever the log says. Process dead → classify by artifacts, not
   by absence of events (next finding). Reap-vs-reclaim (kept, correctly) then covers the
   residue. Invariant 8 shrinks to "a clock jump defers *classification*, not reaping" —
   or disappears.

3. **The completion crash window destroys classification, not work — as designed it
   still destroys the run.** Sequence: agent commits, pushes, exits 0; app crashes before
   the terminal event lands; restart finds run `running`, process dead → step 1 "mark the
   run failed" — unconditionally, per L3.5. The work sits complete in the worktree and as
   an open PR while the system reports failure, and any retry policy relaunches an agent
   into a worktree that already contains the finished work. Git-as-completion-evidence
   with the log as index (approach.md, Tension 3) **is** coherent — but only if the
   Supervisor actually implements it: on finding a dead run, inspect the worktree and PR
   *first*; commits present → hand to the Verifier (the gate that was always going to
   decide anyway), not to `failed`. The design gestures at this philosophy and then
   specifies the opposite behaviour. Fix the specification of step 1.
   (`exec.CommandContext` detail for later: its default `Cancel` signals one PID; with
   `Setpgid` you must install a custom cancel that signals `-pgid`, SIGTERM then SIGKILL
   — approach.md already says this; carry it into the design.)

4. **`failed` is a dead end.** The state machine has no `failed → anything` transition
   and the design has no restart-intensity policy — approach.md flags this as "the
   sharpest unresolved question" and recommends "2 attempts per hour then escalate";
   the design doc simply omits retry entirely. Unspecified retry is decided at 2 a.m. by
   whoever writes the Supervisor. Direction: adopt the research doc's own answer —
   bounded attempts per task, then `awaiting_human`; the event log then measures the
   transient/deterministic split empirically, which is the load-bearing principle from
   the Erlang material and one of the few citations in `approach.md` that genuinely earns
   its place here.

5. **No single-instance enforcement** (see Modularity 4) and **no operator playbook**:
   `stack-stale`, `quarantined`, and `awaiting_human` all "wait for a human", but the
   only specified human affordance is reading an HTML event log and merging PRs. What
   does the human *do* — which command re-verifies, retries, re-scopes, abandons? For a
   system whose stated purpose is replacing terminal-scrollback ops, the human's verbs
   are the product, and they are absent.

## Cross-cutting themes

1. **The readiness gate contradiction touches everything.** Scheduler rule, state
   machine, base derivation, LinkArgs usage, the restack path, invariants 1–2/11–12, and
   the L1-reuse story all change meaning depending on which gate is real. This is the
   first thing to settle; most other findings resolve differently under each answer.
2. **The design ignores its own research.** `docs/approach.md` correctly diagnoses the
   edge-triggered reap, the completion crash window (git as evidence), restart intensity,
   and the `CommandContext` kill semantics — and the design doc adopts none of the four
   fixes. Whatever process produced the handoff dropped the highest-value conclusions of
   the reading it commissioned.
3. **Push-before-verify.** One ordering choice (agent pushes and opens its own PR)
   simultaneously breaks invariant 1, hollows out `quarantined`, entangles the design
   with an untracked skill contract, and creates the publish-unverified-work security
   finding. Moving push/PR into the Runner, post-verification, fixes all four.
4. **Predicates without mechanisms.** L1 exports decisions whose inputs only L2 can
   compute; three findings (restack facts, `gh` verbs, path derivation) are one theme:
   finish the library before consuming it.
5. **Two orchestrators, one repo.** The app-vs-CLI boundary is specified as prose
   prohibition with no mechanism; the Activity-file interleaving shows prose does not
   hold it.

## Positions on the author's challenges

*(Positions, not options, as requested.)*

- **Q-A — `merged` vs `verified` gate:** Neither is "right" in the abstract; each defines
  a different product, and the doc must pick. **Position: gate chain-descendants on
  parent `verified`; gate fan-in roots on cross-deps `merged`.** Stacking *means*
  building on unlanded work — a human stacking PRs does the same; "a bad parent poisons
  descendants" is answered by verification (the gate the design already built), and
  `merged` gating overshoots into a design where the stack machinery is dead weight and
  `Resolve`'s bases are wrong (Simplicity 1). If, instead, review latency and simplicity
  argue for `merged` — a defensible product — then be honest about it: delete stacking,
  base every task on `main`, drop LinkArgs/RestackDecision from the app, and build the
  much smaller serial-per-chain executor. Half the system either way.
- **Q-B — scope check at Verifier vs Runner prevention:** Detection is the right v1
  mechanism, in the wrong place in the pipeline. Move the *push* after the gate
  (Security 1) and detection becomes containment; bind mounts and pre-commit guards are
  over-engineering once nothing unverified leaves the machine. Add the free feedforward
  (scope list in the agent's prompt/CLAUDE.md) and stop there.
- **Q-C — reap/reclaim split sufficient?** Necessary, kept, not sufficient. Add (a)
  direct process liveness as the crash test (Deployability 2), (b) artifact inspection
  before status assignment (Deployability 3), and (c) an explicit `unknown` disposition
  for "process dead, artifacts ambiguous" rather than reusing `running` or guessing
  `failed`. One enum value; it deletes the false-`failed` class.
- **Q-D — "no model in the coordination loop" absolute?** Hold it absolutely for state
  transitions and scheduling. It already leaks in two places the doc should name: scope
  declarations are model-authored (L0 — acknowledged in Part 7.5), and the agent
  currently decides when work is published (fixed by Q-B). For triage of
  `failed`/`quarantined`: allow a model as an *advisor that writes a recommendation
  event* a human acts on — never as a writer of transitions. That preserves replayability
  (the recommendation is just an event) while buying the one place judgement genuinely
  helps.
- **Q-E — single-writer SQLite ceiling:** Right ceiling; it forecloses nothing this
  product should want (multi-machine is a different product with different auth,
  per Part 6's own constraints). The two real defects are incidental: missing
  single-instance enforcement (flock) and one connection pool doing both duties
  (Performance 3). Temporal (open question 1): agree with the doc's lean — rejecting a
  server dependency for ~12 tasks is correct, *provided* the durable-execution basics it
  would have given you (bounded retries, artifact-first recovery) are actually specified,
  which today they are not.
- **Q-F — open question 6 (parameterise readiness in L1?):** Yes. The divergence is
  legitimate policy difference, but shadowing (two unrelated predicates in two layers) is
  the smell. Export one prefix-walker parameterised by a `func(Member) bool` readiness
  test (or just a second named predicate beside `ReadyToMaterialise` in `batch/ready.go`)
  so both rules live on one page and drift is visible in one diff.
- **Q-G — open question 7 (model the true DAG?):** Yes — Chains as base-assignment only,
  the DAG as the scheduling truth — but it is only coherent with the two amendments in
  Extensibility 1–2: edges must arrive via the Manifest, and fan-in nodes must be chain
  roots. Without those, "the app models the true DAG" is a sentence, not a design.

**Scope verdict.** This is more system than the goal needs, in the specific sense that
v1 as drawn funds two scheduling regimes, an event-sourcing store, four egress
integrations, an adaptive rate governor, and a five-actor concurrency model. The design
that meets the stated goal: one Go process, one flock, SQLite with status columns + audit
events, one reconcile loop calling L1 predicates, goroutine-per-agent pumping a per-run
transcript file, a verifier behind the same semaphore, Runner-owned push-after-verify,
and one localhost HTTP page. Slack, OTel/Datadog, the MCP permission server, the
adaptive governor, and `POST /tasks` (the Manifest directory *is* the intake — it already
exists and `batch.Load` already reads it) are all deferrable without touching the core.

## Prioritised recommendations

Resolve 1–5 before any code; 6–9 before the corresponding component is built.

1. **Settle the readiness/stacking contradiction (Simplicity 1, Q-A).** Rewrite Parts
   2–5 for exactly one regime — recommended: parent-`verified` for chain descendants,
   cross-dep-`merged` for fan-in roots. Cost of leaving it: every downstream component is
   specified against two conflicting truths, and `batch.Resolve`'s bases are wrong in one
   of them. Next step: a one-page decision note; then re-derive the state machine and
   invariants 1–2 from it.
2. **Replace the heartbeat-reap with level-triggered liveness + artifact-first
   classification (Deployability 2–3, Q-C).** Per-run transcript files tailed by the app;
   `kill(-pgid,0)` + start-time check for liveness; dead-process runs classified from
   worktree/PR state with an `unknown` state for ambiguity. Cost of leaving it: the app
   kills healthy agents and marks finished work failed — the two worst outcomes it exists
   to prevent. Next step: rewrite L3.3's stream paragraph and L3.5 step 1; delete or
   demote invariant 8.
3. **Move push/PR-open into the Runner, after verification (Security 1, cross-cutting
   3).** Cost of leaving it: invariant 1 is false, quarantine is theatre, and the design
   depends on an untracked change to the `/implement` skill. Next step: amend Part 4's
   lifecycle and the skill's contract note; add "nothing is pushed before `verified`" as
   a numbered invariant.
4. **Fix the fan-in intake and placement rule (Extensibility 1–2, Q-G).** `blocked_by`
   in the Manifest schema; fan-in nodes are chain roots. Cost of leaving it: the app's
   headline correctness claim over treepad is unimplementable, and fan-in tickets run in
   worktrees missing their declared dependencies. Next step: small L1 schema PR +
   `to-tickets` chain-cutting rule change.
5. **Finish the L1 surface before consuming it (Maintainability 1–2, Modularity 2).**
   Export restack fact-gathering, `PRList`, `StackLink` (against `batch.Runner`), and a
   machine-readable `tp new` result. Cost of leaving it: the app re-implements the
   work-destroying mechanisms treepad already debugged, behind the same predicate names.
   Next step: a treepad ticket; the import-guard test pattern (`batch/api_test.go:26`)
   already polices the boundary.
6. **Collapse L3 to one reconcile loop + per-agent pumps; add flock; split read/write DB
   handles; drop event-sourcing projection for status columns + audit log (Modularity
   3–4, Simplicity 2–3, Performance 3).** Next step: revise L3.1/L3.2/L3.5 into
   "State + one Loop" before the PRD.
7. **Close the L2/L3 coexistence hole (Modularity 5).** Cheapest: the app writes
   treepad's Activity file as a launch guard. Next step: decide the mechanism and record
   it in Part 3's table.
8. **Specify retry and the human verbs (Deployability 4–5).** Bounded attempts →
   `awaiting_human`; explicit operations for `stack-stale` / `quarantined` /
   `failed` / `unknown`. Next step: add a "recovery" column to the state machine and an
   operator-actions section to the doc.
9. **Govern the Verifier under the cap and key verification to base SHA; scope
   "base moved" to the tracked parent (Performance 1–2).** Next step: two sentences in
   L3.4 and the re-verify step; without them the `verified` cycle thrashes exactly as the
   author feared.

Trim from v1 (scope verdict): Slack, OTel/Datadog, MCP permission server (start with
`awaiting_human` surfaced on the HTTP page), the adaptive rate governor (manual cap
knob), `POST /tasks` (Manifest directory as sole intake).

---

## Appendix — Recommended v1 architecture (simplified)

_The reviewer's constructive answer: the smallest design that resolves every blocking
finding. One deliberate product decision drives all of it._

### The decision: v1 gates on `merged` and does not stack

Pick the serial regime and mean it. A task is ready when **all its dependencies are
merged**; every worktree is created off **latest `origin/main`**. This deletes, not
defers: `gh stack link`, `LinkArgs`, `RestackDecision`, in-worktree restacking, the
`verified → rebasing → verified` cycle, base-SHA-keyed re-verification, the
re-verification storm, the squash-divergence problem, chain-depth review latency, and the
entire "L1 exports predicates not mechanisms" gap (recommendation 5 shrinks to one flag on
`tp new`). It also makes **fan-in trivially correct**: merged deps are in `main`, so they
are in the worktree — no placement rule, no missing-dependency worktrees. `batch.Resolve`
is still used for branch naming and chain topology; its stacked `Base` field is simply
ignored (`main` is passed to `tp new` explicitly).

Cost, stated honestly: within a chain, throughput is gated on human review latency —
depth-3 chain means three review round-trips end to end. Across chains it still
parallelises to the cap, which is where fleets earn their keep anyway. Stacking is the
v2 feature, reintroduced by switching the readiness predicate to parent-`verified` and
adding link/restack steps to the same loop — the schema and loop shape below do not
change.

### Shape: one process, one loop, five files

```
cmd/cc/
  main.go      flock(db path) → open SQLite (1 writer conn + read pool) → ticker + HTTP
  store.go     tasks + events tables; status columns are truth, events are audit
  loop.go      the reconcile tick (below)
  runner.go    provision worktree, spawn agent, push after verify
  verify.go    build/test/lint + scope diff
  http.go      one localhost page
```

No Scheduler/Runner/Verifier/Supervisor/State actors — those are *sections of one loop*
plus pure functions, exactly treepad's own `Reconcile` shape. Goroutines: one for the
loop, one per running agent (waits on the process, records exit), one for HTTP.

**The tick** (every ~15s, level-triggered — never branches on why it woke):

1. Load Manifests (`batch.Load` + `batch.Resolve`); upsert tasks.
2. `gh pr list` once; mark tasks with merged PRs `merged`.
3. Liveness: for each `running` task, `kill(-pgid, 0)` + process-start-time check.
   Dead → classify **from artifacts**: worktree has commits → `verifying`; no commits →
   `failed`. Never from heartbeat timing. (Sleep/wake needs no special case.)
4. Verify: any `verifying` task → build/test/lint + scope diff, under the same
   concurrency semaphore. Pass → push branch, `gh pr create`, → `awaiting_review`.
   Scope violation → `quarantined`. Fail → `failed`.
5. Retry: `failed` with attempts < 2 → `pending`; else → `needs_human`.
6. Top up: claim `pending` tasks whose deps are all `merged`, up to the cap.

**State machine — a pipeline, no cycles:**

```
pending → running → verifying → awaiting_review → merged
                  ↘ failed (→ pending ×2 → needs_human)
                  ↘ quarantined (→ needs_human)
```

### The load-bearing rules carried over from the review

- **Agents never push.** `claude -p` works and commits locally
  (`--permission-mode acceptEdits`, stdout → `runs/<id>.jsonl` **file**, `Setpgid`,
  custom Cancel signalling `-pgid` TERM→KILL). The *loop* pushes and opens the PR only
  after verification — invariant 1 becomes true by construction, quarantine actually
  contains, and the `/implement` skill needs no push behaviour.
- **File-redirected output, not pipes.** App crash leaves agents running; restart
  re-reads pids from SQLite, re-checks liveness, re-tails files. Crash-survival for free.
- **Truth = DB ∪ worktrees ∪ `gh pr list`,** re-derived each tick. No event-sourcing
  projection; `events` is an audit table someone reads after the fact.
- **Coexistence guard:** the app writes treepad's Activity file
  (`launcher.ActivityPath` convention) when it launches, so a habitual
  `tp batch sync --launch` cannot double-launch into the app's worktree.
- **Security floor (not simplified away):** HTTP binds `127.0.0.1` only; `.env` copied
  through a denylist (`ANTHROPIC_API_KEY` at minimum); nothing unverified leaves the
  machine.

### The only treepad changes required first

1. Optional `blocked_by` per ticket in the Manifest schema (fan-in edges;
   `to-tickets` already holds the graph).
2. `tp new --json` printing the worktree path (kills the third copy of path
   derivation).

Both are small, additive L1/L2 PRs. The restack/gh mechanism exports from
recommendation 5 are **not needed for this v1** — they return with stacking in v2.

### Cut entirely from v1

Stacks and everything downstream of them · event-sourcing/projection · `POST /tasks`
(the Manifest directory is the intake) · MCP permission server and `awaiting_human`
(denied permissions surface as `failed` → `needs_human`; add the MCP loop when that
demonstrably hurts) · Slack, OTel, Datadog (the HTTP page shows tick age — silence
detection is a stale timestamp) · adaptive rate governor (a `--max-agents` flag; watch
the run files before building a control system) · TUI.

Each cut is a deferral with a named trigger, not a loss: the loop, schema, and states
above are the stable substrate all of them bolt onto.
