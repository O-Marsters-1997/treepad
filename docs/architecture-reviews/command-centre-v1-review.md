# Architecture Review — Command Centre v1 (design-stage)

_Read-only assessment. Date: 2026-08-19. Scope: the design document
`docs/command-centre-v1.md` (unbuilt Go app), verified against the real code it depends on:
treepad (`batch/`, `internal/treepad/`, `internal/launcher/`, `internal/worktree/`,
`internal/gh/`, `internal/sync/`, `internal/config/`, ADR 0001–0003, `CONTEXT.md`,
`docs/commands.md`) and the two target repos in the `plain` workspace
(`services/`, `support-app/`: `package.json`, `.treepad.toml`, `codegen.yml`,
`scripts/gen.sh`, `.github/workflows/`, `.mergify.yml`, `.gitignore`). Read alongside
`.claude/handoffs/treepad__command-centre-v1-alignment.md`, the predecessor review
`docs/architecture-reviews/command-centre-design-review.md`, and `docs/approach.md`.
No source files were modified._

**Measurement is impossible on an unbuilt system.** There is no profile, no tick-duration
histogram, no coverage, no token accounting, no dependency graph. Every claim below is either
(a) read out of source in treepad or the two target repos, cited by path and line, or
(b) reasoned from the document and labelled as reasoning. Where the predecessor review's
crux finding (merged-gating and stacking are mutually exclusive) was resolved, it is not
re-litigated; what *is* assessed is whether the new resolution introduced new problems. It did.

---

## Verdict

**This should not be implemented as written.** The layer model, the level-triggered loop, the
artifact-first classification, the file-redirected agent output and the refusal to merge are
all genuinely right, and they survive review — the document has clearly absorbed its
predecessor's process-model findings. But the design rests on a small number of factual claims
about the world it operates in, and the load-bearing ones are false. Three in particular:
§6's claim that `support-app`'s CI validates a consumer against an unlanded seam is not merely
wrong, it is *inverted* — a required Mergify check regenerates types from the **production**
schema, so every cross-repo consumer PR is deterministically red until the producer *deploys*,
not merges; §6/§13's claim that `services` has "no producer file for a seam to be" is wrong —
there is one committed 15,569-line SDL file, which is exactly the artifact a seam could be
verified against, and the design deleted the verification on the strength of the false premise;
and §7's claim that `.treepad.toml` is committed, from which five separate deletions cascade,
is wrong — it is globally gitignored in both repos.

Underneath the factual errors sits a structural one. §1's headline argument — that the app and
treepad now hold the *same* readiness predicate, so the divergence the prior review flagged is
gone — is false in source. `batch.ReadyToMaterialise` passes chain position 0 **unconditionally**
and accepts **OPEN or MERGED**; the app's rule is "OPEN only" for descendants and "all blockers
merged" for roots. Nothing was unified. Both halves of that divergence produce concrete failures:
a parent PR that merges before its descendant launches strands the descendant permanently (MERGED
is not OPEN, and `waiting` has no exit and no verb), and a fan-in chain root is materialised on
tick 1 off an un-fetched local `main`, so when its blockers finally merge the app launches an
agent into a worktree that provably does not contain them — the exact failure the predecessor
review named, reintroduced through a different door. **The single most important takeaway: the
document's confidence is calibrated to its citations, and its citations are line-accurate while
its premises are not. It reads as verified and is not.**

---

## Executive summary

**Risk 1 — the seam mechanism does not work in the only workspace it was designed for, and the
design deleted the verification that would have worked.** §6 rests its entire "consumer CI is
genuinely meaningful" claim on `support-app/codegen.yml` reading a repo-local `./schema.graphql`.
That one fact is true and every inference from it is false: `schema.graphql` is gitignored
(`support-app/.gitignore:36`) and absent from disk, so there is no committed copy for an agent to
patch; `scripts/gen.sh` `rm -f`s it and `curl`s it from a **running server**, so the developer path
destroys any hand-patch; the whole GraphQL block is guarded by `if [[ -z "${CI}" ]]`, so the
`Generated files` required check never regenerates GraphQL types at all; and
`.github/workflows/graphql-prod-compat.yml` — a **required** check per `.mergify.yml:20` — curls
`https://core-api.uk.plain.com/graphql/v1/schema.graphql` and typechecks against it. A consumer
implementing an agreed seam field therefore fails a required check *by construction*, from the
first commit, until the producer reaches production. Combined with invariant 9 (`waiting on seam`
never escalates) this means every cross-repo consumer parks red and silent, which is the whole
point of the system failing closed on its primary use case. And the mirror finding: `services`
*does* have a single committed producer file — `packages/core-graphql/src/schema.ts`, 15,569 lines
of SDL in one `gql` literal, already linted by `lint:graphql` as a single path — so a seam
*could* be a verbatim fragment of a real file, `lands_at` has an obvious correct value, and a
producer-side assertion is a grep with no parser. §13 deleted decisions 14, 18 and 20 on the
stated grounds that no such file exists.

**Risk 2 — the only correctness gate is a signal the design never defines, and the retry loop
attached to it can produce a worse branch and label it ready.** §3 step 5, §5 and invariants 10
and 13 all turn on "checks are green" / "checks red", and neither is defined over
`statusCheckRollup`'s heterogeneous union. The obvious wrong reading is vacuously true on an
*empty* rollup, which is exactly the state 15 seconds after a push, so the reachable sequence is
push → tick → "all green" → `review me` → un-draft, on zero CI evidence. Nothing binds the rollup
to a `head_oid`. Nothing expires green — and measured, a base change on these repos fires
`pull_request: edited`, which only `services/.github/workflows/linear-issue.yml:5` listens to;
`itest.yml:6` is `[opened, reopened, synchronize]`, so **no test re-runs when a stack's base
moves**, in a design whose entire correctness story is "CI decides". Then the retry: `services`
already auto-retries its own failures (`itest-retry.yml`, `gh run rerun --failed`, up to
`run_attempt < 4`), so FAILURE is a *transient* rollup state there and the app's retry races
GitHub's; `--log-failed` is unbounded across up to 50 non-fail-fast shards
(`itest.yml:239`, `SHARD_MAX: "50"`) with no cap or redaction rule stated; and the cheapest way
for an agent to make `graphql-prod-compat` green is to **delete the use of the field it was asked
to add**. Green is reachable by deletion, nothing else in the system constrains the diff toward
the ticket (§12.6 concedes it), and the page presents the result as `review me`.

**Risk 3 — the app's own 15-second tick hands treepad a licence to mutate worktrees with live
agents in them, and to push their work.** `restack` runs on **every** `Reconcile`
(`internal/treepad/batch.go:191`), gated only on `--dry-run` and the worktree existing
(`internal/treepad/restack.go:26-40`). It consults no liveness signal — I looked; there is none.
`RestackDecision` (`batch/restack.go:25`) returns `RestackFastForward` (`git merge --ff-only`) for
a clean-and-behind worktree and `RestackReset` (`git reset --hard origin/<branch>`,
`restack.go:101`) for a clean, diverged, patch-equivalent one. A clean worktree is the *normal*
state of an agent that has committed and is thinking, so the design's stated guard in §12.4 ("a
descendant with an agent mid-work is dirty") is a race, not a guard. Separately, `link` also runs
every tick, and `gh stack link` **pushes every branch argument** — confirmed from the tool itself:
"Branch arguments are automatically pushed to the remote before creating or looking up PRs."
§3 step 5's retry re-runs an agent *in the same worktree on a branch that already has an open PR*,
which is inside `LinkArgs`' prefix, so that live agent's intermediate commits are pushed within 15
seconds of being made — breaking invariant 1 through a path invariant 4 does not name, performed
by a subprocess the app delegated to and cannot restrain. On `services` each such push is a
`synchronize` event driving ~72 jobs and an OIDC-federated AWS stage deploy.

**Genuine strengths, material to the verdict.** These are real and should survive any redesign:
the level-triggered tick that never branches on why it woke; liveness as `kill(-pgid, 0)` plus a
start-time identity check, with no timing rule; artifact-first classification of a dead run;
file-redirected agent output rather than pipes; `tasks.status` as truth with `events` as audit
rather than an event-sourcing projection; the app never merging; the `Setpgid` / custom-`Cancel`
process-group discipline; and the deferral table's *form* — each cut carrying a named re-entry
trigger. The document also correctly and independently identified two real treepad defects in
§12.7. That is a good foundation carrying a broken middle layer.

---

## System map

*Layers and components as the document proposes them; an unbuilt system has no modules to map.
Everything about treepad and the two target repos below is verified against source.*

- **Entry points.** `to-tickets` writes (a) one treepad Manifest TOML per repo into
  `<git-common-dir>/treepad/batches/*.toml`, read by `batch.Load` (`batch/manifest.go:64`), and
  (b) rows into the app's *own* SQLite `tasks` table (`ticket_url, repo, blocked_by[], seams[]`).
  A 15-second ticker inside the app. A localhost HTTP page on port 7777. `POST /tasks` is deferred.
- **L0 planning skills** — `to-plan` → `to-seams` → `to-tickets`. Own "above-ticket knowledge":
  repos, edges, seams. `to-seams` does not exist. `to-plan` currently routes to
  `design-an-interface`, which exists in neither skills tree (verified:
  `~/.agents/skills/to-plan/SKILL.md:68`; no such directory under `~/.claude/skills/` or
  `~/.agents/skills/`).
- **The App** — new Go repo, sibling of treepad. `cmd/cc/main.go` + `store.go loop.go runner.go
  http.go`. flock on the DB path → ticker + HTTP. Owns launching `claude -p` agents (one per
  worktree, capped at `max_agents = 4`), liveness, push, `gh pr create --fill`, PR draft state, CI
  reading, retry (2 attempts), one status page. Goroutines: one loop, one per running agent, one
  HTTP. **Never imports treepad's `batch/`** — presented as structural rather than a rule.
- **treepad `tp`** — subprocess, once per repo per tick: `tp batch sync --json` and
  `tp remove <branch>`. Never `--launch`. Inside one `Reconcile`
  (`internal/treepad/batch.go:152-190`) it runs, in fixed order: `materialise` → `launch` (no-op
  without `--launch`) → `link` (`gh stack link`, which **pushes**) → `restack` (`merge --ff-only`
  or `reset --hard`) → `retire` (flag only).
- **CI** — the only correctness gate. `services`: 18 workflow files, ~72 jobs per PR push,
  critical path `itest.yml` (setup 45 min → select → dynamically-sized shard matrix at 40 min →
  finalize 25 min) gated on a **deployed AWS PR stage** claimed from a finite pool
  (`pr-stage-creation.yml`), realistically 30–60+ min, with `itest-retry.yml` auto-retrying to
  `run_attempt < 4`. `support-app`: 6 PR jobs, no timeouts, **no concurrency groups on any
  PR-triggered workflow**, order of 5–15 min. The asymmetry is 4–10×.
- **Seams** — files under `plain/.claude/seams/`, private and uncommitted (`plain/` is not a git
  repo at all, so this holds). Pasted whole into prompts. Three jobs: prompt context with
  `hash(composed)` as run identity; PR draft gate; `lands_at` retirement pointer. Nothing verifies
  a seam.
- **State & persistence.** SQLite at `plain/.claude/command-centre.db` (flock target);
  `tasks.status` is scheduling truth, `events` is audit. Agent stdout → one
  `plain/.claude/runs/<run-id>.jsonl` per run, never rotated. Composed prompts stored per run.
  Git worktrees hold the work; GitHub holds PRs and CI. treepad separately caches PR state at
  `<commonDir>/treepad/pr-cache.json` (`internal/treepad/batch.go:424`).
- **External dependencies.** `claude` CLI (subscription auth via macOS keychain), `gh` + the
  `gh stack` extension (v0.1.0, public preview, `link` additive-only), `tp` as subprocess, `git`,
  and — newly discovered — a live production GraphQL endpoint that gates `support-app` merges.
- **Runtime shape.** One Go process on one laptop; agents as detached process groups; no server
  dependency. Blast radius extends into the company's GitHub org and AWS account via CI.

---

## Claims in the document that are wrong or stale

Required deliverable. Line-number accuracy is high — most citations are exact or off by one
argument line. The failures are semantic, and each sits under a paragraph that *decides*
something.

### Wrong (the fact is not as stated, and something depends on it)

| § | Claim | Correct fact |
|---|---|---|
| §1, §10 | "the app's gate for a chain descendant is 'parent branch has an open PR', which is exactly `batch.ReadyToMaterialise` (`batch/ready.go:12`) … The two layers hold the *same* predicate" | The line is right; the predicate is not. `ReadyToMaterialise` has **three** gates (`batch/ready.go:14-22`): `i == 0` is ready **unconditionally, with no PR check at all**; an already-materialised member (`existing[m.Branch]`) stays ready **regardless of parent PR state**; otherwise the parent PR must be **`OPEN` or `MERGED`**. It keys on `m.Base`. The app's rule matches none of these. Note also `batch.LinkArgs` (`batch/link.go:18-29`) is a **fourth** definition — `OPEN` only, keyed on `m.Branch`, nil below 2 members. Four definitions of "ready", two repos, no shared test. |
| §6, §13 | "`services` builds its GraphQL schema code-first across ~100 TypeScript module directories, so for the flagship case in this workspace there is no producer file for a seam to *be*" | Wrong on both counts. The schema is **SDL-first in a single git-tracked file**: `services/packages/core-graphql/src/schema.ts`, **15,569 lines**, one `gql` template literal. `package.json` treats it as a single path already — `lint:graphql` is `tsx ./tools/graphql-lint/index.ts packages/core-graphql/src/schema.ts`, `watch:graphql` watches that one file, and `packages/core-graphql/codegen.ts` imports `{ schema } from './src/schema.js'` to emit the **git-tracked, 30,160-line** `src/schemaTypes.ts`, whose freshness CI already enforces (`services/.github/workflows/generated-files.yml:39,67-76` — `pnpm run codegen` then fail on `git status --porcelain`). The 80 directories under `packages/core-graphql/src/` hold resolvers, not schema. `services` also serves the SDL at runtime (`infra/stacks/coreApiStack.ts:742`) with an itest pinning served-SDL == local-schema. **This is the premise §13 used to delete decisions 14, 18 and 20.** |
| §6, §12.1 | "In `support-app` this works — `codegen.yml` reads `./schema.graphql` from the repo and needs no running server — so CI genuinely validates the consumer's code against the agreed interface before the producer lands" | `codegen.yml:1` is `schema: './schema.graphql'` — true and the only true part. `schema.graphql` is **gitignored** (`support-app/.gitignore:35-36`, under `# Generated files`) and **absent from disk**; there is no committed copy to patch. `scripts/gen.sh` does `rm -f ./schema.json ./schema.graphql` then `curl "$NEXT_PUBLIC_API_URL/schema.graphql"` — **a running server**, and destructive to any hand-patch. The entire GraphQL block is inside `if [[ -z "${CI}" ]]`, so in CI `pnpm run codegen` never regenerates `packages/graphql/types.ts`. And `.github/workflows/graphql-prod-compat.yml` runs `scripts/gen-graphql-prod.sh` — `curl -sSf https://core-api.uk.plain.com/graphql/v1/schema.graphql` then codegen then typecheck — and `.mergify.yml:20` makes `GraphQL production compatibility` a **required** merge check. A consumer using a not-yet-**deployed** field fails a required check deterministically. §12.1's "mitigated" set is **empty**, and the mitigation is worse than absent: it is a guaranteed red. |
| §7, §13 | "`.treepad.toml` is committed in both `services` and `support-app` (it is listed inside its own `[sync] include`), so a verify key there would push private orchestration config into team repos" | Both halves wrong. `git check-ignore -v .treepad.toml` in both repos → `/Users/ollymarsters/.gitignore-plain:1`; `git ls-files .treepad.toml` is empty in both. It is globally ignored via `core.excludesfile`. And only `services/.treepad.toml:12` self-lists; `support-app/.treepad.toml` does not (so a new `support-app` worktree gets no `.treepad.toml` at all). The leakage objection does not exist — and **five deletions in §13 cascade from it**. |
| §7 | "There is no `check` target to detect" | `services`' `lint` is itself an aggregate: `biome lint … && pnpm run lint:graphql && pnpm run lint:mcpGraphql && pnpm run lint:teamOwnership && pnpm plain-service codegen --check`. The doc's script inventory is also incomplete (`services` additionally has `test`, `test:runEmailE2eItests`, `typecheck:cli`, `typecheck:services`, `lint:actions`, `format`; `support-app` has `codegen`, `test:e2e`, `format`). Neither repo has a `build` in the plain sense for `services` (it is `sst:build`). |
| §7 | "`services`' integration tests need shared docker containers (`test:postgres:start`, `test:dynamodb:start`, `test:s3:start`)" | Inverted. Those containers (fixed host ports `5433`, `8000`, `9090` in `docker-compose.yml`; `scripts/postgres.sh` warns explicitly about port 5433 collision) back **`test:runLocalItests`** (`vitest/local/globalSetup.js`, `**/*.litest.ts`). **`test:runItests`** (`**/*.itest.ts`) bootstraps from a **deployed AWS stage** (`vitest/integration/globalSetup.js` → `bootstrapDatabaseEnvFromCachedStack`, `resolveTestStage`). The port-collision argument is real but attaches to the wrong target, and the AWS-stage dependency is a *different* and stronger reason no laptop can run those tests. |
| §3 step 1, §2 | "rebases clean stale worktrees" / "merge → tp rebases stacked children" | treepad never rebases; `docs/commands.md` contains no `rebase` verb. `restack` does `git merge --ff-only origin/<branch>` (`internal/treepad/restack.go:94`) or `git reset --hard origin/<branch>` (`:101`), and it fetches **only `origin/<that member's own branch>`** (`:87`) — it never compares a worktree against its *base*, so "descendant is on a stale base" is a condition treepad has no predicate for. §1's division-of-labour row assigning `tp` responsibility for "in-worktree rebasing" is false as stated. |
| §3 step 2, §1 | "treepad's own `PRList` selects only the first four fields (`internal/gh/gh.go:51`)" | It selects **five**: `--json number,headRefName,baseRefName,state,url`, at `internal/gh/gh.go:50` (line 51 is `--state all --limit 200`). The substantive half — no `statusCheckRollup` — is right. `url` is a field §5's page needs on every row and the doc's proposed app-side call drops. |
| §10 | "**treepad: none.**" | False on at least three counts. (1) `tp remove` has **no `--force` flag** — `removeCommand()` declares none (`internal/commands/remove.go:11-19`), so `RemoveInput.Force` is unreachable and `doRemove` uses `git worktree remove` (refuses dirty) plus `git branch -d` (refuses non-ancestor, `internal/treepad/lifecycle/lifecycle.go:238-247`). §5's `remove worktree` button therefore fails on exactly the two states a user would press it: `merged` after a squash merge (which §12.3 says is the common case) and `failed` with a dirty tree. (2) There is no way to tell `tp batch sync` "materialise everything except this member", so the fan-in root defect below is unfixable app-side. (3) The Activity-file path the app must write (invariant 8) is computed by `internal/launcher` + `internal/slug`, both unimportable and undocumented. |

### Stale or imprecise (right idea, wrong line, or overstated)

- §12.7 "`tp prune` … iterates every worktree in the repo with no batch or branch filter
  (`internal/treepad/lifecycle/lifecycle.go:326`)" — line 326 is a local declaration; the
  unfiltered loops are at `lifecycle.go:327` (`gatherMerged`, func at `:313`) **and** `:387`
  (`gatherAll`, func at `:380`), so the defect exists twice. Entry point is
  `lifecycle/prune.go:31`. "No filter" also overstates it: `gatherMerged` skips main/detached/
  prunable (`:330`), not-merged-into-base (`:334`), cwd-inside (`:337`), dirty (`:343`) and
  unpushed-ahead (`:352`).
- §12.7 "`MergedBranches` uses `git for-each-ref --merged` (`internal/worktree/worktree.go:130`)"
  — the function is at `worktree.go:122` and the call spans `:129-132`. Behaviour verified; it
  additionally excludes branches whose tip SHA equals the base SHA (`:143`).
- §12.5 "The window is one tick wide" — not bounded at one tick. `ReadyToMaterialise`'s
  `existing[m.Branch]` clause means a worktree materialised off a merged parent persists
  indefinitely, and `loadPRs` (`internal/treepad/batch.go:408-419`) falls back to
  `pr-cache.json` with `stale = true` whenever `gh` is absent or errors, so the "merged parent"
  reading can persist across many ticks.
- §12.3 "descendants … land in `stack-stale`" — conditional on GitHub performing the server-side
  branch rewrite under squash, which `docs/approach.md:182-184` records as **explicitly
  unconfirmed**. If it does not happen, `origin/<descendant>` has not moved, `behind == 0`, and
  `RestackDecision` returns `RestackNone` — silence, not `stack-stale`. The accepted trade's
  mitigation may never fire.
- §12.8 "Rate limits, not money, are the wall … `max_agents` is the only control" — `max_agents`
  does not appear in the tick's cost function at all (see Performance below): per repo per tick the
  app drives ~44 process spawns and ~11 network round-trips as a function of *member* count, plus
  three `gh` process starts. It also bounds nothing remote: concurrent PRs, CI runs and claimed
  AWS pool environments are bounded only by the size of the DAG.
- §13 "No cap, no threshold" for chain depth — contradicted by shipped skill text:
  `~/.agents/skills/to-tickets/references/treepad-manifest.md` already says "If the graph produces
  a chain deeper than about four, flag it to the user rather than writing it silently."
- §13 "keeps `/implement` untouched" — `/implement` is not the "ends at commit your work"
  contract the doc assumes. `~/.agents/skills/implement/SKILL.md:11` says "Run typechecking
  regularly, single test files regularly, and **the full test suite once at the end**", and `:13`
  says "use /code-review to review the work." So up to `max_agents` agents each run the full suite
  locally already. §7 does not eliminate local testing; it launders it into a place the app cannot
  see, cap, sequence or count.
- Unmentioned but load-bearing: the `--json` report carries `pr_stale`
  (`internal/treepad/batch.go:117-121`), whose own source comment warns "Its zero value must never
  be mistaken for 'no PR exists'". The document never reads it, so the app will act on cached PR
  state as though fresh. It likewise never mentions `ActionBlocked`, `ActionGHRequired` or
  `ActionError` (`internal/treepad/batch.go:99-107`).

### Verified correct (checked, and they hold)

`internal/treepad/batch.go:497` (`--json` encodes the report; `WorktreePath` at `:112`, though
`omitempty`) · `internal/treepad/batch.go:384` (`retire` is flag-only, sets `Removable` at `:388`;
the recent "retire from-spec-bulk" commit is unrelated, it retired a *command*) ·
`internal/treepad/restack.go:26` (`restack` handles the post-merge server-side rewrite) ·
`batch/link.go:18` (longest prefix, breaks at first miss) · `docs/commands.md:467`
(`tp remove` exists, branch-targeted, guards documented at `:479-482`, implementation
`lifecycle/remove.go:29-31,42-44`) · `internal/worktree/worktree.go` ancestry blindness to
squash merges · `docs/approach.md:141` (the "3" is literature, not workflow) ·
`design-an-interface` exists nowhere · invariant 8's *mechanism* (`readyToLaunch` →
`!launcher.Exists`, `internal/treepad/batch.go:276-282`; `launcher.Exists` is a bare `os.Stat`,
`internal/launcher/activity.go:26-30`, so existence not mtime — the guard does hold for a
multi-hour run) · seams are safely private (`plain/` is not a git repo).

---

## Findings by dimension

### Simplicity & understandability

**The document is the clearest artifact in this project and the state machine underneath it does
not close.** Nine states, at least four undeclared re-entries into `running`, four reachable
conditions with no state at all, one invariant that contradicts §5 directly, and a central
concept ("ready") with four definitions.

**The state machine has states you cannot leave and conditions with no state.** `checking` has
no exit for "CI never reports" — an empty `statusCheckRollup` (no workflow matched the changed
paths, Actions queued, a workflow requiring approval), and its Verbs column is "—". Measured,
`support-app` sets `timeout-minutes` on nothing PR-triggered, so GitHub's 360-minute default
applies and a stalled `pnpm install` parks a row for 1,440 ticks with no escalation. `review me`
is a latch: §3 step 5 only examines `checking` tasks, so a `review me` row whose CI later turns
red stays green-labelled — which violates the design's own level-triggered principle in the one
place it matters. And there is **no state for**: PR closed unmerged (`services/.mergify.yml` has a
"Close stale PRs after 2 weeks" rule, so this *will* fire on a parked chain); `stack-stale`, which
the JSON report carries (`stack_stale`, `internal/treepad/batch.go:126`) and §12.4 leans on as its
mitigation; a failed push or a failed `gh pr create`, which will retry every 15 seconds forever
with `attempts` untouched because §3 step 4 has no failure edge; and a killed run — pressing
`kill` on a wedged agent produces a dead run with commits, which §3 step 3 classifies as "push",
so the human's abort *publishes* the half-finished work.

**Invariant 5 and the §5 table contradict each other.** Invariant 5 forbids any timing rule from
ever marking a run dead. `needs you` is defined as "CI red after two attempts, **or a wedged
agent**." A wedged agent is alive; detecting it requires a timing rule; therefore `needs you` has
an entry condition the design has made uncomputable. §11 lists the MCP permission loop as
deferred, and `claude -p` is non-interactive, so a denied permission either exits (correctly
`failed`) or hangs — and the hang has no detector, no state and no escape but a human noticing.

**`attempts` is undefined, so invariant 10 is not a bound.** §5 draws `failed → running (attempt
2)` and `waiting on seam → refresh against main → running` as edges of the same kind, and the
table lists `re-run` as a manual verb on both. The doc never says whether `attempts` is one
per-task counter or one per cause, and both readings break: shared means a task that ever saw red
CI can never take the `refresh against main` re-run — killing the seam mechanism for exactly the
tasks that need it; per-cause makes the real bound 2 × 4 with no global cap. Worse, §3 step 8's
cap governs the *launch* path only; step 5's retry is a separate action with no cap mentioned, so
4 running + 4 CI-red retries is 8 concurrent `claude -p` processes against the one wall §12.8
names. And a rate-limit blip is a *global* cause charged to *per-task* budgets: four launches die
in seconds, auto-retry, die again, and the whole fleet is in `needs you` within two ticks.

*Direction.* Annotate every edge with its trigger (tick vs user). Name `attempts` as one per-task
counter that a manual `re-run` resets and that infrastructure failures do not charge. Add a
deadline on `checking`, a `push failed` edge, states for `pr closed unmerged` and `stack-stale`,
and make `kill` a distinct disposition from "dead with commits". Then re-read §5's sentence "Every
parked state has an operation" — it is currently false for `waiting`, `checking` and `review me`.

### Maintainability

**The `.treepad.toml` error is the most expensive kind: a single unverified sentence carrying five
deletions.** §7 bullet 1 and §13 row 3 are false (above), and downstream of them §13 records the
removal of the `verified` state, `quarantined`, seam re-verification (decision 18) and the producer
`lands_at` assertion (decision 20). A `[verify]` key in an already-globally-ignored
`.treepad.toml`, or in a `command-centre.toml` the app owns outright, leaks nothing. The
architectural lesson is about the *form* of §10: "Verified against source" should state assertions
in a shape a test could express (`ReadyToMaterialise accepts PR state ∈ {OPEN, MERGED}`) rather
than as paraphrase. Four of its six bullets could be golden-file checks in the app's own suite, at
which point the document stops being the thing that rots.

**The seam does three jobs with three lifecycles and no owner, and "cannot go stale" contradicts
§3 step 7.** §6 job 3 says "the private seam file is never read again, so it cannot go stale."
§3 step 7 recomposes every task's prompt each tick and §6 job 1 defines composition as the ticket
URL plus "the current contents of every seam the ticket consumes." Either the private file is read
every 15 seconds forever — in which case it can absolutely go stale — or composition flips its
source to `lands_at` at merge, in which case every already-launched consumer's hash changes on the
same tick and the page fills with `seam changed` at the exact moment the user is reviewing a
merge, indistinguishable from a real amendment. The doc does not say which. Separately, once
composition sources from `main`, every unrelated commit to that path changes the hash, so the
`seam changed` badge is permanently on and therefore useless. `lands_at` is worse: written at plan
time by a skill, first *used* days later by the app in a different repo, updated by nobody, and
nothing checks the path exists in the producer's merged tree. That is a one-line fitness function
the design does not have, guarding the most expensive failure shape in the system.

**A Markdown skill writing rows into the app's private SQLite schema, while the app's own ingest
endpoint is deferred.** §10 requires `to-tickets` to emit `repo`, `blocked_by[]` and `seams[]`
"into the app's task table" — a database schema is the least stable thing to depend on, DDL
changes have no compile step, and the writer is prose in a third repo with no tests and no
version. It also does not match what the skill is: today it writes **one** TOML manifest into the
repo it is invoked in, resolved with `git rev-parse --git-common-dir`. The design needs it to write
manifests into several repos plus rows into a DB whose location derives from a workspace concept
the skill has no notion of. Meanwhile §11 defers `POST /tasks` — the wrong half to cut. Have the
skill emit a document the app parses; keep the schema private and versionable.

*Direction.* Consume `tp batch sync --json` as the *answer* to "is this member ready and what is it
stacked on" rather than re-deriving it from prose about `Resolve`. The report already carries
`Base`, `Position`, `Chain`, `PRState`, `PRStale`, `Removable`, `StackStale` and `WorktreePath`.
That drives the duplication to zero while keeping the boundary structural.

### Extensibility

**Four of §11's twelve deferrals are genuinely additive. The rest touch the loop, the schema, the
state machine or an invariant.** Additive: Slack/OTel egress; TUI (`tp ui` exists); chain-depth
display and cap; multi-machine (declared never). The others:

- **Local verify** rewrites the loop, the state machine, the config schema and §1's identity row.
  Its outcome — "dead run with commits that failed a local check" — is not `checking`, not `failed`
  ("dead run with no commits"), and not `needs you` without discarding the retry it exists to
  enable. §7 deleted the two words that would name it. §8's config forecloses it in one sentence.
- **Scope containment / `quarantined`** adds a third parked state against §5's "acyclic apart from
  the two explicit re-entries", and its exit does not exist: §6 fixes re-runs as "incremental on
  the existing branch (no force-push, no discarded work)", so re-running a quarantined agent leaves
  the out-of-scope commits and the diff check fails again, forever.
- **Automatic rebase of stale descendants** is not additive at all. Its target population is
  exactly the population `RestackStale` exists to *refuse*, and `restack.go`'s contract states the
  reason ("Treepad never stashes on an agent's behalf"). Re-entry means either the app rebases —
  against invariant 4 and §1's Must-never — or treepad reverses ADR 0003. It also needs a new
  cross-step interlock, since you cannot rebase a dirty worktree without first killing its agent.
- **`POST /tasks`** converts a single-writer artifact into an unreconciled dual write. Nothing today
  asserts that every task row has a Manifest member or vice versa. Write only the DB row and the
  task never appears in `batch.Load`'s glob, so no worktree is materialised and the row sits in
  `waiting` — a state with no verb — silently forever. Write the Manifest too and the app must emit
  TOML and re-derive branch names without importing `batch/`, duplicating `DeriveBranch` and
  `slug.Slug`, and choose append (irreversible per ADR 0003) versus new chain.
- **Adaptive rate-limit governor** and **MCP permission loop** share a root cause: the app never
  reads agent output, so a rate-limited or permission-denied run dies with no commits and is
  indistinguishable from a crash or a wedge. Both re-entries require reversing the never-piped
  decision or invariant 5.
- **Parent decision-trace** needs the run transcript to become queryable — i.e. the `events` schema
  §13 deliberately deleted — and it poisons `hash(composed)`, which becomes a function of a mutable
  upstream artifact that changes on every parent re-run.

A framing problem covers the whole table: every trigger is a human-perception threshold ("often
enough to notice"), and the design records no data against which any could be evaluated — no
CI-latency field, no stranded-descendant counter, no rate-limit signal, and no drive-by measure
because §7 deleted the `files` field. Eleven deferrals are gated on instruments the design does not
build.

**Foreclosed axes the document does not list as limits.** A second workspace — DB path, port,
`max_agents` and flock are all per-workspace, so the one genuinely global resource (the
subscription rate limit §12.8 names as *the* wall) is capped per process and two workspaces
silently double concurrency. A non-GitHub repo — PR-state strings are the domain vocabulary of §4,
§5, `batch/ready.go`, `batch/link.go` and `retire`, and draft state is the seam gate. A non-`claude`
agent — treepad already generalised launching into a templated argv (`launcher.Render(cfg.Batch.
Launch, data)`); the app declines it, hardcodes `claude -p`, closes §8's config ("a name and a path
and nothing else") and pins one vendor's flags into invariant 11. A seam with more than one
producer — unrepresentable, though a three-repo feature has fan-in seams almost by construction. A
ticket in two chains — `DeriveBranch` is `prefix + slug(ref)` (`batch/resolve.go:84`), so the same
ticket in two chains yields the same branch with two different `Base` values; whichever
materialises first wins and the second is silently `ActionSkipped`, with `batch.Load` unioning
every `*.toml` with no cross-manifest uniqueness check. And **fan-out**, which the shipped
`to-tickets` rule does not cover: two slices each blocked by the same single blocker both satisfy
"joins that blocker's chain, directly after it", so `Resolve` bases the second on the first's
branch and an agent works in a worktree containing code it does not depend on. Fan-out is the
*more* common plan shape. The correct single rule is "a chain is a maximal path in which each node
has exactly one blocker **and** its blocker has exactly one dependent."

### Security

**In these repos, opening a pull request is arbitrary code execution with the company's
credentials, and the design has deleted every gate in front of it.** `services/.github/workflows/
pr-stage-creation.yml` fires on `pull_request: [opened, reopened, synchronize]` with
`id-token: write`, assumes `AWS_PR_ACCOUNT_DEPLOYMENT_IAM_ROLE` (`:54-56`, `:258-260`, `:338-340`,
`:372-374`, `:403-405`) and performs a real SST + OpenTofu deploy into the company AWS account,
claiming an environment from a finite pool. `itest.yml` has the same trigger and `id-token: write`.
In `support-app`, five PR workflows inject `secrets.TIPTAP_PRO_TOKEN` into the job env. Crucially
the app pushes to the **same** repo, not a fork, so the `pull_request` fork restriction that
withholds secrets never applies. Any agent-authored change to `package.json` scripts, a vitest or
lint config, a devDependency, or `.github/` itself executes on the runner with that identity — and
§7's answer to "what checks the diff first" is the single word "Nothing", with §12.6 conceding
"ticket precision is inside the correctness surface and nothing checks it now that scope
containment is cut." §7 costs this as "CI minutes and the possibility of a broken branch being
briefly public." That undercounts by a category. `pull_request_target` appears in neither repo,
which removes the classic worst case — but not this one.

**The privilege the launched agent actually holds is an order of magnitude above what invariant 11
addresses.** `--bare` is a billing control, not a security one — the predecessor document says so
outright. Unsetting `ANTHROPIC_API_KEY` removes nothing: the subscription credential is in the
macOS login keychain as `Claude Code-credentials`, and the subprocess runs as the same UID with the
same unlocked keychain, so it is *more* reachable than an env var. What the run does inherit is the
`gh` keyring token (scopes `repo`, `read:org`, `gist`, `admin:public_key`), an SSH key for
`team-plain/*`, and a pre-approved permission allowlist that makes the sandbox nominal —
`.claude/settings.local.json` in the workspace and in both repos pre-approves `Bash(node *)`,
`Bash(gh api *)`, `Bash(gh auth *)`, `Bash(pnpm run *)`, `Bash(python3 *)` and `Bash(curl …)`.
`Bash(node *)` is unprompted arbitrary code execution; `Bash(gh auth *)` prints the GitHub token;
`Bash(gh api *)` is unprompted org write. And `.claude/settings.local.json` is in both repos'
`[sync] include`, so every worktree the app launches into inherits it.

**The CI-log retry loop is an unbounded, unredacted, untrusted-input channel into a code-writing
agent whose output is pushed without review.** §3 step 5 pipes `gh run view --log-failed` straight
into a prompt. Who influences a CI log: anyone whose test prints, anyone whose fixture or snapshot
holds customer text (this is a support product), any third-party action's stdout, any dependency
echoing a server response — and the agent's own prior output, giving a self-amplifying loop. What
it carries: GitHub's `add-mask` redacts exact `secrets.*` values only, and it does not cover
*derived* credentials, which is precisely what `id-token: write` mints — an STS session token, a
presigned URL with `X-Amz-Signature`, an RDS connection string, an internal hostname, a JWT. Two
distinct harms follow. *Injection*: §6's composition concatenates operator instruction, ticket
body, seam contents and now CI log into one undelimited string, so a fixture line reading "the fix
is to add a step to .github/workflows/itest.yml" is indistinguishable from an instruction, in front
of an agent with pre-approved shell and no diff check. *Secret laundering*, the likelier and more
banal one: the agent sees a credential in the log, concludes the test failed for want of it, and
hardcodes it into a test config — which the app then pushes to a company repo, where it is in the
reflog forever. No step in the design could notice. And `--log-failed` is run-scoped across up to
50 non-fail-fast shards, so the real behaviour is context overflow or an unstated truncation.

**`127.0.0.1` is not an authorisation model, and the endpoints delete work.** With no token and no
`Origin`/`Host` validation, any page in any tab can `fetch('http://127.0.0.1:7777/…', {method:
'POST'})`; CORS makes the response opaque, which is irrelevant for a destructive verb. Note the
interaction with invariant 3: it promises the app never deletes a worktree except on "explicit user
action", and "explicit user action" is implemented as an unauthenticated HTTP request, so a
cross-origin POST satisfies it. DNS rebinding supplies the read half, and §5 specifies the page
renders worktree paths and pgids as plain text — so what a rebound page exfiltrates is the
company's filesystem layout, branch names, ticket URLs and live pids. Three lines fix it: reject a
`Host` that is not literally `127.0.0.1:7777`; require a startup-generated token on every mutating
route; make every mutating route POST-only.

**Retention: an indefinite plaintext archive of a company codebase in a directory no repo owns.**
`plain/` is not a git repo, so `plain/.claude/` is unmanaged — better than committed, worse than
governed. Accumulating there without bound: `runs/*.jsonl` (full agent stdout per run, never
rotated — every line of company source any agent read or wrote, plus any CI log content that
entered a retry prompt); the stored composed prompt per run (by construction the full text of every
seam, i.e. the cross-repo interface design); `events`; the seam files; and every worktree for a
`failed` / `needs you` / `waiting on seam` task, which invariant 3 forbids deleting — each holding
a copied `.env`. And `.env` is *not* gitignored in either repo (verified) while being in both
`[sync] include` lists, so every fresh worktree has real local secrets as untracked files that
`git add -A` sweeps up, on a branch the app then pushes. Invariant 11's "`.env` is copied through a
denylist" describes a mechanism that does not exist: `internal/sync/sync.go:246` parses an
**allowlist** of include globs with optional `!` exclusions, and neither repo uses a single `!`.
More structurally, the app cannot enforce it from where it sits — treepad does the copying, reading
a config file the app does not own and (per §10) is not going to change. A numbered invariant that
cannot be tested from the app is worse than an absent one.

**The draft gate is advisory and this repo has less behind it than the design assumes.** A draft PR
greys out the merge button and Mergify respects it (`services/.mergify.yml:83`), but any teammate
with write access clicks "Ready for review" and merges. `.mergify.yml`'s own header documents two
bypasses (`bypass` label skips review; label plus an emergency checkbox skips checks *and* review),
and `services/.github/CODEOWNERS` is **0 bytes**, so nothing routes an agent-authored PR to the
human who owns that code — while reviewer load is the one cost of this design that scales linearly
with its throughput. And no provenance is marked anywhere: `gh pr create --fill` makes
agent-written commit messages the PR description, under the operator's identity, so a teammate
reading `git log` or an auditor reading CloudTrail sees a human where there was none. Textbook
confused deputy: the app is the deputy, the operator's credentials are the authority, the
ticket/seam/log content is the less-privileged caller, and there is no validation at the boundary.

*Direction, cheapest first.* A path denylist on the diff before pushing (`.github/`,
`infrastructure/`, lockfiles, `package.json`, `pnpm-workspace.yaml`, `.env*`) — ~20 lines, zero
per-ticket input, and the only thing between "an agent wrote something odd" and "something odd ran
with `id-token: write`". Cap and scrub the CI log, write it to a file and reference the path rather
than inlining it, and fence it as untrusted. Add `.env*` to `~/.gitignore-plain`. Restate invariant
11 as a startup precondition the app validates (read each repo's `.treepad.toml`, assert every
synced path is git-ignored, refuse to start otherwise) or drop it. Launch runs with a
narrowly-scoped `GH_TOKEN` and a launched-run settings file that denies `gh auth`, `gh api` and
`curl`. `0700` on `plain/.claude/`, a retention rule on `runs/`, and store a hash plus a path for
the composed prompt rather than the text. And replace the draft gate with a real one: post a check
run (`agent-authored: unreviewed`) in the failing state on every PR the app opens, add it to
`common_checks`, and let only an explicit human action clear it — that composes with the existing
merge policy and, unlike a draft flag, fails **closed** if the app crashes mid-tick.

### Performance & scalability

**The CI gate is an undefined predicate over a union type, evaluated on a 15-second poll with no
commit binding, and the most likely wrong reading un-drafts an untested PR.** `statusCheckRollup`
is a union: `CheckRun` carries orthogonal `status` (QUEUED/IN_PROGRESS/COMPLETED) and `conclusion`
(SUCCESS/FAILURE/CANCELLED/SKIPPED/NEUTRAL/TIMED_OUT/STALE/STARTUP_FAILURE/ACTION_REQUIRED);
`StatusContext` carries a separate `state`. The doc never enumerates any of it. "Every check
present is SUCCESS" is **vacuously true on an empty rollup**, which is the state of a freshly
pushed head for the window between the push and check-suite creation — and §3 step 4 pushes inside
a tick body while step 5 evaluates 15 seconds later. On `services` I count ~72 jobs attaching to a
PR push; none exist at t+15s. The mirror bug is also reachable: after a retry pushes a fix, the
rollup can still carry the previous commit's FAILURE, so attempt 2 is spent against a failure
already fixed. `services` also deliberately separates reported from required —
`lint-typecheck-unit-test.yml:84-95` exists so branch protection can require a single "Unit Test"
check independent of shard count — and the app has no notion of *required*. Path-filtered
workflows (`opentofu-boundary-reminder.yml:5-9`) make "absent because filtered", "absent because
not yet created" and "absent because the workflow is broken" indistinguishable, and their correct
actions are opposite. **This is the one gate in a design that removed every other gate.**

**Green never expires, and on these repos a base change does not re-run tests.** §5 states it
outright: "there is no `verified` state — CI is the only gate, so nothing is verified locally and
nothing perishes when a base moves." Measured: a base retarget fires `pull_request: edited`, and
only `services/.github/workflows/linear-issue.yml:5` lists `edited`; `itest.yml:6`,
`local-itest.yml:6` and `pr-stage-creation.yml:6` are `[opened, reopened, synchronize]`. So a
descendant keeps the green it earned against a base that no longer exists, and `review me` means
"passed CI against something else". If the repo instead enables "require branches up to date", the
cost reappears as the quadratic: merging a depth-D chain bottom-up forces D(D−1)/2 pushes, each
~72 jobs on `services` — D=4 is ~432 jobs and six AWS stage deploys, with
`pr-stage-creation.yml:10,301` setting `cancel-in-progress: false` so the deploys **queue**.

**The retry loop races GitHub's own retry, and can produce a strictly worse branch labelled `review
me`.** `services/.github/workflows/itest-retry.yml` triggers on `workflow_run: [completed]` for
"Integration Test" and, when `conclusion == 'failure' && run_attempt < 4`, runs `gh run rerun
--failed`. FAILURE is therefore a *transient* rollup state on this repo — the existence of that
file is the team's own measured statement that these tests flake — and the app's reaction is
immediate and irreversible: fetch log, re-run agent. The agent's fix pushes,
`itest.yml:15-17`'s `cancel-in-progress: true` cancels the in-flight retry, a fresh shard run
starts, repeat. Then the sharper mechanism: the cheapest way for an agent to make
`graphql-prod-compat` green is to **remove the use of the new field** — undo the ticket. Lint,
test, typecheck and generated-files then all pass, step 5's green branch promotes the row to
`review me`, and invariant 13 un-drafts it. This generalises: given a flaky test, the agent weakens
the assertion, adds a retry, or skips it, and a `.skip` disappears into 3,525 `.itest.ts` files and
a 50-shard log. The human notices only by reading the diff — the work the system exists to remove —
and the page's label discourages looking. **Yes, the design can produce a branch worse than the one
it started with, and the honest answer to "how would the human notice" is that they would not.**

**A large fraction of "CI red" is not a code problem, and the app's response is unconditional.**
Nameable from these configs rather than guessed: a second push cancels the in-flight run
(`cancel-in-progress: true`) → CANCELLED → non-SUCCESS → the app asks an agent to fix "The
operation was canceled", *self-inflicted by its own retry push*; `itest.yml`'s `setup` polls for a
deployed stage and times out at 45 minutes, so a failed deploy or an exhausted environment pool
yields an AWS/OpenTofu log no code change fixes; `pnpm install --frozen-lockfile` in every job of
both repos; `support-app`'s `lint.yml` installing eslint ad-hoc; `graphql-prod-compat` depending on
a live production endpoint; and `--max-old-space-size` pinned at 3584/4096/30000, so OOM presents
as "JavaScript heap out of memory", which an agent will confidently "fix" in application code.
Note `services` already publishes a *structured* failure list — `itest.yml:329` uploads
`itest-failed-shard-<n>` artifacts of failed test paths. Feeding the agent that instead of raw logs
is cheaper and more informative, and it is a few lines.

**Tick cost scales with member count, not agent count — so `max_agents` does not govern the wall
§12.8 names.** Counted from source, one `tp batch sync --json` with N materialised members and C
linkable chains: `git worktree list --porcelain` ×1, `git rev-parse --git-common-dir` ×1,
`gh auth status` ×1, `gh pr list --state all --limit 200` ×1, `git branch --list` ×N,
`gh stack link` ×C (**and it pushes**), `git fetch origin <branch>` ×N
(`internal/treepad/restack.go:87` — per member, per tick, forever, and it fails every 15 seconds
for a never-pushed root), plus 3N–4N status/upstream/rev-list/cherry calls. At N=6, C=2 that is
**~44 process spawns and ~11 network round-trips per repo per tick**, plus the app's own
`statusCheckRollup` call — ~350 spawns and ~88 round-trips per minute across two repos. Three `gh`
process starts per repo per tick is 24 `gh` cold starts a minute for data that changes on the order
of minutes. And the expensive call is the hot one: mirroring `--limit 200` with
`statusCheckRollup` on `services` would request on the order of 10⁴ nodes every 15 seconds, which
is the shape that trips GitHub's *API-CPU-seconds* secondary limit rather than the request-count
one. There is no ETag, no cache (treepad caches its own at `pr-cache.json`; the app's richer call
has no equivalent), no jitter and no backoff. **The failure direction is the worst possible**: the
doc specifies no error handling, so an empty JSON array from a rate-limited or unauthenticated `gh`
is indistinguishable from "no PRs", and under the vacuous-truth green reading a rate limit promotes
the **entire fleet** to `review me`.

**Head-of-line blocking in the single loop.** Everything in §3 is serial in one goroutine, and
step 1 is a synchronous subprocess whose duration the app neither controls nor bounds. Measured
worktree provisioning cost: `services/.treepad.toml [sync] include` lists `.sst/` — **28 GB** — and
`support-app`'s lists `node_modules/` — **2.0 GB across ~155,000 files**. On APFS treepad clones
these copy-on-write (`internal/sync/darwin.go`), so the byte cost is largely avoided and I should
not overstate it; the inode cost is not, and `internal/sync/sync.go:387-399` falls back to plain
`io.Copy` where clone is unsupported, at which point materialising one `services` worktree copies
28 GB of real bytes inside a tick body with no budget and no timeout. While the loop sits in that,
or in N serial `git fetch`es, or in a `gh stack link` that is pushing: repo 2 gets no tick, dead
agents are not reaped, `checking` tasks are not read, the retry does not fire — and §11's only
silence-detection mechanism, "a tick-age field on the page", is blocked by exactly the condition it
exists to detect. A fixed `time.Ticker` whose body exceeds its period degrades silently: Go drops
ticks, so the effective period is `max(15s, body)` with no signal. The doc's justification — "this
is treepad's own proven `Reconcile` shape" — does not transfer: treepad's `Reconcile` runs once per
CLI invocation with a human waiting, where none of these costs matter.

*Position on the loop shape.* Right for the state machine, wrong for the I/O. Run `tp batch sync`
and both `gh` calls as jobs whose results land on a channel, at most one in flight per repo, drop a
tick when the previous has not returned, and have the loop read only the last completed snapshot
plus its age. That keeps one reconcile goroutine and level-triggering, bounds the loop body, and
produces the tick-duration observable the design currently cannot emit.

### Modularity (deep modules + coupling & cohesion)

**"Ready" has four definitions in two repos with no shared test, and §1's clean-layers argument is
the inverse of the truth.** The four: `ReadyToMaterialise` (position 0 unconditional; already-
materialised sticky; else parent OPEN **or MERGED**; keyed on `m.Base`); `LinkArgs` (OPEN **only**,
keyed on `m.Branch`, min 2); `readyToLaunch` (Activity-file absence); and the app's §4 rule. The
app's stated predicate matches **none** of treepad's three, and is closest to `LinkArgs` — the one
predicate that is not about readiness at all. Concrete divergences run both ways. treepad
materialises what the app calls not-ready: a MERGED parent passes `ReadyToMaterialise`, so a
descendant worktree is created with `base` = the parent's merged branch, persists indefinitely via
the `existing` clause, is refused by `LinkArgs` because MERGED breaks the prefix, and is never
launched into by the app because MERGED is not OPEN — three components, three answers, one
permanently empty worktree on a dead base. And the app wants to launch what treepad has not
materialised: `gh` unavailable this tick yields `ActionGHRequired` and no worktree, while the app's
*separate* `gh pr list` may have succeeded, so §3 step 8 tries to launch into a path that does not
exist. `pr_stale` exists to signal exactly this and the document never reads it.

**This is the answer to the "no-import boundary" question, and it is a clear position: the rule as
written buys nothing and costs a silent second implementation.** `batch/` exports precisely the
pure predicates and hides the mechanisms; its public surface is *designed* for reuse of the
decisions, and `batch/api_test.go:26` already polices it. Forbidding the import means the app
reimplements the one thing that was safe to share — and got it wrong in the sentence claiming the
two are identical, before a line of code exists — while every *untyped* coupling remains: the JSON
report shape (`ReportEntry` lives in `internal/treepad`, so the app must hand-mirror a struct whose
fields all carry `omitempty`, with no version and no contract test), the Manifest TOML, the
`pr-cache.json` semantics, and the Activity-file path. The rule prohibits the typed, testable,
versioned coupling and leaves every untestable one in place. **The right correction is not
vendoring — it is dependency inversion**: consume the report as treepad's *verdict* on readiness
rather than recomputing it, and add one checkable invariant ("the app never launches into a member
whose report entry is not `created`/`skipped`") in place of the reimplemented predicate.

**Inappropriate intimacy on the one coupling with no compiler and no error path.** Invariant 8
requires the app to write `<commonDir>/treepad/activity/<slug.Slug(branch)>.log`
(`internal/launcher/launcher.go:32-36`), computed from `internal/launcher` and `internal/slug` —
both unimportable by construction, and the path is documented nowhere in `docs/` or `README.md`.
The app must reimplement `slug.Slug`'s exact folding (lowercase, collapse every run of
non-alphanumerics to one hyphen, trim) with no shared test vector. **Drift fails open**, on unusual
branch names, and what it stops preventing is two agents in one worktree — the worst thing this
system can do. Worse, the file has two owners with two meanings: the app uses it as a mutex,
treepad derives `RunState` from its **mtime** against a 90-second window
(`internal/launcher/activity.go:22-45`), and because §8 sends stdout to `runs/*.jsonl` the mtime
freezes at launch — so 90 seconds in, every app-driven worktree reads `idle` in `tp ui`, and
`tui_update.go:351` refuses a manual launch on anything not `pending`. One file, two liveness
models, and the treepad one is wrong for essentially the entire duration of every run. Exposing
`activity_file` on `ReportEntry` is a one-field change that deletes a reimplementation of two
private functions — another reason §10's "treepad: none" is false.

**The seam is a shallow module masquerading as a contract.** Its interface is a file's entire
contents pasted verbatim into a prompt: the interface *is* the implementation, byte for byte,
hiding nothing while adding a file, a directory convention, a hash, a `lands_at` field and a skill.
§6's abandonment of the "verbatim file" rule makes it worse — once a seam is "a fragment: the few
declarations both sides must agree on", it has no referent in any repo (and per the factual
correction above, it *could* have had one). Name it precisely: **shared mutable state** across a
**distributed monolith**, with **connascence of value at a distance** (producer code, consumer
code and the seam file must agree on names, types and nullability with no compiler, test or CI job
co-locating them) and **temporal coupling** via `lands_at`. Cohesion is a grab-bag: job 1 wants the
seam to be whatever helps an LLM; job 2 wants a boolean about producer state, which is an edge that
already exists in the task table as `blocked_by[]`/`seams[]`; job 3 wants a path in another repo,
which is metadata about the producer. Only job 1 concerns the interface's content.

**The divergence sequences, and why the only detector points the wrong way.** *Producer renames a
field*: nothing fires — `hash(composed)` hashes the *seam*, and the seam did not change, reality
did. Discovered after the producer merges *and deploys*, by which point the consumer's whole query
layer plus every component consuming it is wasted, and the re-run is "incremental on the existing
branch" so the agent is patching its own wrong code. *Producer makes a field nullable*: strictly
worse, because it is silent at every gate — generated types still say non-null, nothing fails to
compile, and it surfaces as a runtime null. There is no state in §5 for "green, merged, wrong".
*Producer implements at a different path than `lands_at`*: discovered at the moment the app needs
`lands_at` to work, and the paste silently yields nothing. *Review feedback changes the interface
and the human patches by hand*: the worst of the set, because the seam file is unchanged, so the
hash still matches, so every consumer row reads as *correct and current* while being wrong. Common
root cause: **`hash(composed)` detects changes to the seam, and every one of these failures is a
change to reality with the seam held constant.** Cheapest real fix is not verification but
*attribution* — have the producer's PR declare which seams it claims to satisfy, so the seam moves
from nobody's responsibility to somebody's. And with `services/packages/core-graphql/src/schema.ts`
now known to exist, a mechanical producer-side assertion is available and cheap.

**Merge-time collision on the generated files.** Verified: `support-app/packages/graphql/types.ts`
is git-tracked, **23,067 lines**, marked `linguist-generated=true` in `.gitattributes:2` (display
only) with **no merge driver** — while `.gitattributes:7` gives `.github/workflows/*.lock.yml` a
`merge=ours`. Its *input* (`schema.graphql`) is uncommitted and fetched per-developer from whatever
`NEXT_PUBLIC_API_URL` points at, so two developers on different stages regenerate it from different
schemas. Recent per-PR churn is small (+23/-0, +4/-25, +16/-0), which is not reassuring: codegen
output is grouped by type and alphabetised, so two PRs adding fields to the same GraphQL type edit
the *same hunk* — and a cross-repo seam means producer and consumer touch the same types by
definition. The conflict is the expected case, not a risk. Layer §12.3's `stack-stale` on top and a
human is hand-resolving a 23k-line generated file, with no state in §5 for `stack-stale` and no
verb on the page. And "regenerate" is not a safe automatic resolution: locally it means
`pnpm codegen`, which `rm -f`s `schema.graphql` and refetches from a dev server — silently
discarding the agent's patched schema and producing types for whatever is deployed to dev, which is
neither the seam nor `main`; in CI it means nothing at all, because `gen.sh` skips the block. There
is no operation available anywhere that regenerates `types.ts` against the agreed interface.

**A published-language boundary with no published language.** The app runs `tp` once per repo and
stitches, so it owns cross-repo joins on identifiers treepad never validates — branch strings,
ticket URLs, seam names. Nothing asserts that a `seams[]` entry in `support-app`'s task table names
the same seam as the producer declaration in `services`'. §6 presents "no contract registry to
maintain" as simplification; what it means is that the identifier namespace has no definition, so
the failure mode of a typo'd seam name is a task that reports green with an empty seam in its
prompt.

### Deployability

**The load-bearing safety story is not implemented by the code the app delegates to.** §12.4's
mitigation is "`tp` reports `stack-stale`, human handles it". `restackOne`
(`internal/treepad/restack.go:42-84`) fetches `origin/<e.Branch>` and computes
`worktree.AheadBehind` against **its own upstream**; `RestackDecision` returns `RestackNone`
whenever `behind == 0`; `doctorCheckStackStale` (`internal/treepad/doctor.go:205-231`) uses the same
inputs. **Nothing in treepad ever compares a worktree against its base.** So "descendant is on a
stale base" — the exact condition §12.3 and §12.4 name as the common one — is a condition treepad
has no predicate for, and `stack-stale` never fires for it. The signal plumbing is fine
(`stack_stale` is in the JSON), and it is the wrong signal; `remote-gone`, which would catch the
closed-parent case, exists only in `tp doctor`, a command this design never invokes.

**The failure sequences, ranked by how bad each is.**

1. *Parent's PR open, its CI red.* Descendant launches on a base known to be broken, works an hour,
   pushes, and opens a PR based on it. Its CI is **uninterpretable** — the app cannot distinguish
   "descendant is wrong" from "parent is wrong" — and §3 step 5 hands the descendant's agent the
   *parent's* failure log. That is a category error: the cheapest thing the agent can do is patch
   the parent's bug inside the descendant's worktree, producing a duplicate fix that conflicts when
   the real one lands. Two attempts burn (~144 jobs and two AWS stage deploys on `services`), the
   row parks in `needs you`, and when the parent's fix arrives `origin/<descendant>` has not moved
   so nothing reports anything. **Severity: heavy waste plus duplicated fixes; no work loss.**
2. *Parent's PR closed unmerged.* Not hypothetical — `services/.mergify.yml` closes stale PRs after
   two weeks, and §6 says a consumer can park *indefinitely* by design. `existing[m.Branch]` keeps
   descendants "ready" forever; `LinkArgs` breaks at the parent and returns nil, permanently, since
   `link` cannot unlink; if the parent's branch is deleted, `restack` still succeeds on the
   descendant's own ref and reports `RestackNone`. §5 has no state for "my base is gone" and no
   operation. **Severity: descendants' PRs point at a dead base, shown as `review me`; recovery is
   manual rebase-and-force-push per descendant with no tooling.**
3. *Parent force-pushed, or any base rewrite.* **This is the one with real work loss, and the app
   causes it.** `restack` runs on every `Reconcile` with no liveness input; only a single sampled
   `worktree.Dirty` stands between a live agent and mutation, and two more git subprocesses run
   after that sample before `merge --ff-only` or `reset --hard` executes. The *commonly reached*
   branch is not even the reset: an agent that has committed and is thinking has a clean tree with
   `ahead == 0` relative to a rewritten upstream, which is `RestackFastForward` — files rewritten
   under a running `claude -p`, invalidating every read it has made. Patch-unique commits do
   survive (`git cherry` yields `+` → `RestackStale`); uncommitted edits and the agent's worldview
   do not. A four-agent fleet across two repos evaluates this race on the order of a thousand times
   an hour. **Severity: silent destruction of uncommitted agent work, caused by the app's own tick.**
4. *Parent squash-merges mid-run.* Once MERGED, `LinkArgs` breaks and `link` stops for the whole
   chain forever. The descendant's PR keeps `base = <parent branch>`; GitHub retargets it to `main`
   on base deletion, at which point its diff re-presents the parent's unsquashed commits on top of a
   `main` that already contains their squashed equivalent — a wrong diff no check name explains.
   `origin/<descendant>` did not move, so `RestackNone`. **Severity: highest expected cost, because
   the doc says to expect it as the common case and §12.3's stated mitigation does not fire. A
   plausible-looking wrong PR reaches the team's `main`.**
5. *Depth 3+, middle goes `needs you`.* The human does stack surgery unaided — no `tp` rebase verb
   exists — on the team's remote, while the app keeps ticking and may `reset --hard` the worktree
   they are working in. `re-run`, the only verb offered, re-runs the agent; it does not repair a
   base. **Severity: the human is the recovery mechanism and has no tools.**
6. *The extra sequence, not in the brief.* `link` runs unconditionally and `gh stack link` pushes
   its arguments (confirmed from `gh stack link --help`: "Branch arguments are automatically pushed
   to the remote before creating or looking up PRs"). A retry re-runs an agent on a branch that
   already has an open PR, so that branch is inside the prefix and its live agent's intermediate
   commits are pushed every 15 seconds — breaking invariant 1 via a path invariant 4 does not name.
   On `services`, `pr-stage-creation.yml:10,301` sets `cancel-in-progress: false`, so a chatty agent
   enqueues a serial deploy backlog for its own PR while `itest.yml:15-17` cancels and restarts its
   own tests indefinitely — a `checking` state that may never resolve.

**So: yes, something worse than staleness can happen — uncommitted agent work destroyed by the
app's own tick, and structurally wrong diffs reaching `review me` on the team's `main` with no
warning.** The stated mitigation is inadequate on three counts: it fires for a condition that is
not the one described; §5 has no state and no operation for it even when it does fire; and the
finding that would catch sequence 2 lives in a command the design never calls.

**The tick mutates worktrees the app does not own.** `Reconcile` filters manifests only when
`in.Batch != ""` (`internal/treepad/batch.go:174`), and the document never says the app passes
`--batch`. With an empty filter, **every** manifest in the repo is reconciled: materialised, linked
(pushed), restacked. A batch a human made by hand, or an abandoned one from last week, gets its
worktrees hard-reset and its open-PR branches pushed every 15 seconds by a background daemon. The
blast radius of running the app is "every treepad batch in the repo", not "the tasks in the app's
table".

**Nothing stops a human launching into a parked chain root — and the app can then never own it.**
Invariant 14's root clause is the app's alone; `readyToLaunch` returns true for any materialised
member with no Activity file, consumed by `tp batch sync --launch`
(`internal/treepad/batch.go:329`) and `tp ui`'s two launch keys (`tui_batch.go:149,170`), and the
workspace's own treepad skill tells agents and humans to prefer `tp`. A parked fan-in root has no
Activity file precisely *because* the app never launched it, so invariant 8 does not help. One
keystroke launches an agent into a worktree branched off a `main` containing neither blocker. Then
the blast radius compounds: the app holds no `running` row, so it will never push, read CI or retry
that work — and §3 step 8 launches only into **empty** worktrees, so once the human's agent commits,
the app's own task can *never* start. When the blockers merge, the row stays in `waiting`, whose
Verbs column is "—". A state with no operation now holds work that is done and wrong.

**Restart, crash and upgrade recovery is asserted, not designed.** Three gaps. The per-agent
goroutine that "waits on the process, records exit" cannot be re-established — a re-attached process
is not a child, so `Wait` is unavailable and the exit code is permanently lost, with no state for
"alive but unwaitable". The artifact test is "commits in the worktree", which cannot distinguish
this attempt's commits from a previous attempt's in the same worktree (§3 step 5 reuses it
deliberately), so a second attempt that produced nothing is misclassified as success and pushed —
invariant 6 holds mechanically and fails semantically. And there is no schema-migration story:
`flock` guarantees one instance, not one *version*, and an upgrade between ticks against an existing
DB is exactly the all-or-nothing release with no rollback, where the state needed to reconstruct the
fleet is in the file being migrated. No runbook exists.

**Shared-resource blast radius lands on the team, not the operator.** `services`: 18 workflow files,
seven firing per PR, ~72 jobs, an environment claimed from a finite pool that is prewarmed on a
30-minute cron for a *planned* set of targets (`pr-stage-prewarm-pool.yml`). Concurrency groups are
per-PR or per-head-ref, so the app will not cancel colleagues' runs — but it will exhaust the pool,
and when it does, **colleagues' PRs are what fail to claim**. `support-app` is the opposite shape and
worse in one respect: six PR workflows and **no concurrency group on any of them**, so nothing is
superseded and every force-push stacks a complete duplicate set. §7's "the cost is paid in CI
minutes" understates this by a category. And `.mergify.yml`'s `common_checks` begins with
`base=main`, so **no stacked descendant PR can ever satisfy the merge protection** — while the
`Merge when ready` rule needs only a label and `#commits-behind=0`, meaning the team's habitual
merge gesture on a descendant squash-merges it into its *parent branch* with none of the required
checks. `common_checks` also requires `verify / Linear issue is linked`, and a failing metadata
check is indistinguishable to §3 step 5 from a failing test — so the app will spend both attempts
re-running an agent against a problem no code change can fix.

**Coexistence with the human.** `tp remove` on a clean worktree with a live agent succeeds and
pulls the ground out from under it; the next tick reports `failed` rather than "the human removed
this". A human `git rebase` in a worktree makes it ahead-and-not-patch-equivalent, which is the one
case that safely yields `RestackStale` — so hand-rebasing is *safer* than leaving the worktree
clean, an inverted incentive. A human committing by hand into a worktree with an open PR gets their
commit pushed by the next tick's `gh stack link`, unreviewed and unannounced. An app-owned lock file
per worktree that treepad's `restack` and `remove` both honour would collapse most of these — and
that is a treepad change.

---

## Positions on the seven items

Positions, not options.

**1 — Is "CI is the only gate" coherent?** *It is coherent as a policy and incoherent as
implemented, and the hole has a precise name: the design treats a remote, asynchronous,
multi-valued, per-commit, partially-required, self-retrying, cancellable signal as a synchronous
boolean, and then takes an irreversible action on it every 15 seconds.* Four consequences, each
independently sufficient: green is not bound to a commit, so an empty or stale rollup reads as
green and un-drafts an untested PR; green does not expire, and on these repos a base change does
not re-trigger CI, so the gate is evaluated against a base that will not be the merge base; red is
not a code fact a large fraction of the time, and on `services` it is a fact CI itself retries; and
green is reachable by deletion with nothing constraining the diff toward the ticket. The first two
are cheap to fix — bind to `head_oid`, require a named set of required checks, treat a base change
as invalidating. The last two are **not** fixable inside "CI is the only gate", because they are
properties of using an external optimisation target with no local constraint. §7's reasoning was
individually plausible on each premise and two of its three premises are false; more importantly it
concluded "therefore no gate" when the available conclusion was "therefore a *weak* gate". A weak
gate that reads the diff is exactly what catches an agent deleting the field it was asked to add.

**On the retry loop specifically.** It can thrash (it races `itest-retry.yml`, and its own push
cancels the run it is reacting to). It burns CI minutes on a shared, capacity-limited resource
rather than the operator's laptop, which is strictly worse than the predecessor's laptop storm
because exhaustion lands on colleagues with no attribution. And it can absolutely produce a worse
branch than it started with, presented as `review me`. Flaky CI: the agent "fixes" the flake,
plausibly by weakening it. Queued 20 minutes: 80 no-op ticks, and under the wrong green reading the
empty window is a false promotion. Huge log: unbounded across 50 shards with no cap, so context
overflow or silent truncation. Uninformative log: CANCELLED, TIMED_OUT, OOM, a docker pull, a pool
exhaustion — all met with another agent run and another ~72-job cycle. **My position: delete the
retry from v1.** Park CI red to `needs you`. That removes the thrash, the injection channel, the
adversarial-green risk and the log-size problem for the cost of one click, and it is the single
biggest simplification available. If retry survives, it must require a *terminal* failure (no
pending re-dispatch, `run_attempt` at ceiling), N consecutive stable red readings, a conclusion not
in {CANCELLED, TIMED_OUT, STARTUP_FAILURE, STALE}, a capped and scrubbed log delivered as a file
rather than inlined, and a refusal to auto-push a retry diff that touches `.github/`, a lockfile or
`package.json`.

**2 — Stacking on open PRs.** *The mechanism is defensible; the stated mitigation is not, and the
design is missing the two states it needs.* Six concrete sequences are traced above with severity
judgements. The mitigation ("`tp` reports `stack-stale`, human handles it") is inadequate because
`restack` compares a branch only against its own upstream and therefore has **no predicate for a
moved base** — the condition §12.4 describes never produces the signal §12.4 relies on. And **yes,
worse than staleness can happen**: uncommitted agent work destroyed by `RestackFastForward` /
`RestackReset` firing on a clean worktree with a live agent (the app's own 15s tick supplies the
trigger), and a structurally wrong post-squash diff reaching `review me` on the team's `main`.
Additionally the descendant gate is *too strict* in the direction nobody checked: a parent that
merges before its descendant launches strands the descendant permanently, because MERGED is not
OPEN and `waiting` has no exit and no verb — and that fires on the *happy path*, whenever review is
faster than the launch. **Fix, minimally: state the descendant gate as `OPEN or MERGED` (matching
treepad); add `stack-stale` and `pr closed unmerged` as states with operations; and stop the app
handing treepad a mutation licence on live worktrees** — either the app skips `tp batch sync` for
repos with running agents, or treepad's `restack` learns a busy-worktree veto (the Activity file is
already the fleet's busy marker; `restack` simply does not read it).

**3 — The seam mechanism with no verification.** *Yes, a seam can silently diverge and waste
consumer work, and the stated `support-app` mitigation does not work — it is inverted.* The
divergence sequences and their discovery points are traced above; the common root cause is that
`hash(composed)` detects changes to the seam while every real failure is a change to reality with
the seam constant. On the mitigation: verified false in three independent ways plus a fourth that is
worse than false — `schema.graphql` is gitignored and absent (nothing to patch), `gen.sh` `rm -f`s
and re-`curl`s it from a server (any patch has a lifetime of one command), CI skips the GraphQL
block entirely (`if [[ -z "${CI}" ]]`), and `graphql-prod-compat.yml` — **required** via
`.mergify.yml:20` — regenerates from **production** and typechecks, so a consumer implementing an
agreed seam field fails a required check deterministically until the producer *deploys*. That also
breaks invariant 13 (green requires deployment, not merge), misnames `refresh against main` (the
referent is `core-api.uk.plain.com`, not `main`), and makes §12.1's "mitigated" set empty. On the
merge-time question: `packages/graphql/types.ts` is tracked, 23,067 lines, no merge driver, with an
*uncommitted* input fetched per-developer; a seam means producer and consumer touch the same types
by definition, so a same-hunk conflict is the expected case, it lands the descendant in a
`stack-stale`-shaped state the app has no room for, and "regenerate" is not a safe resolution
because locally it destroys the patched schema and in CI it does nothing.
**Position: the seam mechanism as specified should not be built.** The honest v1 statement is "no
CI anywhere validates a consumer against a seam, and every cross-repo consumer PR is red until the
producer deploys" — at which point the draft gate and `waiting on seam` are doing all the work and
the seam file is only prompt context, which is fine and much smaller. And the door the document
closed on a false premise should be reopened: `services` has **one committed 15,569-line SDL file**,
so a seam can be a verbatim fragment of a real artifact, `lands_at` has an obvious correct value,
and a producer-side assertion is a grep — no parser, no registry. That is the cheapest verification
in the whole design and §13 deleted it for a reason that is not true.

**4 — Fan-in as chain root.** *The "no `Manifest` schema change is needed" claim is narrowly true
about the TOML and materially unsafe, and `tp batch sync` does something harmful with a chain root
the app is not launching into.* Narrowly true: the Manifest has no edges (`batch/manifest.go:27-37`)
and nothing in `batch/` models blockers, so no field must change for the app to hold edges itself.
Unsafe, for two reasons. First, the harm I traced: `ReadyToMaterialise` passes position 0
**unconditionally** (`batch/ready.go:15`), so `tp batch sync` creates the root's worktree on tick 1
via `git worktree add --no-checkout -b <branch> <path> <base>`
(`internal/treepad/lifecycle/lifecycle.go:67`) — using the **local** `main` ref, and there is **no
fetch of `main` anywhere in `Reconcile`** (`internal/treepad/batch.go:152-190`; `restack` fetches
only `origin/<member's own branch>`, `restack.go:87`). Nothing ever advances that root. So when the
app's gate ("every blocker MERGED") finally passes — hours or days later — it launches an agent into
a worktree cut from a `main` that predates the merges it waited for. **Invariant 14 holds in letter
and is false in substance: a test written from it passes while the agent writes code against a base
missing its own dependency.** This is precisely the failure the predecessor review identified for
fan-in, reintroduced through a different door, and §12.5's "one tick wide" framing does not cover
it — the window runs from tick 1 until the blockers merge. Under invariant 3 the app has no legal
repair (it may not delete the worktree) and under §1 it may not rebase. Second, the collateral: the
root is not inert. `[sync]` copying runs (28 GB of `.sst/` in `services`, ~155k files of
`node_modules/` in `support-app`), `pre_new`/`post_new` hooks fire, the artifact is written, and
`git fetch origin <branch>` fails every 15 seconds forever for a never-pushed branch. It is *not*
pushed and *cannot* get a PR (`LinkArgs` needs ≥2 OPEN-PR members, so the prefix terminates at
index 0), and it does *not* falsely unblock its own descendants (gate (b) keys on the descendant's
own branch). So no push, but a stale base and a human-launchable trap.
**Position: this needs one small upstream change, and §10's "treepad: none" is wrong.** One optional
Manifest field (`hold = true` on a chain, or `blocked = true` on a member) plus one condition in
`ReadyToMaterialise` and one in `readyToLaunch` makes invariant 14 enforceable in the layer that
*acts* — which also closes the human-launch hole. Alternatively the app passes `--batch` and writes
the Manifest incrementally, adding a chain only when its blockers merge; that needs no treepad
change but makes the app a Manifest writer, which `batch/manifest.go:26` explicitly forbids
("never by treepad and never by hand").

**5 — The 14 invariants.** A violating sequence for each, or a statement that I cannot construct
one.

1. *Nothing pushed until a run is dead and has commits.* **Violated.** A retry re-runs an agent on
   a branch that already has an open PR; that branch is in `LinkArgs`' prefix; `tp batch sync` →
   `gh stack link` pushes it within 15 seconds of the live agent's commit. Also violated in spirit
   by the `kill` verb: a killed run is dead with commits, so the human's abort publishes the work.
2. *The app never merges a PR.* **Cannot construct one** in the app's own code path. It is a
   statement about the app, not the system: every `claude -p` run has `gh` on PATH and (verified)
   `Bash(gh pr *)` pre-approved.
3. *Never deletes a worktree except on explicit user action.* **Cannot construct a literal
   violation, which is the problem** — it is stated about the wrong noun. `tp batch sync`'s
   `RestackReset`/`RestackFastForward` destroys work *inside* a worktree with only a racing dirtiness
   check. And "explicit user action" is implemented as an unauthenticated localhost HTTP request, so
   a cross-origin POST satisfies it.
4. *Never `--launch`, never `gh stack init/add/submit/modify/rebase/sync`.* **Holds for direct
   calls, circumvented one level down** — `tp batch sync` calls `gh stack link`, which pushes and
   can create PRs. A denylist over another tool's CLI surface; a new verb escapes silently.
5. *Liveness is `kill(-pgid,0)` plus start-time; no timing rule.* **Violated as a system property.**
   A wedged agent is alive forever and never reaches `needs you`, whose own definition includes "a
   wedged agent" — so §5 names a state the design cannot compute. Also a window: the app spawns,
   then writes the pid; a crash between them leaves a live orphan with no DB row.
6. *Disposition from artifacts, never absence of events.* **Violated semantically.** "Commits in the
   worktree" is not per-run. Attempt 1 commits and is pushed; attempt 2 dies producing nothing; the
   worktree still holds attempt 1's commits, so attempt 2 is classified as success and pushed. The
   design has no per-run baseline. Conversely, an agent that wrote forty files and died before
   committing is classified identically to one that never started.
7. *One app instance per workspace, via flock on the DB path.* **Violated trivially:** a second
   config pointing at the same repos with a different DB path takes a different lock and both loops
   launch into the same worktrees. The lock guards the database, not the resource the app contends
   over.
8. *A worktree holding an Activity file is never launched into.* **Violated by the app itself, four
   ways, by design** — `failed → running`, `refresh against main → running`, and the `re-run` verb
   on `needs you` and `waiting on seam` all launch into a worktree whose Activity file exists from
   the first launch and never expires. The invariant is written as a system property and is really a
   statement about treepad's automatic path only. It is also one slug-algorithm drift away from
   failing open, in the direction of two agents in one worktree.
9. *`waiting on seam` never escalates to `needs you`.* **Violated via the `failed` route** (a
   re-run that dies with no commits goes `failed → running → failed → needs you`), and — more
   importantly — it guarantees the wrong thing: a genuinely broken consumer sits red and silent
   forever, and given `graphql-prod-compat` this is now the *default* state of every cross-repo
   consumer. The stated escalation ("the producer's row shows `needs you`") fails on the case §6
   itself names, an abandoned producer, which has no state at all.
10. *At most two attempts per task before `needs you`.* **Violated**, and indeterminate. Violated by
    the push / `gh pr create` failure loop, which retries every 15 seconds forever with `attempts`
    untouched. Indeterminate because `attempts`'s scope is never defined: shared kills the
    `refresh against main` re-run for any task that ever saw red CI; per-cause makes the real bound
    2 × 4 with no global cap. And a rate-limit blip is a global cause charged to per-task budgets.
11. *No `ANTHROPIC_API_KEY`, no `--bare`, `.env` through a denylist.* **The clause is unenforceable
    and the mechanism does not exist.** treepad does the copying via `[sync] include`, which is an
    **allowlist** (`internal/sync/sync.go:246,272`) with no `!` exclusions in either repo, from a
    config file the app does not own. The credential the invariant guards is not the one that is
    there (the subscription credential is in the macOS keychain, reachable by the same UID), and
    `--bare` is a billing control. Meanwhile `.env` is not gitignored in either repo and *is* in
    both include lists.
12. *HTTP binds `127.0.0.1` only.* **Holds as written and does far less than assumed** — no auth, no
    CSRF token, no `Host`/`Origin` check, in front of `kill` and `remove worktree`, on a machine
    that also runs a browser; DNS rebinding supplies read access to worktree paths and pgids.
13. *A consumer PR is un-drafted only when every seam has a merged producer and its checks are
    green.* **Violated, and no step in §3 performs it** — the eight steps contain the draft decision
    (step 4, reading seam state that step 6 is the thing that updates, i.e. last tick's snapshot)
    and no un-draft action at all. Any reviewer clicking "Ready for review", or an agent running
    `gh pr ready`, falsifies it permanently, because draft state is decided once rather than
    re-derived each tick like everything else in §8. And on `support-app` "checks green" is
    unreachable until the producer *deploys*, so the invariant either never fires or fires on a
    green computed against a different schema.
14. *Descendant launches only on parent OPEN; root with blockers only when all merged.* **Both
    halves fail, in opposite directions.** Too strict: a parent that merges before the descendant
    launches strands it permanently (MERGED ≠ OPEN; `waiting` has no exit, no verb). Too weak: the
    root's *worktree* is materialised unconditionally off an un-fetched local `main` on tick 1, so
    the gate passes while the worktree provably lacks the blockers. Both are consequences of §1's
    false "same predicate" claim.

**6 — The state machine.** Audited above. Summary: every state has an entry. **Not** every state
has an exit — `checking` has none when CI never reports, `review me` is a latch (§3 step 5 only
examines `checking`), and `waiting` has none for a merged parent or a closed blocker. **Reachable
conditions with no state at all**: PR closed unmerged, `stack-stale` (which §12.4's mitigation
depends on), push/`gh pr create` failure, killed run, alive-but-unwaitable after restart. §5's
sentence "Every parked state has an operation" is false for `waiting`, `checking` and `review me`.
On the two re-entries: the `failed → running` loop terminates **if** `attempts` is one enforced
per-task counter, which the doc does not say; the `waiting on seam ⇄ checking` cycle is gated on
producer merge state rather than on an attempt count and nothing bounds re-entry, so it can cycle
indefinitely — and on `support-app` it *will*, because `graphql-prod-compat` cannot go green before
the producer deploys. The `push failed` loop is the clearest unbounded one: every 15 seconds,
forever, with no counter and no escalation.

**7 — Scope.** *This is simultaneously more system than the goal needs and less rigour than it
needs, and it fails the "additive later, not rewrites" test on eight of twelve deferrals.* Too much:
the seam mechanism as specified (three jobs, a hash-drift detector that points the wrong way, a
`lands_at` pointer nobody owns, and a `refresh against main` state) is the largest single piece of
machinery in the design and it does not work in the workspace it was designed for; the retry loop is
the second largest and should be deleted; stacking on open PRs buys throughput the review bottleneck
will not cash and costs six failure sequences, one of which loses work. Too little: no local gate of
any kind in front of pushes that trigger OIDC-federated AWS deploys; no definition of the one signal
everything depends on; no `stack-stale` state; no error handling on any subprocess; no diff check; no
retention rule; no schema migration; no runbook. On additivity, eight of twelve deferrals touch the
loop, the schema, the state machine or an invariant (audited above), and two of them — automatic
rebase of stale descendants, and the local verify — have triggers the document *itself* predicts
will fire immediately (§12.3 calls `stack-stale` "the common case"; §7's own CI-latency numbers on
`services` are 30–60+ minutes). A deferral whose trigger the author expects to fire on day one is
not a deferral; it is missing scope. **The design that meets the stated goal** — days not months,
real value day one, clean layers, additive omissions — is smaller than this one: one repo at a time,
no stacking (base everything on a freshly fetched `main`), no seams beyond "paste this file into the
prompt", no retry, a 20-line pre-push path denylist, CI read only to label a row, and one localhost
page. Every cut above is genuinely additive onto *that* substrate in a way that several are not onto
this one.

---

## Cross-cutting themes

1. **Premises, not reasoning, are where this design fails — and the citation habit disguised it.**
   Line numbers are accurate almost everywhere; four premises are false, and each carries a chapter:
   `.treepad.toml` is committed (→ five deletions in §13), `services` has no producer file
   (→ decisions 14/18/20 deleted), `support-app`'s CI validates against a repo-local schema (→ §6's
   whole mitigation and §12.1's "mitigated" set), and the app and treepad hold the same predicate
   (→ §1's clean-layers argument and invariant 14). Four lenses landed on these independently. The
   process fix is structural: §10's assertions should be stated in a form a test can express, and
   the app's own suite should carry them as golden checks against `tp batch sync --json`.
2. **"Ready" is one decision with four homes, and the no-import rule is what forces that.** Named by
   Simplicity, Maintainability, Modularity and Extensibility. `batch/` exports precisely the pure
   predicates *so they can be reused*, and forbidding the import buys a boundary that is already
   test-enforced while leaving every untyped coupling (JSON shape, Manifest TOML, `pr-cache.json`
   semantics, the `internal/slug` Activity path) in place. The fix is dependency inversion — consume
   treepad's verdict — not vendoring.
3. **The app's own tick is the most dangerous actor in the system.** Every 15 seconds it invokes a
   subprocess that hard-resets or fast-forwards worktrees with live agents in them, pushes those
   agents' in-progress commits via `gh stack link`, reconciles every batch in the repo including ones
   the app knows nothing about, and fires ~72 CI jobs plus an AWS deploy per push on a shared,
   capacity-limited resource. Performance, Deployability, Security and Simplicity all converged here.
   The app delegated mechanism to `tp` and thereby delegated *authority* it cannot restrain.
4. **Every parked state is a place work goes to die, and the invariants guarantee it.** Invariant 3
   forbids deletion, invariant 9 forbids escalation, `waiting`/`checking`/`review me` have no exits
   for their real failure conditions, and `tp remove` cannot remove the squash-merged case §12.3 says
   is common. The design has excellent discipline about not destroying things and no discipline about
   finishing them.
5. **The design's confidence is inversely correlated with its correctness.** The three places it
   argues hardest — §1's same-predicate claim, §6's mitigation, §7's no-local-gate case — are the
   three places it is wrong. The places it hedges (§12's accepted limits) are largely sound. That is
   a signal about how the document was produced, and the remedy is adversarial verification of
   premises before any more design work.

---

## Prioritised recommendations

Resolve 1–5 before writing code; 6–9 before the corresponding component is built.

1. **Re-verify every premise, then rewrite §6, §7, §12.1 and §13 from true facts.** *Buys:* a
   document whose confidence is earned. The four corrections change the design, not just the prose:
   `.treepad.toml` is ignored (so a local gate is configurable and leaks nothing); `services` has one
   committed SDL producer file (so seams can be verbatim fragments of a real artifact and a
   producer-side assertion is a grep); `support-app`'s required `graphql-prod-compat` check makes
   every seam consumer red until the producer *deploys* (so §6's mitigation is inverted and invariant
   13's gate is on deployment, not merge); and `/implement` already runs the full suite locally (so
   §7's docker argument describes what already happens, uncapped and unobserved). *Cost of leaving
   it:* five deletions and a whole mechanism rest on falsehoods. *Next step:* a one-page correction
   note listing each premise, the verified fact, and which decisions it reopens.
2. **Fix the readiness divergence and the fan-in root base.** *Buys:* invariant 14 becomes true in
   substance and testable. State the descendant gate as `OPEN or MERGED` (matching
   `ReadyToMaterialise`, and closing the permanent-strand-on-merge deadlock). Then close the root
   hole: one optional Manifest field (`hold`/`blocked`) plus one condition in `ReadyToMaterialise`
   and one in `readyToLaunch`, so the gate lives in the layer that acts and a human's `tp ui` launch
   key cannot bypass it. *Cost of leaving it:* the fan-in case the multi-repo design exists to serve
   runs agents in worktrees missing their declared dependencies, indefinitely, and the happy path
   deadlocks. *Next step:* a small treepad ticket; delete "treepad: none" from §10.
3. **Stop the app's tick handing treepad a mutation licence on live worktrees, and stop `link`
   pushing live work.** *Buys:* removes the only work-loss sequence and repairs invariant 1. Either
   the app skips `tp batch sync` for repos with running agents, or treepad's `restack` gains a
   busy-worktree veto reading the Activity file it already has. Pass `--batch` explicitly so the app
   never reconciles batches it does not own. And gate `link` so a branch whose worktree has a live
   agent is never in the prefix. *Cost of leaving it:* uncommitted agent work destroyed silently, at
   a race evaluated ~1,000 times an hour, plus unreviewed WIP pushed to team repos triggering AWS
   deploys. *Next step:* decide which side owns the veto and record it in §1's table.
4. **Delete the retry loop from v1; park CI red to `needs you`.** *Buys:* removes the thrash against
   `itest-retry.yml`, the untrusted-log injection channel, the adversarial-green risk, the
   unbounded-log problem, and the double-spend on a shared CI pool — for the cost of one human click.
   *Cost of leaving it:* the loop can burn ~144 jobs and two AWS stage deploys per task to produce a
   branch worse than the one it started with, labelled `review me`. *Next step:* rewrite §3 step 5 as
   "checks red → `needs you`, log shown on the row"; keep the log *display*, drop the log *feedback*.
5. **Define the CI gate, and add the missing states.** *Buys:* the one gate in the system stops being
   a race. Define green as `(head_oid matches the pushed SHA) ∧ (every configured required check is
   present ∧ COMPLETED ∧ SUCCESS)`; give `pending` and `unknown` distinct behaviour; put a deadline
   on `checking`; treat a base change as invalidating green. Add states with operations for
   `stack-stale`, `pr closed unmerged`, `push failed`, and a `killed` disposition distinct from "dead
   with commits". Name `attempts` as one per-task counter that manual `re-run` resets and that
   infrastructure conclusions do not charge. *Cost of leaving it:* an empty or rate-limited rollup
   promotes the fleet to `review me` on zero evidence, and three parked conditions have no home.
   *Next step:* rewrite §5's table with an explicit trigger column and re-derive invariants 10 and 13
   from it.
6. **Add the 20-line pre-push path denylist, and the security floor.** *Buys:* the only thing between
   "an agent wrote something odd" and "something odd ran with `id-token: write`" in the company AWS
   account. Refuse to push a diff touching `.github/`, `infrastructure/`, lockfiles, `package.json`,
   `pnpm-workspace.yaml` or `.env*`. Plus: `Host` and CSRF-token checks on the HTTP surface; `.env*`
   into `~/.gitignore-plain`; invariant 11 restated as a startup precondition the app validates (or
   dropped); a narrowly-scoped `GH_TOKEN` and a launched-run settings file denying `gh auth`,
   `gh api` and `curl`; `0700` and a retention rule on `plain/.claude/`. *Cost of leaving it:* a
   same-repo PR from an unread branch is arbitrary code execution with the team's credentials, and
   invariant 11 asserts a mechanism that does not exist. *Next step:* add these as numbered
   invariants that are testable *from the app*.
7. **Consume treepad's verdict instead of re-deriving it, and make the JSON contract explicit.**
   *Buys:* deletes the second implementation of readiness and the `internal/slug` reimplementation.
   Read `Action`, `Base`, `Position`, `PRState`, **`PRStale`**, `StackStale`, `Removable`,
   `WorktreePath` from the report; add the invariant "the app never launches into a member whose
   entry is not `created`/`skipped`"; ask treepad to expose `activity_file` and to promote the report
   type (or pin a golden-JSON contract test). Use treepad's PR read rather than a second `gh pr list`
   — two snapshots per tick is a window nobody named. *Cost of leaving it:* four definitions of
   "ready", a slug-drift that fails open into two-agents-one-worktree, and decisions made on cached
   PR state read as fresh. *Next step:* one treepad ticket; one golden-file test in the app.
8. **Cut the seam mechanism to prompt context plus the draft gate, and reopen producer-side
   assertion.** *Buys:* removes the largest piece of non-working machinery. Drop `hash(composed)`
   drift detection (it points the wrong way and, once sourced from `main`, is permanently on), drop
   `lands_at` as an app-read pointer (nothing owns it, nothing checks it), and state the honest limit:
   no CI validates a consumer against a seam, and cross-repo consumers are red until the producer
   deploys. Then, given `services/packages/core-graphql/src/schema.ts`, add the cheap thing the false
   premise removed: a producer-side check that the merged file contains the seam's declarations.
   *Cost of leaving it:* the flagship use case parks red and silent by invariant, and a seam can
   diverge with the page reporting "correct and current". *Next step:* rewrite §6 around two jobs, and
   move seam edges into the task table where `blocked_by[]` already lives.
9. **Specify the operational basics.** *Buys:* the design becomes runnable by someone other than its
   author. Error handling on every subprocess (distinguish "no PRs" from "the call failed"); a per-run
   commit baseline so artifacts are attributable to an attempt; run the `tp` call off the reconcile
   goroutine with at most one in flight per repo and tick-drop on overrun; a DB schema version with
   refuse-to-start-on-mismatch; `tp remove --force` upstream so the button works on the squash-merged
   case; provenance in every PR the app opens; a fan-out rule in `to-tickets` ("a chain is a maximal
   path in which each node has exactly one blocker **and** its blocker has exactly one dependent");
   and a written stop-the-fleet runbook. *Cost of leaving it:* the loop blocks on 28 GB of `[sync]`
   copying while agents go unreaped, and the first upgrade is an all-or-nothing release over the only
   file that could reconstruct the fleet.

**Trim from v1 beyond §11's existing cuts:** stacking (base everything on a freshly fetched `main`;
reintroduce it by switching one predicate once the failure sequences have states); the retry loop;
`hash(composed)` drift detection; `lands_at`; `refresh against main`; and the second repo, until the
first one works end to end.
