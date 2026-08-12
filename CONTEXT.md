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

## Relationships

- A **Tracker** hosts many **Tickets**
- A **Ticket** is addressed by a **Ticket URL**, which yields exactly one **Tracker** and one **Ref**
- A **Ref** alone does not identify a **Ticket** — it only resolves against a **Tracker** the repo has declared
- A **Ticket URL** always overrides the declared **Tracker**, so a repo can pull a one-off **Ticket** from elsewhere
- A **Ticket** carries exactly one **Spec**
- Treepad resolves a **Ticket** to a **Ticket URL**; the agent resolves the **Ticket URL** to a **Spec**
- A **Prompt** cites exactly one **Ticket URL** and seeds exactly one worktree

## Example dialogue

> **Dev:** "So `--issue 42` and `ENG-123` are different things?"
> **Domain expert:** "No — both are **Refs**. A GitHub issue and a Linear ticket are the same concept to us: a **Ticket**. What differs is the **Tracker** hosting it. I use GitHub issues on personal projects to get Linear's shape on a zero budget."
> **Dev:** "Then why pass a **Ticket URL** rather than the **Ref**?"
> **Domain expert:** "Because a bare **Ref** doesn't say which **Tracker** it lives on. The URL carries both, so I can paste one thing and treepad works it out."

## Flagged ambiguities

- "issue" was used for both the **Ticket** and its **Spec** (`resolveIssueSpec` returns a body, not a ticket) — resolved: the container is a **Ticket**, the content is a **Spec**.
- Docs in `internal/config/init.go` and `docs/from-spec.md` described a `--file` spec source that never existed in code — resolved: it is neither a **Ticket** nor a separate concept, there is no such source; the stale docs have been deleted.
- "tracker" names a real-world system but is deliberately *not* a treepad concept — treepad only knows **Ticket URLs** and the template that produces them. See [ADR 0001](./docs/adr/0001-treepad-does-not-read-trackers.md).
