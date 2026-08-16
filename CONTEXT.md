# Treepad

Treepad manages git worktrees as disposable, fully-provisioned workspaces, and seeds them from tracked work so an agent can start immediately.

## Language

### Spec-driven creation

**Ticket**:
A unit of tracked work, hosted by a Tracker, that a worktree is created to deliver.
_Avoid_: issue, story, card

**Spec**:
The body text a Ticket carries. Treepad never reads it — it points the agent at the Ticket URL and the agent retrieves it.
_Avoid_: description, body, requirements

**Tracker**:
The external system that hosts Tickets — Linear at work, GitHub for personal projects. Treepad holds no knowledge of any specific Tracker; a repo declares its own by giving the URL shape its Refs expand into.
_Avoid_: provider, backend, source, tool

**Ticket URL**:
The canonical web address of a Ticket, from which its Tracker and Ref are both derived.

**Ref**:
The Tracker-local identifier of a Ticket — `42` on GitHub, `ENG-123` on Linear. Opaque to treepad.
_Avoid_: issue number, ID

**Prompt**:
The rendered `PROMPT.md` a worktree is seeded with: Spec, Skills, and closing instructions.

### Batch orchestration

**Batch**:
A collection of Tickets treepad orchestrates as one unit, declared by a Manifest.
_Avoid_: project, epic, group, bulk

**Manifest**:
The uncommitted local file declaring one Batch's Chains, written by an agent that read the Tracker — never by treepad and never by hand.
_Avoid_: config, plan, spec

**Chain**:
An ordered run of Tickets within a Batch, each worktree branched from the one before it. Exists from the moment the Manifest is written, before any pull request does.
_Avoid_: lane, track, sequence, line

**Stack**:
GitHub's object — a Chain's pull requests linked into an ordered series, each targeting the branch below it. A Chain *becomes* a Stack when its pull requests are linked.
_Avoid_: using this for the treepad-side ordering; that is a Chain

**Launcher**:
The configured command treepad renders and runs to start an agent in a worktree. Treepad supplies the template data and nothing else — it does not own the resulting process.
_Avoid_: runner, executor, driver

**Activity file**:
The per-worktree file whose modification time is the only evidence treepad has that an agent is working. Whatever touches it — the Launcher's log redirect, a harness hook, a human — is not treepad's concern.
_Avoid_: heartbeat, pidfile, lockfile

## Relationships

- A **Tracker** hosts many **Tickets**
- A **Ticket** is addressed by a **Ticket URL**, which yields exactly one **Tracker** and one **Ref**
- A **Ref** alone does not identify a **Ticket** — it only resolves against a **Tracker** the repo has declared
- A **Ticket URL** always overrides the declared **Tracker**, so a repo can pull a one-off **Ticket** from elsewhere
- A **Ticket** carries exactly one **Spec**
- Treepad resolves a **Ticket** to a **Ticket URL**; the agent resolves the **Ticket URL** to a **Spec**
- A **Prompt** cites exactly one **Ticket URL** and seeds exactly one worktree
- A **Manifest** declares exactly one **Batch**; a repo may hold several Manifests and treepad reads them all
- A **Batch** holds one or more **Chains**; Chains have no ordering between them and run in parallel
- A **Chain** holds one or more **Tickets** in a fixed order, and seeds one worktree per Ticket
- A **Chain** maps to at most one **Stack** — a Chain of one Ticket never becomes a Stack
- A **Stack** is created and mutated only through `gh`; treepad reads the Chain, GitHub owns the Stack
- A **Launcher** starts one agent in one worktree; treepad neither parents nor supervises it
- An **Activity file** belongs to exactly one worktree and is the sole source of its liveness

## Example dialogue

> **Dev:** "So `--issue 42` and `ENG-123` are different things?"
> **Domain expert:** "No — both are **Refs**. A GitHub issue and a Linear ticket are the same concept to us: a **Ticket**. What differs is the **Tracker** hosting it. I use GitHub issues on personal projects to get Linear's shape on a zero budget."
> **Dev:** "Then why pass a **Ticket URL** rather than the **Ref**?"
> **Domain expert:** "Because a bare **Ref** doesn't say which **Tracker** it lives on. The URL carries both, so I can paste one thing and treepad works it out."

> **Dev:** "If a **Chain** is just the **Tickets** in order, why isn't it a **Stack**?"
> **Domain expert:** "Because a **Stack** is GitHub's word and GitHub's object — it's pull requests, and it doesn't exist until they're linked. A **Chain** exists the moment the **Manifest** is written, before a line of code."
> **Dev:** "So a **Chain** of one **Ticket** is a **Stack** of one?"
> **Domain expert:** "It's not a **Stack** at all. One pull request against main is just a pull request."
> **Dev:** "And two **Tickets** that block nothing — same **Chain** or two?"
> **Domain expert:** "Two. A **Chain** means each one waits on the one below it. Putting independent work in the same **Chain** invents a dependency, and the top one can't merge until the bottom one does."

## Flagged ambiguities

- "issue" was used for both the **Ticket** and its **Spec** (`resolveIssueSpec` returns a body, not a ticket) — resolved: the container is a **Ticket**, the content is a **Spec**.
- Docs in `internal/config/init.go` and `docs/from-spec.md` described a `--file` spec source that never existed in code — resolved: it is neither a **Ticket** nor a separate concept, there is no such source; the stale docs have been deleted.
- "tracker" names a real-world system but is deliberately *not* a treepad concept — treepad only knows **Ticket URLs** and the template that produces them. See [ADR 0001](./docs/adr/0001-treepad-does-not-read-trackers.md).
- "stack" was used for both the treepad-side ordering and GitHub's object — resolved: the plan is a **Chain**, GitHub's linked pull requests are a **Stack**, and a Chain *becomes* a Stack when linked. The ideas doc's "Stacks" feature name predates this split.
- "blocked" means two different things and only one is modelled — a **Ticket** below another in a **Chain** waits for that Ticket's *pull request to exist*, not for it to merge. Merge-gated dependencies are not expressible in a Manifest; they are the author's job to linearise away.
- A **Batch** is not a Tracker project, cycle, or milestone. It is whatever collection of **Tickets** a Manifest names, and treepad has no way to check it against the Tracker.
