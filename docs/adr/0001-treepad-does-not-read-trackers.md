# 1. Treepad does not read Trackers

Date: 2026-08-12

## Status

Accepted

## Context

`tp from-spec` seeds a worktree from a Ticket. Until now it resolved the Spec itself by
shelling out to `gh issue view`, inlining the body into `PROMPT.md` under `## Spec`.

That works for GitHub and nothing else. Supporting Linear — the Tracker used at work —
raised the question of how treepad would read a second Tracker. Linear has no official
CLI, so treepad would have had to hold a `LINEAR_API_KEY` and speak GraphQL, or depend on
a third-party binary. Every further Tracker (Jira, Shortcut) repeats the cost.

Meanwhile the agent treepad hands off to already has a Linear connection through MCP.
Treepad was positioning itself as a second, worse client for data the agent could fetch
for itself.

## Decision

Treepad does not read Trackers. It resolves a Ticket to a **Ticket URL** and cites that
URL in `PROMPT.md`; retrieving the Spec is the agent's job.

Resolution is total and Tracker-agnostic:

- input that looks like a URL is used verbatim
- anything else is a **Ref**, rendered through the repo's `[from_spec] ticket_url`
  Go template
- no `ticket_url` configured and a bare Ref given is an error naming both fixes

`ticket_url` has no default. Treepad ships no knowledge of any specific Tracker — no
named provider enum, no credentials, no HTTP client, no remote-URL parsing.

## Consequences

**Good.** Any Tracker works on day one with a config line and no code — Jira and Shortcut
included. `gh` stops being a dependency of `from-spec`. There is no auth surface, no token
storage, and no API client to keep current. Adding a Tracker is never a release.

**Bad.** `PROMPT.md` is no longer self-contained or reproducible: it cites a URL rather
than carrying the Spec, so the prompt cannot be reviewed offline and re-running against a
changed Ticket silently yields different work. `docs/from-spec.md` §5 ("inspect the prompt
before running the agent") degrades to inspecting a link. The agent must have access to
the Tracker, which is now an unstated prerequisite — a missing Linear MCP surfaces as the
agent failing mid-run rather than as a treepad error.

**Also.** `from-spec-bulk` derived branch names from Ticket titles, which required
fetching. Branch names are now derived from the Ref instead (`feat/eng-123`), losing
readability. `deriveBranch`'s numeric collision suffix becomes redundant, since Refs are
already unique within a Tracker.

## Alternatives considered

**Inline the Spec, treepad reads Linear directly.** Keeps `PROMPT.md` self-contained and
reproducible. Rejected for the auth surface: a `LINEAR_API_KEY`, a GraphQL client, and
per-Tracker code for every future Tracker.

**Named Trackers behind an interface.** `tracker = "linear"` with one implementation per
Tracker, each knowing its URL shape and fetch mechanism. Rejected once the fetch was
delegated to the agent: with nothing left to retrieve, the interface had no behaviour to
abstract over, and every new Tracker would still need a code change.

**Inline when possible, fall back to a URL.** Rejected because the prompt's shape would
vary per run, and the fallback fails silently — it looks like success.
