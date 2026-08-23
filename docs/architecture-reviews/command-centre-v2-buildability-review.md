# Command Centre revision 2 — buildability review

_Date: 2026-08-20. Target: `docs/command-centre-v1.md` revision 2. Read-only; no source file
was modified. Verified against treepad source, `/Users/ollymarsters/Documents/plain/services`,
`/Users/ollymarsters/Documents/plain/support-app`, and the installed skills trees. §14 was read
first; nothing it already corrected is re-reported, but each fix was checked for new defects and
three of them introduced one._

---

## Verdict — BUILD IT AFTER FIXING 6 THINGS

**The primitives are right.** The level-triggered tick, the task table as scheduling truth with
an audit log beside it, pid liveness with no timing rule, artifact-first classification of a dead
run, file-redirected agent output, never merging, and the `tp` subprocess boundary all survive
this review — none of them is the thing that would have to be rewritten. Revision 2's own
corrections are real corrections: the OPEN-or-MERGED gate, the per-run baseline SHA, the
`head_oid` binding, the push denylist, the four added states, and dropping the CI retry each
close a genuine defect. What is wrong is a layer below the primitives and above the code: six
specific *mechanisms* that are stated with confidence and do not work as written. Three of them
were introduced by revision 2's fixes. Each is cheap to fix now — mostly one or two lines of
spec, one of them a third treepad change — and each becomes expensive after two weeks of code,
because each one determines a database column, a config schema, an ownership boundary, or an
upstream contract, not a function body. In particular **the same failure mode that sank revisions
1 and 2 is still present in revision 2**: this document contains at least three claims whose
citations are line-accurate and whose premises are false, and two of them were used to *delete*
something (an invariant, and a verification step). The document is close. Fix the six, then
build; do not treat them as backlog.

The six, in the order they must be settled:

1. **The CI verdict is wrong in three ways** (§8, §3.6, inv. 11/14): its config cannot represent
   the predicate it claims to copy, two of the four check names given for `services` are wrong,
   and it has no expiry — a stacked descendant's green survives a base change that re-runs nothing.
   This is the only gate left in a design that deliberately removed the others.
2. **The Activity file's ownership model is under-determined and inverted** (§10.1, inv. 6/9).
   It is the fleet's mutual-exclusion primitive; the design has the app deleting it, which
   re-arms the very double-launch it was settled to prevent.
3. **The stale-root fix is a no-op** (§3.10, §4). `git fetch origin` followed by
   `tp new --base main` produces the same commit it would have without the fetch.
4. **The app never scopes its `tp` calls to its own batch.** That breaks invariant 2 outright
   (unowned branches get pushed with no denylist check) and makes teardown loop forever. Two
   symptoms, one decision.
5. **The DAG contract with `to-tickets` is a behaviour reversal on an installed skill, and it
   omits fan-out**, which has the identical defect and is the more common plan shape.
6. **`waiting on producer deploy` has no working verb**, so the flagship cross-repo state is a
   sink that drains to `needs you` under invariant 15.

None of these six is a rewrite of the tick, the task table, or the state machine. That is why the
verdict is not DO NOT BUILD YET.

---

## Claims found WRONG or STALE

Line numbers in this document are, as before, almost all exact. The failures are semantic, and
every one of them sits under a sentence that *decides* something.

| § | Claim | Correct fact |
|---|---|---|
| §8 | "`required_checks` and `compat_check` are the only per-repo values, and both come straight out of `.mergify.yml`." Config gives `services` and `support-app` the same four names: `["Lint","Typecheck","Tests","Generated files"]` | **They do not come out of `.mergify.yml`, and for `services` they are wrong.** `services/.mergify.yml:59-61` names `Lint`, `Typecheck`, **`Unit Test`** (not "Tests"); `:39` names **`Generated files up-to-date`** (not "Generated files"). The doc's list also omits six required conditions: `Integration Tests Passed` (`:43`), `Local Integration Tests Passed` (`:47`), the deploy disjunction (`:31-35`), `Lint GitHub Actions / Lint` (`:52-57`), the Linear check (`:78-80`), and `base=main` (`:12`). `support-app`'s four names *do* match (`support-app/.mergify.yml:16,20,23-25`) but its list omits the Linear check (`:38-40`). More structurally: the real predicate is **a boolean expression**, not a flat name list — `or` branches, an `and` with `check-skipped = Deploy`, and a clause where an **absent** check is legitimately passing (`services/.mergify.yml:51-57`, a path-filtered workflow). A `[]string` cannot express any of that. Consequence in both directions: a misnamed check is never present, and inv. 11 says absent is never green, so **every `services` row parks in `checking` and then walks to `needs you`** — the app never reports `review me` on the flagship repo; while on `support-app` the omitted Linear check makes `review me` a lie the user discovers at the merge button. |
| §13.3, §14 | "`[sync] include` … is an allowlist in a config file the app does not own (`internal/sync/sync.go`), so **there is no denylist for the app to apply**." Used in §14 to justify *dropping* the `.env` invariant | **False. `!` exclusions are supported.** `internal/sync/sync.go:56-59` documents "`!` prefix negates a pattern"; `parsePatterns` (`:245-254`) splits include/exclude on the `!` prefix and `matchesInclude` (`:271-277`) applies it. So a denylist mechanism exists. And the "does not own" half contradicts §12's own row, which calls `.treepad.toml` "a ready-made private per-repo home for a `[verify]` key the app reads directly" — it is gitignored and untracked (§14 row 1), i.e. private and local. Also `.env` is not gitignored in **either** repo (`git check-ignore -v .env` → rc=1 in both), not just `support-app`; and `.env` is listed explicitly at `services/.treepad.toml:5` and `support-app/.treepad.toml:6`, so the cheapest fix is deleting one line from a private file, not a denylist at all. **This is a fifth false premise, and like the four in §14 it was used to delete something.** |
| §3.10, §4 | "immediately before launching such a root, the app removes and recreates the worktree off a freshly fetched `main`": `tp remove`, `git fetch origin`, `tp new <branch> --base main` | **The fix is a no-op.** `git fetch origin` updates `refs/remotes/origin/main`; it does not move the local `main` ref. `tp new --base main` resolves the **local** `main`, which is exactly the ref `git worktree add` already used (`internal/treepad/lifecycle/lifecycle.go:69`). The recreated worktree is cut from the identical commit. The base must be `origin/main` (or the fetch replaced by a `main` update) — and the same correction applies to the manifest `base` that `batch/manifest.go:23` defaults to `"main"`, which is what §3.1's fetch was introduced to make load-bearing. As written, §3.1 fetches and nothing reads the result. |
| §10.2, §13.7 | `tp remove --force` is the teardown mechanism; "the app passes `--force` only when it holds MERGED PR state" | The `--force` path exists as claimed, but **the mechanism does not terminate.** `materialise` skips by *branch existence* (`internal/treepad/batch.go:213,239-243`), so once `--force` has run `git branch -D`, the next `tp batch sync` re-creates the worktree from `m.Base` — and it is immediately `readyToLaunch` (`:276-282`, Activity file also deleted per inv. 6). For a chain root that is local `main`; for a descendant whose parent branch was deleted post-squash it is `ActionError` forever. Removing a merged member and continuing to call `tp batch sync` are mutually exclusive as specified. This needs a third treepad change (member/manifest retirement that `materialise` honours) or a per-batch call plus manifest deletion. §10's "treepad — two, both small" is understated for the same reason revision 1's "none" was. |
| §10 (`to-tickets` row), §4 | "**A fan-in ticket must be a chain root, never mid-chain**" — presented as an emit-fields change | **It is a reversal of a shipped, documented, deliberate rule, and it leaves the worse case standing.** `~/.agents/skills/to-tickets/references/treepad-manifest.md:61-68` instructs the opposite: "Put it at the end of the chain holding its longest blocker path, and tell the user which dependency the Manifest drops." And **fan-out has the identical defect and is unmentioned**: rule `:47` ("blocked by exactly one other joins that blocker's chain, directly after it") is satisfied by *both* of two slices blocked by the same single blocker, so both land in one chain and `batch.Resolve` bases the second on the first (`batch/resolve.go:52`) — an agent working in a worktree containing code it does not depend on, and independent tickets serialised, which is exactly what rule `:49` exists to prevent. Fan-out is the more common plan shape. A fan-in-only fix does not touch it. |
| §8 | "Intake is the manifest directory plus the task table — **both already exist**, and `batch.Load` already reads the former." | Half true. The manifest directory and `batch.Load` exist (`batch/manifest.go:64-72`). **The task table does not exist anywhere** — treepad has no database: no `database/sql`, sqlite, or bbolt import, no DDL, no migrations. It is a proposed artifact of the unbuilt app, which is fine, but it is not intake that "already exists". Relatedly, `Chain` is the whole type — `Tickets []string` (`batch/manifest.go:35-37`) — and the to-tickets manifest schema (`treepad-manifest.md:30-35`) has no `repo`, `seams`, or `blocked_by` field, so those three live only in the app's own store, which is consistent with settled decision 23 but means the two intake halves share exactly one join key (the ticket ref) that nothing validates. |
| §3.3 | "`pr_stale` from the sync report is read explicitly; its zero value must not be taken to mean 'no PR exists'." | Correct in intent, **unimplementable as stated.** The field is `PRStale bool \`json:"pr_stale,omitempty"\`` (`internal/treepad/batch.go:120`), so `false` is *absent from the wire* and the app cannot distinguish "fresh, no PR" from "field not emitted". Worse, `ReportEntry` lives in `internal/treepad` (`:113`), so the type is unimportable by any external repo, every field is `omitempty`, and there is no version. The design's most important cross-repo contract is a private struct's JSON shape that must be hand-mirrored. Fixable upstream in a few lines (drop `omitempty` on the two booleans the app reads, or promote the type). |
| §2 | Data flow: "agent runs `/implement` → runs the repo's tests → commits → exits" | Incomplete. `~/.agents/skills/implement/SKILL.md` is 16 lines and its end-of-run order is tests (`:11`) → **`/code-review`** (`:13`) → commit (`:15`), and `code-review` fans out parallel Standards/Spec sub-agents. That is real wall-clock and token cost inside every run, against the one wall §13.11 names, and the tick's elapsed-time display is the only thing the user has to interpret it. The load-bearing half of the claim **does hold**: `git push` and `gh pr create` appear nowhere in `implement`, `code-review`, or `tdd`, so the app genuinely owns pushing. |
| §11 | "`tp batch sync` … has no flag to select steps … (`docs/commands.md:70-76`)" | The behaviour is right, the citation is wrong: `docs/commands.md:70-76` is `tp new`'s flag table. `tp batch sync` is documented at `:172-188`, and its flags are `--json --dry-run --batch --offline --launch` — confirming no step selection. |
| §10.1, §11 | "`Reconcile` runs both unconditionally on every tick with no liveness input" | Overstated in two ways that matter to the fix's design. `link` and `restack` are gated on `in.DryRun`, and the **launch** step already reads the Activity file (`readyToLaunch` → `launcher.Exists`, `internal/treepad/batch.go:276-282`). So treepad *does* have a liveness input; it is existence-based and only the launch step consults it. That is the hook §10.1 should extend, and it is why the semantics question in finding 2 below has to be answered before the change is written. |
| §12 (chain depth row) | "No threshold is asserted" | A threshold is already shipped: `treepad-manifest.md:56` — "If the graph produces a chain deeper than about four, flag it to the user rather than writing it silently." |
| §3.6, inv. 14 | "CI verdict … re-derived every tick, never latched", bound to a rollup whose `head_oid` matches the pushed tip | **The `head_oid` binding does not catch a base change, and on these repos a base change re-runs nothing.** Re-parenting a stacked descendant fires `pull_request: edited`, and `edited` is listened for by exactly one workflow in each repo — the Linear check (`services/.github/workflows/linear-issue.yml:4-5`, `support-app/.github/workflows/linear-issue.yml:4-5`). Every test workflow is `[opened, reopened, synchronize]` (`services/itest.yml:5-6`, `local-itest.yml:5-6`, `pr-stage-creation.yml:5-6`; `support-app`'s five are bare `on: pull_request`). So the descendant's head SHA is unchanged, its old check-runs stay attached and green, and the tick re-derives the same green forever. `review me` on a descendant means "passed CI against a base that no longer exists" — and re-deriving every tick cannot detect it, because nothing the app reads changed. This is the sharpest cost of the settled stacking decision and §13.4 understates it: the stated trade was "a fix pushed to the parent leaves descendants on a stale base", which is true; the unstated half is that the app will label that stale base green. Fix is one predicate, and the data is already in §3.3's call: record `baseRefName` plus the base's SHA with the verdict, and treat either changing as invalidating. |
| §3.6, §14 | "There is no retry. Revision 1 re-ran the agent with the failing log; that races the repo's own check retries" | The reasoning is right and the *conclusion* is right; the consequence for the surviving branch is unhandled. `services/.github/workflows/itest-retry.yml:10-31` reruns failed itest shards on `workflow_run: completed` while `conclusion == 'failure' && run_attempt < 4`. So on `services` a red rollup is **transient by design, up to three times**, and §3.6's "anything else red → `needs you`" fires on a red that self-heals minutes later — parking a row for human attention that needed none, which is the failure direction the design is otherwise careful about. `itest.yml:576` also notes `gh run rerun --failed` creates a *second* check-run, so check-run identity is not stable across attempts and "the check named X is red" is ambiguous mid-retry. `support-app` has no retry workflow. Needs a settle rule — red is only red when no attempt is pending — not a retry. |
| §13.3 | "`.env` is copied wholesale into every worktree … `.env` is not gitignored in `support-app`" | Directionally right, aimed at the wrong file. **Neither repo has a root `.env`**, so the `".env"` entry at `services/.treepad.toml:5` and `support-app/.treepad.toml:5` is a no-op today. The untracked secret-bearing file that *does* exist is `support-app/.env.development.local` (ignored via `support-app/.gitignore:73`), and it is **not** in the sync list — so the accepted limit describes an exposure that does not exist, while a fresh worktree silently boots without the local override that does. `support-app/.npmrc` is tracked and holds a registry token. |
| §13.9 | "Mergify's merge queue batches up to four PRs. Its interaction with stacked PRs is unexamined by this design." | Accurate as far as it goes, and understated in one specific way worth knowing before betting on stacking: `base=main` is the *first* merge condition in both repos (`services/.mergify.yml:12`, `support-app/.mergify.yml:12`), so **no stacked descendant PR can satisfy the team's merge protections at all** while its base is its parent's branch. Stacking's payoff is therefore realised only by strict bottom-up merging, one level at a time. Not a defect in the design — a fact that bounds what stacking buys. |

Genuinely verified and holding: `LinkArgs` prefix semantics and the "do not widen this filter"
comment (`batch/link.go:3-31`) · `ReadyToMaterialise` OPEN-or-MERGED, position 0 unconditional,
plus a third `existing[m.Branch]` clause the doc does not mention but does not depend on
(`batch/ready.go:12-26`) · `RestackFastForward` on a clean, behind worktree (`batch/restack.go:32-34`)
· `restack` compares a branch only against its own upstream, so §13.5 is right that there is no
moved-base predicate · `retire` flag-only · `MergedBranches` ancestry-based · `merge_method: squash`
and `batch_size: 4` (`services/.mergify.yml:124-125`, `support-app/.mergify.yml:85-86`) ·
`graphql-prod-compat` required (`support-app/.mergify.yml:20`) and the whole §6 consumer story ·
`schema.ts` as a single tracked SDL file · `-draft` as a merge condition in both repos, so the
draft gate in §6.2 composes correctly with the existing merge policy · `Close stale PRs after 2
weeks` (`services/.mergify.yml:184`), which makes the new `pr closed unmerged` state a certainty
rather than a precaution.

---

## The three structural findings

### 1 — The only gate in the system is configured from a premise that does not hold, and it never expires. WRONG IDEA (patch-sized fixes, but they must precede code)

Everything downstream of a push turns on §3.6's verdict, and §8 gives it a `[]string` of check
names asserted to come "straight out of `.mergify.yml`". They do not (table above). Two failures
follow, in opposite directions, and both are silent:

- On `services`, two of four names do not exist as checks. Inv. 11 correctly refuses to read an
  absent check as green, so those rows never reach `review me`; §5's bounded wait then sends them
  to `needs you`. The app's flagship repo produces no green rows at all, and the symptom looks
  like flaky CI rather than a typo.
- On `support-app`, the omitted `verify / Linear issue is linked` check means `review me` can be
  reported while Mergify still blocks — and it is a check no agent re-run can fix.

The deeper point is that `.mergify.yml`'s predicate is a boolean expression including a clause
where an **absent** path-filtered check is passing (`services/.mergify.yml:51-57`). That directly
contradicts inv. 11's "an empty, absent or stale rollup is never green", which is otherwise the
right rule. So the config schema is not merely mis-populated; it cannot represent the target.

And the verdict has a second half the design does not have at all: **when does a green expire?**
Invariant 14 says the verdict is re-derived every tick and never latched, which is right — but
re-derivation only helps if something the app reads changes. Two cases where nothing does:

- **A base change.** Re-parenting a stacked descendant fires `pull_request: edited`, and no test
  workflow in either repo listens for it (only `linear-issue.yml:4-5` does). The head SHA is
  unchanged, so the old check-runs stay attached and green and `head_oid` still matches. The tick
  re-derives the identical green forever, and `review me` on a descendant means "passed CI against
  a base that no longer exists". This is the sharpest cost of the settled stacking decision, and
  §13.4 states only half of it — the stale base is admitted, the *false green over* it is not.
- **A red that is not yet red.** `services` reruns failed itest shards up to three times
  automatically (`itest-retry.yml:10-31`), so a red rollup there is transient by design, and
  §3.6's "anything else red → `needs you`" parks rows for human attention that needed none.

Both are one predicate each, and both need data the design already fetches or nearly does:
invalidate a verdict when `baseRefName` or the base SHA changed since it was computed; treat red as
red only when no attempt is pending. Note also that `#approved-reviews-by>=1` is a Mergify
*condition*, not a named check (`services/.mergify.yml:97`, `support-app:57`), and CODEOWNERS is
0 bytes in both repos — so a name-list model structurally cannot see the review requirement. That
is fine (the human is the reviewer) but it bounds what `review me` can ever mean.

Why before code, not after: this decides whether the CI verdict is a pure function in its own
package — taking the rollup, the required-check predicate, the pushed tip and the base as data —
or an `if` ladder inside `loop.go`. Get that wrong and every later widening (the compat check, the
skipped-deploy disjunction, base invalidation, a second repo) edits the tick. Get it right and
every finding in this section is a table test.

Cheapest direction, in ladder order: do not model Mergify's expression. Ask an authority.
`gh pr view --json mergeStateStatus,statusCheckRollup` gives GitHub's own verdict over branch
protection in one field, and Mergify reports through check-runs (`merge_protections_settings:
reporting_method: check-runs`, `services/.mergify.yml:198-199`), i.e. the answer is already a
check on the PR. Worth one afternoon's spike before hand-copying eleven check names into TOML.
If the spike fails, the fallback is the same list plus a `passes_when_absent` flag — still data,
still in a pure package.

### 2 — The fleet's mutual-exclusion primitive has two owners with two meanings, and the app's half of it re-arms the failure it was settled to prevent. WRONG IDEA (ownership decision, must precede code)

The Activity file is what makes the 15-second tick legal (§10.1) and what stops two agents in one
worktree. In treepad it means two different things in two places:

- **Existence** is the double-launch guard: `readyToLaunch` → `launcher.Exists`, a bare `os.Stat`
  (`internal/treepad/batch.go:280`, `internal/launcher/activity.go:27-30`).
- **Mtime** is the run state: `launcher.State` returns `working` inside a 90-second window and
  `idle` outside it (`internal/launcher/activity.go:22,34-45`).

Three consequences the design does not address:

1. §10.1 says `link` and `restack` must "skip any member whose worktree holds a **live** Activity
   file". "Live" is not defined, and the two available readings differ. On the mtime reading — the
   one treepad uses everywhere it reasons about a run — the veto **expires 90 seconds into every
   run**, because §8 redirects agent stdout to `plain/.claude/runs/<run-id>.jsonl`, so nothing
   ever touches the Activity file after spawn and its mtime is frozen at launch. The safety
   property that makes the entire tick legal would then hold for 90 seconds of a 40-minute run.
   On the existence reading it works, but then `tp ui` and `tp doctor` still show every app-driven
   worktree as `idle`, and the veto and the run state disagree by construction.
2. Invariant 6 has the app **delete** the file when the process is found dead. That deletion
   re-arms treepad's double-launch guard. Settled decision 10 says the app writes the file
   precisely "so a hand-run `tp batch sync --launch` cannot double-launch into a busy worktree" —
   and after a dead run with commits (a row in `checking`, `review me`, or `needs you`, with a
   worktree full of work), one keystroke in `tp ui` (`tui_batch.go:132-170`) launches a second
   agent into it. The app holds no `running` row for that agent, so it will never push it, read
   its CI, or reap it. `tp ui` also runs its own `Reconcile` on a 60-second timer
   (`tui_batch.go:19`), so a second writer to the same worktrees is one open terminal away.
3. The path itself is computed by `launcher.ActivityPath` over `internal/slug`, both under
   `internal/`, hence unimportable, and documented nowhere. The app must reimplement the slug fold
   with no shared vector. Drift **fails open**, in the direction of two agents in one worktree.

This is the finding I would least want to discover in week three, because it is not a bug in a
function — it is a question about which process owns a lock, and the answer changes invariants 6,
7 and 9, the §10 treepad change, and whether `tp ui` is safe to have open. Decide it once, now.
The cheap resolution is one sentence in each direction: treepad's veto reads **existence** (not
mtime), the app never deletes the file while a worktree holds uncommitted or unpushed work, and
treepad exposes `activity_file` on `ReportEntry` so the app stops reimplementing the path. That is
still "two, both small" — it just changes what the second one is.

### 3 — The intake contract is a behaviour reversal on an installed skill, and it fixes the rarer of two identical defects. WRONG IDEA about scope; UNFINISHED as specified

§4's fan-in rule is correct and important: `Chain.Tickets` is `[]string` (`batch/manifest.go:36`),
strictly linear, and `Resolve` bases each member on the previous one (`batch/resolve.go:52`), so a
fan-in node appended to one blocker's chain sits in a worktree missing the other blocker's code.
But §10 presents the change as "emit `repo`, `blocked_by[]`, `seams[]`", when the actual ask is to
**reverse** `treepad-manifest.md:61-68`, which today instructs the opposite and tells the user
which dependency it dropped. A behaviour reversal on a shipped skill with its own documented
rationale is a different-sized task than adding three fields, and §4 does not say what happens to
the fan-in node's own descendants.

And the same file's rule `:47` produces the fan-out version of exactly this bug: two slices
blocked by the same single blocker both "join that blocker's chain, directly after it", so both
land in one chain, the second based on the first. Two harms at once — an agent working in a
worktree containing code it does not depend on, and two independent tickets serialised behind one
review, which rule `:49` exists to forbid. Fan-out is the more common plan shape than fan-in. A
fan-in-only fix leaves it standing.

One rule covers both and replaces `:47`, `:49` and `:61-68`: **a chain is a maximal path in which
each node has exactly one blocker and its blocker has exactly one dependent.** Everything else is
a chain root with base `main` (`origin/main`, per finding 3 in the table), gated on all blockers
merged. Write that once, in the skill, before the app's task table encodes a different assumption.

Third item in the same contract, cheap and worth closing now: `DeriveBranch` is prefix + slug of
the ref (`batch/resolve.go:84`), and `batch.Load` unions every `*.toml` (`manifest.go:64-73`) with
no cross-manifest uniqueness check. The same ticket in two chains yields one branch with two
different bases; whichever materialises first wins and the other is silently `ActionSkipped`. One
uniqueness assertion at intake, or one in `batch.Load`.

---

## Three more that must precede code

**4 — The stale-root fix is a no-op** (table, row 3). `git fetch origin` + `tp new --base main`
= the same commit. The fix is `origin/main`, and it is the *only* justification offered for
relaxing invariant 4 to permit deleting a worktree — so as written, revision 2 traded an
invariant for nothing. Two words in §3.10, plus deciding whether the manifest's `base` should be
`origin/main` so the whole remove-and-recreate dance disappears. UNFINISHED IDEA.

**5 — The app's `tp` calls are unscoped, which breaks invariant 2 and makes teardown loop.** One
decision, two symptoms.

*Symptom A — invariant 2 is false as written.* `Reconcile` filters manifests only when a batch
name is supplied (`internal/treepad/batch.go:172`), and the design never says the app passes
`--batch`. So the report that flows into `link` (`:341-354`, over `chainsOf(report.Members)`)
covers **every** manifest in `.git/treepad/batches/`, including hand-made ones, and §13.8 concedes
`gh stack link` pushes every branch it is given. Branches with no task row, no baseline SHA and no
denylist check are therefore pushed by the app's own tick — each one ~72 jobs and an OIDC AWS
stage deploy on `services`. Invariant 2 says "no diff touching `.github/`, a lockfile or
`package.json` is **ever** pushed"; §7 calls that "the only push whose consequences are not
confined to a branch". Both are load-bearing and both are false until the flag is passed. Fix: one
flag, plus rewording invariant 2 to bound it to pushes the app initiates.

*Symptom B — teardown does not terminate* (table, row 4). `materialise` skips by branch existence,
so every member removed by `tp remove --force` is recreated on the next tick and is immediately
launchable. This is the one open item the alignment handoff left unresolved, and §10's answer
creates a loop.

Both dissolve into the same question: does the app own a batch, or does it drive whatever treepad
finds on disk? Answer "it owns a batch" and it passes `--batch`, and manifest lifecycle (write on
intake, delete on teardown) is the app's — no third treepad change needed. Answer "it drives the
repo" and it needs a retirement mark that `materialise` honours, upstream. WRONG IDEA (the current
unscoped call), patch-sized once the owner is chosen — but it decides who writes manifests, which
is a schema decision, so settle it before code.

**6 — `waiting on producer deploy` has no working verb.** §5 gives it `re-run`, which relaunches
the *agent*. What needs to happen is a **CI re-dispatch**: nothing in the consumer's code changed;
`graphql-prod-compat` needs to run again against a deployed schema. Re-running the agent spends
the wall (§13.11), risks a worse branch, and — since inv. 15 caps attempts per task at two —
drains the flagship cross-repo state into `needs you` after two producer deploys. Two spec fixes:
the verb is "re-check" (`gh pr checks --watch` / a workflow re-dispatch, no agent), and
infrastructure-caused transitions do not charge the attempt counter. UNFINISHED IDEA, but it
lands in the state machine, which is why it precedes code.

Adjacent and free: the `!`-exclusion premise (table, row 2) reopens the `.env` invariant §14
dropped. The fix is deleting one line from a private, gitignored `.treepad.toml`. Take it.

---

## Per-primitive calcification assessment

| Primitive | If it is wrong: patch or rewrite | Assessment |
|---|---|---|
| **The tick** (level-triggered, 10 ordered steps, one goroutine) | **Patch** | Right, and the right shape for this problem. The step *order* has two use-before-compute hazards within one pass — step 6 needs a rollup for a tip step 5 just pushed, and step 7's draft decision reads seam-landed state that step 8 sets — but level-triggering absorbs both: the row simply settles one tick later. Say so explicitly rather than leaving it to be discovered. The I/O cost is a tune, not a restructure: `[sync]` copying is clonefile-backed on APFS (`internal/sync/darwin.go`), and moving the `tp` call to "at most one in flight per repo, loop reads the last snapshot plus its age" is additive whenever tick duration becomes visible. Do not pre-build it. |
| **The task table** (`tasks.status` truth, `events` audit) | **Patch** | Right, and the explicit refusal of an event-sourcing projection is the correct call for a single-writer local app. But §8 never enumerates a single column while the design names at least seven per-run facts it must persist: run id, pgid, process start time, baseline SHA, `hash(composed)`, pushed tip SHA, pushed-at (for §5's bounded wait), attempts. All are `ALTER TABLE`s, hence patch — but write the schema down before code, because "task fact" versus "run fact" is a real split and the two-attempt rule and re-run verbs sit across it. `flock` on the DB path is the right amount of durability machinery; a schema-version row that refuses to start on mismatch is the one addition worth having on day one. |
| **The state machine** | **Patch** | Thirteen states is the right size and the four added by revision 2 were all reachable conditions. Three defects, all local: `waiting on producer deploy`'s verb is wrong (finding 6); `push failed` has no outgoing edge drawn, though `re-run` implies one; and `stack-stale` / `pr closed unmerged` offer `remove` as a verb, which is blocked by finding 5's re-materialisation loop. Also unresolved: whether a manual `re-run` resets the attempt counter (it must, or seven states have a verb that stops working), and whether a global cause — one rate-limit blip killing all four agents — is charged to per-task budgets (it must not). All spec, none structural. |
| **The seam file** | **Patch, and it is correctly small** | "A file pasted whole into a prompt, plus a draft gate, plus a retirement pointer" is the right size, and revision 2's honest §6 statement — nothing verifies it, consumers are red until the producer *deploys* — is the most valuable paragraph in the document. Two unfinished edges. (a) The format is undefined: §6 says a seam *is* a file pasted verbatim, and also that it "declares `lands_at`", and there is no sidecar or header convention for where that declaration lives. §10 calls seam files "the one artifact worth real attention"; they have no schema. (b) `hash(composed)` detects changes to the seam, and the divergences that actually cost money are changes to *reality* with the seam constant. That is worth stating as a known limit next to §13.1 rather than fixing. |
| **The `tp` subprocess boundary** | **Boundary: patch. The no-import rule: cheap to reverse now, expensive in three weeks** | The subprocess boundary is right — process isolation, a stable CLI, no shared build. The **rule** layered on top ("never imports treepad's `batch/` package — separate repos make that structural") is a calcification, and it is stronger than what was settled: decision 1 says the app "does not import treepad's `batch/` package **to re-implement mechanisms**". `batch/` is explicitly the deep module for exactly these predicates (`batch/manifest.go:1-5`: "every scheduling predicate that can be dangerously wrong lands here as a pure, table-testable function"), and `api_test.go` already polices its purity so it is *safe* to import. The rule forbids the typed, compiled, tested coupling and leaves in place every untyped one: `ReportEntry`'s JSON shape (private type, all `omitempty`, unversioned), the Manifest TOML, `pr-cache.json` semantics, the Activity-file path, and `DeriveBranch`/`slug.Slug`. Revision 1 got the readiness predicate wrong in prose *before a line of code existed*; the rule guarantees a second implementation of the same predicate. Reversing it today costs one sentence and a `require`. Reversing it after two weeks costs finding a silent divergence between two copies of `ready`. |
| **The Activity file as busy marker** | **Rewrite-class if the ownership question is answered wrong** | Finding 2. This is the one primitive where a wrong answer is not a patch, because it is the mutual-exclusion mechanism for the whole fleet and it is shared with a program the app does not control. Every other primitive here fails visibly; this one fails by running two agents in one worktree, silently, on an unusual branch name or 90 seconds into a run. Answer it before code: existence not mtime, no deletion while work is present, and `activity_file` exposed on the report. |
| **Manifest-directory intake** | **Patch, with one honest correction** | The right choice: an existing on-disk format with a public type, a documented external-writer contract (`batch/manifest.go:25-26`), and a loader that already unions a directory. Three unfinished edges, all cheap: it is not true that "both already exist" (the task table does not exist at all); the two halves share one unvalidated join key, the ticket ref; and §8 claims `batch.Load` as evidence intake is cheap while §1 forbids the app from calling it, so the app must write its own TOML reader for a schema it does not own. Pick one: import `batch` (see above), or state that the app parses the manifest itself and pin a golden-file test. |

---

## §12 additivity assessment

Fourteen rows. The form of the table — every cut carrying a named re-entry trigger — remains the
best thing about how this design was produced. Judged on whether adding each later requires
rewriting the tick, the task table, or the state machine:

**Genuinely additive (9).** Producer-side seam assertion (a grep in a skill, outside the app);
detecting producer deploys (one `curl`, one existing state's exit condition — the state already
exists, which is what makes it additive); adaptive rate-limit governor (`max_agents` becomes a
function; the launch step already consults a cap); `POST /tasks` (one handler; the table is
already the intake half — note it needs a manifest write too, which is the §1 no-import question,
not a loop change); Slack/OTel egress (a reader of `events`); MCP permission loop (a classifier
over the run log, which already exists as a file); TUI (a second reader of the same store); chain
depth cap or warning (already shipped in the skill, `treepad-manifest.md:56`); multi-machine
(declared never — correctly).

**Additive only because of a decision made elsewhere (2).** *Scope containment / `quarantined`* is
one diff check plus one state, and §5's states are data — but §7 states flatly that "`quarantined`
does not exist and `to-tickets` needs no `files` field", and the exit condition is the one the
prior review flagged: re-runs are incremental on the existing branch, so the out-of-scope commits
survive and the check fails again. Additive in mechanism, blocked in semantics. *CI retry* is
additive precisely because revision 2 deleted it and wrote down the five conditions any future
version must meet — that row is a model of how to defer something.

**Not additive as the design stands (3).**

- **Local verify.** Its outcome — a dead run with commits that failed a local check — is not
  `checking`, not `failed` ("dead run with no commits"), and not `needs you` without discarding
  the re-run it exists to enable. So it is a new state plus a new tick step between 4 and 5 plus a
  config key. §7 forecloses the config in a sentence ("There is no verify command and no per-repo
  verify config") and §12 reopens it in a cell, on the now-correct premise that `.treepad.toml` is
  private. One cheap thing makes it additive later: model a run's disposition as data (`kind`,
  `outcome`) rather than as a branch in step 4, so a second run-kind is a row, not an edit.
- **Automatic repair of descendants after a merge.** The row already admits it: `restack` has no
  moved-base predicate at all, so this is new logic in treepad, not a flag. It also needs a
  cross-step interlock, since you cannot repair a worktree without first killing its agent — which
  is the Activity-file ownership question again. Correctly deferred, correctly labelled.
- **Parent decision-trace into descendant prompts.** Needs the run transcript queryable, which is
  a different shape from "one jsonl per run", and it makes `hash(composed)` a function of a mutable
  upstream artifact — so it changes what a hash means, not just what goes in a prompt.

**Triggers gated on instruments the design does not build (5).** "you find work reaching `review
me` that /implement's suite should have caught", "you click `re-run` often enough", "`waiting on
producer deploy` rows sit unnoticed", "you reject PRs for drive-by edits often enough",
"`stack-stale` rows become routine". Each is a human-perception threshold with no counter behind
it. The `events` table is one insert away from being the instrument for all five — record a row on
every state transition and every verb click, and every one of these becomes a query. That is the
single cheapest thing on this list and it makes five deferrals real rather than notional.

**Foreclosed axes not listed as limits.** Worth adding to §13 rather than acting on. A **second
workspace** doubles concurrency silently: `max_agents` and the flock are per-workspace, while the
subscription rate limit §13.11 calls "the wall" is per-account. **The agent binary is hardcoded**
(`claude -p`, and `--bare` is pinned into invariant 17) while treepad already generalised launching
into a templated argv (`launcher.Render` over `[batch] launch`; cf. `[from_spec] agent_command` in
both repos' `.treepad.toml`) — a config key today, a grep through the tick later. **A seam with
more than one producer** is unrepresentable, and a three-repo feature has them by construction.
**`>2 repos`** is fine for the loop and not fine for the tick's serial `gh` calls, which is a tune.

---

## Lower priority — report only

Deferrable, per the brief. Listed because each is either load-bearing to a claim made elsewhere or
expensive to retrofit; everything cheap is omitted deliberately.

1. **An empty `gh pr list` is not covered by any invariant, and one path turns it into a verdict.**
   Inv. 11 covers an empty *rollup*; nothing covers a rate-limited or unauthenticated `gh`
   returning `[]`, which is indistinguishable from "no PRs". §3.3 gets the analogous point exactly
   right for treepad's report and then does not apply it to the app's own call. Most paths fail
   closed and silent, which is acceptable — but §5's "`checking` exits to `needs you` if no rollup
   appears within a bounded wait" converts a transient GitHub outage into a terminal,
   human-attention state on **every in-flight row at once**, and §13.11 already names rate limits
   as the wall. The fix is not error handling in general; it is one distinction — `gh` exited
   non-zero ⇒ skip this tick's GitHub-derived transitions entirely rather than treating `[]` as
   data. That is a loop-structure property (a tick that can be partially applied), so it is not
   deferrable to "add error handling later". Promote it to an invariant. UNFINISHED IDEA, but do
   it with the loop.
2. **The denylist is narrower than the executing surface, and the per-repo sentence in §8 is what
   has to give.** `.github/` already covers `.github/scripts/`. What it misses, all tracked, all
   executing in jobs holding AWS credentials or the merge decision: `services/.pnpmfile.mjs`
   (arbitrary ESM on every `pnpm install --frozen-lockfile`, i.e. before every job),
   `services/pnpm-workspace.yaml:32,37` plus `services/patches/*.patch` (code injected into
   `node_modules` at install time), `services/vitest.config.js` and `services/vitest/**`,
   `services/terraform/**` (run under the OpenTofu IAM role), `services/sst.config.ts`,
   `services/infra/`, `services/tools/`; `support-app/scripts/gen-graphql-prod.sh` (executed
   directly by the required compat check), `vite.config.ts`, `eslint-ci.config.ts`,
   `next.config.js`, `codegen.yml`, `.npmrc`. No git hooks and no `turbo.json` in either repo, so
   drop those from the checklist. The list is cheap; the problem is that it is **per-repo** —
   `terraform/` exists only in `services`, `vite.config.ts` only in `support-app` — and §8 states
   that `required_checks` and `compat_check` "are the only per-repo values". That sentence is what
   gives, not the list. UNFINISHED IDEA; widen on day one, it is the sole gate.
3. **`.mergify.yml` is not denylisted**, and it is the file that defines the required checks the
   app's own verdict is configured from. An agent-authored diff relaxing a `check-success`
   condition or adding an automatic-merge rule, pushed by the app, produces a merge the app did
   not perform but did cause — against §1's "Must never: merge" — and silently de-syncs
   `command-centre.toml` from reality, which invariants 11 and 12 both rest on. One path.
   WRONG IDEA to omit it, one-line fix.
4. **Restart loses exit codes, and that is fine — so delete the goroutine that collects them.**
   A re-attached process is not a child, so `Wait` is unavailable. Definitively: nothing in §5
   reads an exit code (`failed` is "dead run with no commits since its baseline"), inv. 8 forbids
   event-absence reasoning, and inv. 7 mandates pid + start-time. So §3's "one goroutine per
   running agent (waits on the process, records exit)" is redundant with the pid poll that must
   exist anyway for re-attached runs, and it is the only component whose behaviour differs before
   and after a restart. Poll for everything and restart recovery stops being a special case at
   all — four goroutine kinds, not five. UNFINISHED IDEA, and a deletion rather than an addition.
5. **Invariant 18 is the right shape and stated one clause short.** The `Host` check stops DNS
   rebinding and the `Origin` check stops cross-site `fetch` — but a plain cross-site navigation,
   `<img src>`, or form GET carries a correct `Host` and **no `Origin` header at all**, so
   "rejects any request whose `Origin` does not match" lets them through. Since `remove worktree`
   runs `git branch -D`, state it properly instead: destructive verbs are POST-only, and a missing
   `Origin` is a rejection. That is a wording fix, not a token — and it makes the token
   unnecessary.
6. **`.env` in every worktree** — see the `!`-exclusion premise correction above. Verified in both
   repos: `.env` is at `services/.treepad.toml:5` and `support-app/.treepad.toml:5`, and
   `git check-ignore .env` exits 1 in **both** (§13.3 names only `support-app`). Neither repo has
   a root `.env` on disk today, so the exposure is latent, not live. Accepting the limit is
   defensible; calling it unfixable because "`[sync] include` is an allowlist the app does not
   own" is not — the gate that matters is gitignore, not sync, and one line in each repo's
   `.git/info/exclude` closes it with no team PR. Separately worth knowing: `support-app/.npmrc`
   is tracked and holds a registry token in plaintext.
7. **Retention on `plain/.claude/runs/` and the stored composed prompts.** `plain/` is not a git
   repo and `.claude/` holds no tracked path, so unbounded run logs are disk cost, not exposure
   through git. `0700` plus a `max_age` sweep in the tick is a later five-liner. Correctly
   deferred.

---

## What must change before code, versus what can follow

**Before code (the six, plus one free correction).**

1. **The CI verdict.** Settle where the predicate lives (a pure package, data-driven) and how it is
   populated — ideally by asking Mergify's own check-run rather than transcribing eleven names — plus
   the two expiry rules: a base change invalidates a verdict, and red is red only when no retry
   attempt is pending. *Why first:* it is the only gate, it is currently wrong in three ways
   including one that mislabels stacked descendants green, and it decides a package boundary.
2. **Activity-file ownership.** Existence not mtime; no deletion while work is present; treepad
   exposes `activity_file`. *Why first:* it is the fleet's mutual exclusion, it is shared with
   `tp ui`, and it changes invariants 6, 7, 9 and the §10 upstream change.
3. **`origin/main`, not `main`.** Fix §3.10 and decide whether the manifest base changes, which may
   delete the remove-and-recreate step entirely. *Why first:* it is the sole justification for
   relaxing invariant 4.
4. **Batch scoping and teardown.** Pass `--batch`, and choose the owner of manifest lifecycle: a
   treepad retirement mark, or app-written manifests deleted on teardown. *Why first:* unscoped,
   the tick pushes branches the app never chose and invariant 2 is false; and §10's teardown answer
   loops. This decides who writes manifests, i.e. a schema decision, not a function.
5. **One chain rule in `to-tickets`,** covering fan-in and fan-out, plus a ticket-uniqueness
   assertion. *Why first:* it is the intake contract and the task table encodes it.
6. **`waiting on producer deploy`'s verb** (re-check, not re-run) and the attempt-counter rules
   (manual re-run resets; infrastructure causes do not charge). *Why first:* it is in the state
   machine, and the state column is the schema.
7. Free: correct §13.3's `!`-exclusion premise and drop `.env` from both `[sync] include` lists.

**Strongly recommended before the first line, though not blocking.** Delete the no-import rule (one
sentence, one `require`) — or, if the boundary is kept, pin a golden-JSON contract test against
`tp batch sync --json` and ask upstream to drop `omitempty` from `pr_stale` and `stack_stale`.
Write down the `tasks`/`runs` columns. Put one `events` insert on every transition and verb, which
converts five notional deferrals into real ones. Widen the denylist. Add the HTTP token.

**Can follow.** Everything in §12 judged additive above. The loop's I/O restructuring — wait for a
tick-age number. Retention rules. Operability polish. The seam file's format, unless `to-seams` is
written first, in which case settle it there.

**Do not add.** Nothing. Revision 2 cut the right things — the CI retry above all — and the
temptation now is to re-add machinery in response to this review. Every one of the six findings is
smaller than the mechanism it replaces.

---

## Findings sorted

**WRONG IDEA (the mechanism is wrong; replace it, do not extend it).** The `required_checks` flat
name list and the `services` names in it · a CI verdict with no expiry, which labels a re-parented
descendant green on evidence from a base that no longer exists · the Activity file's dual ownership and the app's
deletion of it · the unscoped `tp batch sync` call, which falsifies invariant 2 · `tp remove
--force` as the teardown mechanism · `tp new --base main` as the stale-root fix · the `to-tickets`
change scoped to fan-in only · the "never imports `batch/`" rule · §13.3's no-denylist premise and
the invariant dropped on it · omitting `.mergify.yml` from the denylist.

**UNFINISHED IDEA (backlog; the shape is right).** The `tasks`/`runs` schema · `push failed`'s
outgoing edge and the attempt-counter rules · `checking`'s unnamed bounded wait and the `pushed_at`
it needs · the seam file's format and where `lands_at` lives · `pr_stale`'s `omitempty` and the
report contract · the empty-`gh pr list` error path (do it with the loop) · the denylist's width and
§8's per-repo-values sentence · invariant 18's missing-`Origin` clause · the redundant per-agent
exit goroutine (a deletion) · retention · `/implement`'s `/code-review` step in the §2 cost model ·
§11's `docs/commands.md:70-76` citation · the "both already exist" intake claim · the `events`
instrumentation that five deferral triggers depend on.
