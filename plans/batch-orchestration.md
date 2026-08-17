# Plan: Batch orchestration — parallel worktrees, Chains, and gh Stacks

> Source: [`docs/adr/0003-treepad-drives-stacks.md`](../docs/adr/0003-treepad-drives-stacks.md) plus the
> "Batch orchestration" section of [`CONTEXT.md`](../CONTEXT.md). No PRD — the ADR plus the glossary
> is the settled spec. Handoff: `.claude/handoffs/treepad__batch-orchestration-to-plan.md`.

Turn a scoped collection of Tickets into a fleet of provisioned worktrees that respects blocking
relationships — unblocked work runs in parallel, dependent work is **stacked** rather than
serialised — and expose it as a CLI harness that Conductor or tmux could sit on top of.

Vocabulary is [`CONTEXT.md`](../CONTEXT.md)'s: **Batch**, **Manifest**, **Chain**, **Stack**,
**Launcher**, **Activity file**. A Chain is treepad's ordered run of Tickets; a Stack is GitHub's
linked pull requests. A Chain *becomes* a Stack when linked.

---

## Prerequisites

### ADR 0002 lands first, as separate work

[ADR 0002](../docs/adr/0002-treepad-writes-playbooks-not-prompts.md) is accepted and unimplemented.
It deletes `PROMPT.md` rendering, `resolveOrBuildPrompt`, `writePromptFile`, `renderPrompt`,
`buildPrompt` and `from_spec.skills`, collapses `from-spec` into `tp new --ticket`, and keeps
`agent_command` with `.TicketURL` replacing `.PromptPath`.

It is a **prerequisite of this plan, not a phase of it**, for three reasons:

- Both features edit `internal/treepad/fromspec` — 0002 rewrites it, this plan retires
  `from-spec-bulk` out of it. Doing them in parallel against the same files is the failure the
  handoff warns about.
- `.TicketURL` as template data is precisely what the Launcher needs. Building the Launcher against
  `.PromptPath` means writing the migration twice.
- 0002 is small and net-negative LOC.

**Phase 1 below does not start until ADR 0002 is merged.** Everything this plan says about template
data assumes 0002's `.TicketURL` exists.

One thing this feature *resolves* for 0002: its open question *"a bulk-created worktree carries no
record of its Ticket beyond the branch name."* The Manifest is that record, and it lives in the
common dir where every worktree can read it.

### The module path must resolve first

`go.mod` declares `module treepad`, which is not a resolvable path — so no package is importable
from outside this module, and the README's `go install` line is broken today. Fixed by #146, which
is mechanical and independent of everything else here. It lands before Phase 1 so `batch` can be
created at its final path rather than renamed later.

### External dependency: `to-tickets` must learn to write the Manifest

The Manifest is written by an agent that read the Tracker — never by treepad, never by hand
(`CONTEXT.md`). That work lives in a different repo. **The feature is useless without it.** Until it
exists, Manifests are fixture files written by hand for testing, and that is the only sanctioned
hand-authoring.

### Verification spike (before Phase 3, not a phase)

Nothing in this design has been run against a real repository. Before writing a line of Phase 3:
create a scratch repo, open two throwaway pull requests, and prove from **outside any worktree**
that `gh stack link a b`

- creates the stack and sets `b`'s base to `a`,
- corrects an existing wrong base without a `gh pr edit --base` call,
- is idempotent when re-run with the same arguments,
- and, run again as `gh stack link a b c` where `c` has no pull request, **pushes `c` and opens a
  pull request for it** — the behaviour the argument filter exists to prevent.

Record the observed output. If any of these differ from ADR 0003's quotes, stop and revise the ADR —
stacked pull requests are in public preview and the documentation is a moving target.

---

## Technical design decisions

Durable across all phases.

### Storage — the common dir

All Batch state lives under the git **common dir**, resolved once with
`git rev-parse --git-common-dir` (in a linked worktree `.git` is a *file*, so `filepath.Join(path,
".git")` is wrong outside the main worktree):

```
<common-dir>/treepad/
  batches/<name>.toml       the Manifests
  activity/<branch-slug>.log the Activity files
```

Uncommitted, so no scheduling state enters history. In the common dir, so every worktree in the
fleet sees the same files. Nothing else is persisted: **run state is derived**, as everywhere else
in treepad.

### Manifest format

One Batch per file. Many Batches at once — treepad reads them all and unions them. There is **no
"current Batch"**; the existing `/` fuzzy filter in `tp ui` narrows when needed.

```toml
# <common-dir>/treepad/batches/silent-refresh.toml
name          = "silent-refresh"   # optional; defaults to the filename stem
branch_prefix = "feat/"            # optional; default "feat/"
base          = "main"             # optional; default the repo's default base

[[chain]]
tickets = ["ENG-12", "ENG-13"]

[[chain]]
tickets = ["ENG-14"]
```

A **list of Chains, not a DAG**. A gh Stack is strictly linear, so a format that cannot express a
diamond needs no validation, no toposort, and can never invent a dependency. Diamonds and
merge-gated edges are the Manifest author's job to linearise away.

Chains have no ordering between them and run in parallel. A Chain of one Ticket never becomes a
Stack.

### Key models

```go
// batch
type Manifest struct {
    Name         string
    BranchPrefix string
    Base         string
    Chains       []Chain
}
type Chain struct { Tickets []string }

// A Chain member resolved against config and PR state.
type Member struct {
    Ticket    string
    Ref       string
    TicketURL string
    Branch    string
    Base      string   // Chain position 0 → Manifest.Base; otherwise the previous member's Branch
    Batch     string
    Chain     int
    Position  int
}
```

Branch derivation is the load-bearing link between "a Manifest names Tickets" and "the fleet has
branches": `deriveBranch(prefix, ref)` (currently `internal/treepad/fromspec/bulk.go:99`) moves to
`batch` and becomes shared. It must stay deterministic and total — reconcile re-derives
every branch name on every tick and stores none.

### Module boundaries

| Module | Owns | Exposes |
|---|---|---|
| `batch` | Manifest parsing, Chain→Member resolution, branch derivation, the `PR` data type, **and every scheduling predicate as a pure function** | `Load`, `Resolve`, `ReadyToMaterialise`, `LinkArgs`, `RestackAction` |
| `internal/gh` | The two `gh` calls and nothing else | `PRList`, `StackLink`, `Available` |
| `internal/launcher` | Detached spawn, Activity file creation and mtime reads | `Launch`, `Activity` |
| `internal/treepad` (`batch.go`) | The reconcile orchestrator — the only module that does I/O in a loop | `Reconcile`, `BatchSync` |

`batch` is the deep module: substantial scheduling logic behind a pure, table-testable
interface with no `context.Context`, no runner, and no filesystem beyond `Load`. **Every decision
this feature can get dangerously wrong is a pure function in here.**

It is treepad's importable surface — the only one that is deliberate. (`skills` also sits outside
`internal/`, but only as an `embed.FS` for `tp skill install`; it becomes importable as a side
effect of #146, not as a designed API.)
Chain resolution and restack decisions are the part no fleet tool has an equivalent for; the rest —
spawning, `gh` invocation, the reconcile loop — is implementation that an embedder should supply or
drive itself, so it stays internal. Two consequences: the module path must resolve (#146), and no
exported identifier in `batch` may name a type declared under `internal/...`, which is why `PR` is
declared here rather than in `internal/gh`.

### Interface contracts

```go
// batch — pure, no I/O.
func Resolve(m Manifest, ticketURLTmpl string) ([][]Member, error)

// Position 0 is always ready. Position i is ready when member i-1's branch has an OPEN PR.
func ReadyToMaterialise(chain []Member, existing map[string]bool, prs map[string]PR) []Member

// The longest PREFIX of chain whose branches all have an open PR, in stack order,
// bottom to top. Returns nil for fewer than two members.
func LinkArgs(chain []Member, prs map[string]PR) []string

type RestackAction int // RestackNone, RestackFastForward, RestackReset, RestackStale
func RestackDecision(clean bool, ahead, behind int, patchEquivalent bool) RestackAction
```

```go
// batch — plain data, no gh knowledge, because the predicates above take it.
type PR struct {
    Number      int
    HeadRefName string
    BaseRefName string
    State       string // OPEN | MERGED | CLOSED
    URL         string
}

// internal/gh — the ONLY gh surface. Two commands, both keyed by the git remote.
func Available(ctx context.Context, r worktree.CommandRunner) bool
func PRList(ctx context.Context, r worktree.CommandRunner) (map[string]batch.PR, error) // keyed by head branch
func StackLink(ctx context.Context, r worktree.CommandRunner, branches []string) error
```

`PRList` is **one** `gh pr list --json number,headRefName,baseRefName,state,url --state all --limit
200` for the whole repo, never one call per branch. `--state all` because merged and closed states
drive retire and the prefix filter.

Treepad **never** calls `gh stack init`, `add`, `submit`, `modify`, `rebase`, or `sync`. There is no
code path to them, and a test asserts the `gh` module exposes nothing else.

### Reconcile — the single function

```go
func Reconcile(ctx context.Context, d deps.Deps, in ReconcileInput) (Report, error)
```

`tp batch sync` and `tp ui`'s tick call this same function. The non-interactive verb is what a hook,
cron, or Conductor drives later. Its five steps, in order:

```
materialise → launch → link → restack → retire
```

`Report` is the JSON shape for `--json` and the row source for the TUI: one entry per Member with
Batch, Chain index, position, branch, worktree path, PR number/state, run state, and any action
taken this tick.

### Two tick cadences

Local work (worktree list, dirty, ahead/behind, Activity mtime) on the existing ~5s TUI tick.
`gh pr list` on a separate ~60s tick, cached between. `tp status` has never touched the network and
still doesn't. Offline or `gh` absent → last-known PR state plus a staleness marker, **never a blank
column**.

### Launcher

`[batch] launch` in `.treepad.toml` — `agent_command` generalised. A **config template, not an
interface**, so tmux, a new terminal, detached-with-log, and Conductor are all *values* rather than
code paths.

```toml
[batch]
launch = ["claude", "--dangerously-skip-permissions", "{{.TicketURL}}"]
```

Template data: `.Branch`, `.Slug`, `.WorktreePath`, `.TicketURL`, `.Ref`, `.ActivityFile`, `.Batch`,
`.Chain`, `.Position`.

Spawn is **detached**: `cmd.Dir = worktreePath`, stdout and stderr both redirected to the Activity
file, `Setpgid: true`, `Start()` then release. It cannot reuse `fromspec.runAgent`
(`from_spec.go:181`), which is blocking and pty-bound via `d.PTRunner`.

Treepad neither parents nor supervises the result. **There is no supervisor loop.**

### Liveness — the Activity file, never a PID

The Activity file's mtime is the only evidence treepad has that an agent is working. Whatever
touches it — the Launcher's log redirect, a harness hook, a human — is not treepad's concern. A tmux
Launcher's child exits in milliseconds and a Conductor agent was never treepad's child, so PID
tracking would buy precision by spending the entire pluggability argument.

Derived run state:

| State | Rule |
|---|---|
| `pending` | worktree exists, no Activity file |
| `working` | mtime within 90s |
| `idle` | mtime older than 90s |

No `stuck` label. A label that is wrong a third of the time is worse than no label.

**Existence of the Activity file is also the double-launch guard.** Once launched, treepad never
relaunches automatically — derivable state, no new persistence.

### Restack safety predicate

Merging a lower pull request rewrites every branch above it **server-side**. Every worktree above
the merge then holds a local branch pointing at commits that no longer exist upstream, possibly with
an agent mid-edit.

ADR 0003 specifies auto-repair as "a fast-forward to `origin`, which cannot lose work". **A
fast-forward cannot repair that case** — a server-side rebase leaves local and `origin/<branch>`
diverged, not merely behind, and `merge --ff-only` refuses diverged branches. The ADR's *safety
property* is correct; its mechanism only covers the plain-behind case. Resolution, decided
2026-08-16:

```
git -C <wt> fetch origin <branch>

clean && behind > 0 && ahead == 0              → git merge --ff-only origin/<branch>
clean && diverged && no `+` lines from
  `git cherry origin/<branch> <branch>`        → git reset --hard origin/<branch>
otherwise                                      → stack-stale, wait for a human
```

`git cherry` compares by **patch id**: no `+` lines means every local commit is already present
upstream in rewritten form, so the reset discards nothing. Anything dirty, or holding a commit that
is genuinely not upstream, is reported as `stack-stale`.

**Treepad never stashes on an agent's behalf.**

### Treepad does not merge

Responsibility ends at a linked Stack with correct bases and coherent worktrees. Reviewers merge on
github.com; the tick notices, repairs the branches above, and marks the merged worktree removable.
Atomic whole-stack merge works *against* the goal of small independently-reviewed chunks.

### Degradation without `gh`

`gh` is a hard dependency of Batch orchestration and this does not contradict ADR 0001: a Stack is
keyed by the git **remote** treepad already knows, not by a Tracker. No per-provider code path, no
credential storage, no API client — only an installed `gh`.

When `gh` is absent or the network is down: **materialise and launch Chain heads only** (position 0
is unblocked by definition and needs no PR state), skip link, skip retire, and report `gh required`
against every deeper member. A `doctor` finding names it. Deeper members are *not* materialised on a
branch-exists fallback — that would start agents on top of parents that have produced nothing
reviewable, which is exactly what the PR gate exists to prevent.

### Command surface

```
tp batch sync [--json] [--dry-run] [--launch] [--offline] [--batch <name>]
tp batch list [--json]
tp ui                       # same Reconcile on its tick
```

`sync` materialises and links but **does not launch** unless `--launch` is passed. Spawning a fleet
of agents is not a side effect of a reconcile verb. In the TUI, launch is a keystroke behind the
existing confirm modal.

### Nothing corrects pull request bases

`gh stack link` already does: *"Existing pull requests whose base branch does not match the expected
chain are corrected automatically."* A `gh pr edit --base` call would be redundant. An earlier design
decision to add one was reversed on this evidence — **do not reintroduce it.**

---

## Phase 1: Manifest → materialised Chain

**Covers**: a Manifest declares a Batch of Chains; each Chain seeds one worktree per Ticket, each
branched from the one before it.

### What to build

`batch` with the Manifest type, `Load` (glob `<common-dir>/treepad/batches/*.toml` via
BurntSushi/toml, already a dependency; union every file; filename stem as the default name), and
`Resolve` turning Chains into `[]Member` — resolving each Ticket through the repo's
`[from_spec] ticket_url` template to a Ticket URL and a Ref, deriving the branch from
`branch_prefix + slug(ref)`, and setting each member's base to the previous member's branch.
`deriveBranch` moves here from `fromspec/bulk.go:99`.

`tp batch sync` reads every Manifest and materialises each Chain **in order**, calling the existing
`lifecycle.CreateWorktreeWithSync` (`lifecycle.go:40`) — which already creates a stacked worktree
when `base` is a sibling's branch (`lifecycle.go:69`), it just never recorded the relation.
Materialisation is gated on "the parent branch exists", which after materialising the parent it
does. **Phase 2 tightens this same gate to "the parent has an open pull request" — same code path.**

Skipping is by branch existence, so re-running is idempotent and a partial failure resumes. A
failure on one member records the error and stops that Chain only; other Chains continue.

Reconcile's step order and `Report` shape land here in skeleton; later phases fill in the steps.

### Acceptance criteria

- [ ] `Load` reads every `*.toml` under the batches dir and unions them; a malformed file is an
      error naming the file, not a silent skip
- [ ] `Resolve` is pure and table-tested: a two-Chain Manifest yields correct branch and base for
      every member, position 0 basing on `Manifest.Base`
- [ ] Common dir is resolved with `git rev-parse --git-common-dir` and works when invoked from
      inside a linked worktree, not just the main one
- [ ] `tp batch sync` on a Manifest with chains `["ENG-12","ENG-13"]` and `["ENG-14"]` creates three
      worktrees; `feat/eng-13`'s merge-base with `feat/eng-12` is `feat/eng-12`'s tip
- [ ] Re-running creates nothing and exits 0
- [ ] A Ticket that fails to resolve stops its own Chain and leaves other Chains untouched
- [ ] `--json` emits the `Report`; `--dry-run` prints what would be created and touches nothing
- [ ] e2e testscript covering the two-Chain case

---

## Phase 2: `gh pr list` and the ready gate

**Covers**: blocked work stacks immediately rather than waiting for a merge; the ready signal is the
parent's pull request existing.

### What to build

`internal/gh` with `Available` (an `exec.LookPath` plus an auth probe) and `PRList` — one
`gh pr list --json … --state all --limit 200` for the repo, parsed into a map keyed by head branch.
Never one call per branch.

Tighten `ReadyToMaterialise`: position 0 is always ready; position *i* is ready only when member
*i-1*'s branch has an **open** pull request. A pushed branch is not a promise the work is reviewable
— an agent pushing a work-in-progress commit at minute two must not unblock the layer above against
nothing.

Degradation: `gh` absent or the call fails → materialise Chain heads only, report `gh required`
against deeper members, exit 0. `--offline` forces this path. Never a hard error, never a
branch-exists fallback.

### Acceptance criteria

- [ ] `PRList` issues exactly one `gh` invocation regardless of fleet size — asserted with a
      counting fake runner
- [ ] `ReadyToMaterialise` is pure and table-tested, including: parent open → ready; parent has no
      PR → not ready; parent's PR closed → not ready; parent merged → ready
- [ ] `tp batch sync` on a fresh Manifest creates only Chain heads; after a pull request is opened
      on a head, the next run creates position 1 and no further
- [ ] With `gh` absent, Chain heads still materialise, deeper members report `gh required`, and the
      exit code is 0
- [ ] `--offline` behaves identically to `gh` being absent

---

## Phase 3: `gh stack link`, filtered to the PR-having prefix

**Covers**: a Chain *becomes* a Stack when its pull requests are linked.

> Run the verification spike first. Do not write this phase against the documentation alone.

### What to build

`gh.StackLink(ctx, runner, branches)` shelling out to `gh stack link <branch> <branch> …`, arguments
in stack order, bottom to top. No checkout required, no local state written.

`batch.LinkArgs` returns the **longest prefix** of the Chain whose members all have an open pull
request — not a filter of all members that have one. If member 1's pull request is closed while
member 2's is open, passing `[m0, m2]` would set m2's base to m0's branch: a silently wrong Stack.
A prefix cannot produce one. Fewer than two members returns nil — a Chain of one Ticket never
becomes a Stack; one pull request against main is just a pull request.

**The filter is load-bearing and easy to lose.** `gh stack link` pushes every branch argument to the
remote and *creates* pull requests for any that lack one, so a refactor that passes the whole Chain
"for tidiness" would push every agent's work-in-progress and open pull requests for it.

Treepad re-lists the whole Chain-so-far every tick and persists no stack number: `link` is additive
and idempotent, arguments already in the stack are skipped, and existing pull requests are never
removed. The corollary is that treepad can build a Stack but can never take one apart or reorder it
— that is a human job on github.com, and the command's help text should say so.

Nothing calls `gh pr edit --base`.

### Acceptance criteria

- [ ] **`LinkArgs` excludes a Chain member that has no pull request** — the explicit test ADR 0003
      demands, and it fails loudly if someone widens the filter
- [ ] `LinkArgs` returns a prefix: given PR states `[open, none, open]` it returns only member 0's
      branch and therefore links nothing
- [ ] `LinkArgs` returns nil for a single-member Chain and for a Chain whose head has no pull request
- [ ] `LinkArgs` output is bottom-to-top order
- [ ] A test asserts `internal/gh` exposes no `init`, `add`, `submit`, `modify`, `rebase`, or `sync`
- [ ] Re-running `tp batch sync` twice issues the same `link` arguments and treepad persists no
      stack identity between runs
- [ ] Spike findings recorded in the phase's PR description; any divergence from ADR 0003's quotes
      raised as an ADR amendment before merge

---

## Phase 4: Restack and retire

**Covers**: merging a lower pull request rewrites every branch above it server-side; treepad repairs
in-worktree, where the branch is legitimately checked out and plain `git` works.

### What to build

`batch.RestackDecision` as the pure predicate above — clean plus behind-only → fast-forward; clean
plus diverged plus patch-equivalent → reset; anything else → `stack-stale`. This function and
`LinkArgs` are the two things in the feature that must not be got wrong; both get the TDD treatment.

Reconcile's restack step: for each materialised member with an upstream, `git fetch origin <branch>`,
gather clean/ahead/behind from the existing `worktree.Dirty` and `worktree.AheadBehind`, run
`git cherry origin/<branch> <branch>` only when diverged, and apply the decision inside the member's
own worktree.

`stack-stale` slots into `deriveStatus` (`status.go:270`) as a new label and key, and into
`doctor.go`'s fleet loop as a new finding kind. Retire: a member whose pull request is `MERGED` is
marked removable, reusing the existing `merged (safe rm)` path.

`StatusRow` gains `Batch`, `Chain`, `Position`, `PRNumber`, `PRState` and `RunState`.

### Acceptance criteria

- [ ] `RestackDecision` is pure and table-tested across the full matrix: clean/dirty × behind-only /
      diverged / ahead-only / in-sync × patch-equivalent or not
- [ ] Dirty never auto-repairs, under any combination — asserted directly
- [ ] Diverged with a genuinely local commit never auto-repairs
- [ ] A rewritten-upstream fixture (rebase and force-push a branch, clean worktree) resolves via
      `reset --hard` and leaves the worktree at `origin/<branch>` with no lost commit
- [ ] Treepad issues no `git stash` anywhere — asserted against the fake runner's command log
- [ ] `stack-stale` appears in `tp status`, `tp ui`, and as a `tp doctor` finding
- [ ] A merged member is reported removable

---

## Phase 5: Launcher and the Activity file

**Covers**: treepad supplies the template data and nothing else — it does not own the resulting
process.

### What to build

`[batch] launch` config as specified above. `internal/launcher`: render each element through
`text/template`, create `<common-dir>/treepad/activity/<branch-slug>.log`, spawn detached with
`cmd.Dir` set to the worktree, both streams redirected to that file, `Setpgid: true`, `Start()`,
release. Behind a `Launcher` interface so tests fake it and nothing spawns in CI.

`launcher.Activity(commonDir, branch)` reads mtime and returns `pending` / `working` / `idle`.
Activity file existence is the double-launch guard: a member that has one is never relaunched.

Reconcile's launch step runs only under `--launch`. Empty `[batch] launch` → materialise, report
"N ready to launch", exit 0, exactly as `agent_command` behaves today.

### Acceptance criteria

- [ ] Template rendering is table-tested over all documented fields including `.TicketURL` and
      `.ActivityFile`
- [ ] The spawned process survives `tp batch sync` exiting — verified with a fixture command that
      sleeps then writes a sentinel, asserted after the parent has returned
- [ ] Agent stdout and stderr both reach the Activity file
- [ ] Run state derivation is table-tested against synthetic mtimes; no `stuck` state exists
- [ ] A second `--launch` run does not relaunch a member that already has an Activity file
- [ ] Empty `[batch] launch` exits 0 with a "ready to launch" report and spawns nothing
- [ ] `tp batch sync` without `--launch` spawns nothing

---

## Phase 6: `tp ui` — Batch and Chain in the fleet view

**Covers**: the fleet view is the only place that knows the whole shape.

### What to build

`tp ui` calls `Reconcile` on its tick — the same function `tp batch sync` calls — with launch
disabled. Rows group by Batch, then by Chain, with position indicated so a Chain reads bottom to
top. New columns for run state and pull request state. The `gh` cadence is a second, slower tick
alongside the existing `doRefresh`/`doTick` pair (`tui_update.go:19,33`).

An `l` key launches the selected member through the existing confirm modal — the
`uiModeConfirmRemove`/`ForceRemove`/`Prune`/`Shell` state machine (`tui_update.go:200–290`) is
already the approve-before-launch UI and is already used four times. A `L` key launches every
`pending` member in the fleet, behind the same modal. A key opens the Activity file in the pager via
`tea.ExecProcess` (`tui_update.go:427`), the mechanism the `e` key already uses.

The `/` fuzzy filter matches Batch and Chain names as well as branches — this is what replaces a
"current Batch" concept.

Degradation: `gh` unavailable → pull request columns show last-known plus a staleness marker, never
blank, matching `doctor --offline`.

### Acceptance criteria

- [ ] Rows group by Batch then Chain, ordered by position within a Chain, main first
- [ ] Run state and pull request state render; a `gh`-less run shows staleness rather than blanks
- [ ] `l` launches exactly one member and only after confirmation
- [ ] `L` launches every pending member and only after confirmation
- [ ] The log key opens the selected member's Activity file and the TUI resumes on exit
- [ ] `/` matches Batch and Chain names
- [ ] A worktree with no Manifest entry still renders, ungrouped — `tp ui` remains the whole fleet's
      view, not just the Batch's

---

## Phase 7: Retire `from-spec-bulk`; document

**Covers**: a Batch of single-Ticket Chains *is* `from-spec-bulk`.

### What to build

Delete `internal/commands/from_spec_bulk.go`, `internal/treepad/fromspec/bulk.go` and their tests;
drop `fromSpecBulkCommand()` from `router.go`. `deriveBranch` has already moved to `batch`
in Phase 1. `tp from-spec-bulk` exits non-zero with a message naming the Manifest as the replacement
— a removed verb that fails silently is worse than one that explains itself.

`docs/commands.md`, `docs/configuration.md` and the README gain a Batch orchestration section:
the Manifest format, `[batch] launch`, the `gh` requirement, and — prominently — **Chain depth is a
review-latency multiplier**. Layer five cannot land until four reviews complete below it, and every
merge below rewrites the base under the agents above. Deep Chains optimise writing throughput and
pessimise review throughput, which is the opposite of the point. **Shallow and wide beats deep and
narrow.** This is guidance for whatever writes the Manifest and a candidate future `doctor` warning.

Document the sharp edge honestly: because `link` is additive only, treepad can build a Chain into a
Stack but can never take one apart. **A Manifest edited after linking leaves a Stack on GitHub that
no treepad command can correct.**

### Acceptance criteria

- [ ] `from-spec-bulk` is gone from the router, the codebase, and the docs
- [ ] Invoking it exits non-zero naming the Manifest as the replacement
- [ ] `just ci` passes with no reference to `fromspec.FromSpecBulk` remaining
- [ ] Docs cover the Manifest format, `[batch] launch`, the `gh` requirement, the depth guidance,
      and the edited-Manifest-after-linking edge
- [ ] `CONTEXT.md`'s Batch vocabulary is used throughout the docs, help text, and error messages

---

## Out of scope for v1

| Deferred | Where it slots in |
|---|---|
| Merging | GitHub owns it — see ADR 0003 |
| tmux / attach-to-running-agent | a different `[batch] launch` value plus one ui key |
| Harness-hook heartbeats | the same Activity file, a better writer |
| Diamonds and merge-gated edges | not expressible; the Manifest author linearises them away |
| `doctor` warning for deep Chains | Phase 4's finding-kind machinery, one more kind |
| Teaching `to-tickets` to write the Manifest | a different repo — but the feature is useless without it |
| Un-linking or reordering a Stack | `gh stack link` is additive only; a human job on github.com |
