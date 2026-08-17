# Writing Playbooks

A **Playbook** is a plain Markdown document saying which Skills a recurring shape of work should use, and why. This guide covers when to write one, what to put in it, and when to use something else instead.

See also:
- [ADR 0002](adr/0002-treepad-writes-playbooks-not-prompts.md) — the design decision and constraints
- [commands.md](commands.md#playbook) — command reference for `tp playbook new`
- [from-spec.md](from-spec.md#playbooks) — how to name a Playbook on a Ticket

## Three separate jobs

Before Playbooks existed, a single file tried to do three things at once:

1. **Prompt authoring** — composing boilerplate like `# branch-name` and `## Spec`. The harness already does this better.
2. **Skill routing** — deciding which Skills a given unit of work should load, and saying why.
3. **General standing instructions** — guidance that applies to every task.

A Playbook captures only the second job. The other two belong elsewhere:

### When it's a Playbook

The guidance is **per-shape-of-work** and **reusable**. The same routing decision applies to every Ticket of a recurring kind. Example:

> Dashboard work needs `/impeccable` for the visual layer — the default styling pass isn't enough. Load `/dataviz` before writing any chart.

That decision is made once and applies to every Ticket tagged with `Playbook: task-dashboard`. The author (a senior dev, or someone who tried it once) writes it down, the agent reads it, and everyone benefits.

### When it's CLAUDE.md instead

The guidance applies to **every Ticket in the repo**. It belongs in [CLAUDE.md](../CLAUDE.md), not a Playbook.

Bad — writing this as a Playbook:

```markdown
# My Playbook

Use /code-review to review all work.
Test each change with a local run before shipping.
```

Why it's wrong: this applies to every task, every day. Encode it once in CLAUDE.md:

```markdown
## Review and test

- Use /code-review to review all work.
- Test each change with a local run before shipping.
```

Now every agent in the repo reads it automatically, without needing to know about a specific Playbook.

### When it's a Skill instead

The guidance is **a reusable capability** — not routing, but a piece of work itself. Playbooks delegate to Skills; they don't replace them.

Example: you have a custom linter only this team uses. Build it as a Skill (`/my-team-lint`), and mention it in a Playbook if only certain Tickets should run it:

```markdown
Run /my-team-lint after writing Go code — it catches our house style before review.
```

If the linter applies to every Ticket, mention it in CLAUDE.md instead.

## Prose, not lists

A bare list of Skill names is weaker than a Skill name with a reason. The reason is what makes the routing stick.

Bad example:

```markdown
- /impeccable
- /dataviz
- /code-review
```

The reader has no idea when to use each one, or why they go together. A debug session later, if the agent didn't load one of them, you can't easily tell whether the Playbook was wrong or the agent chose differently.

Good example:

```markdown
Use /impeccable for the visual layer — this is dashboard work and the default styling pass isn't enough. Load /dataviz before writing any chart, since dashboards are data-heavy. Use /code-review once implementation is done, because visual regressions are easy to miss.
```

Now it's clear: the routing decision is anchored in the *shape* of the work (dashboard), not an arbitrary list. If a future Ticket doesn't feel like dashboard work, the author can skip it. If a chart breaks the style, you know which Skill was meant to catch it.

## Naming and granularity

A Playbook is named after a **recurring shape of work**, not a feature or a ticket.

Good names:

- `go-service` — any backend service in Go
- `task-dashboard` — any dashboard or TUI task page
- `docs` — documentation work in this repo

Bad names:

- `eng-123` — specific ticket, defeats the purpose
- `add-login-button` — one feature, too granular
- `all-work` — too broad, should be in CLAUDE.md

If you find yourself writing a Playbook for a single Ticket, stop and put it on the Ticket instead (as a comment, or in CLAUDE.md if it's truly a standing rule).

## The designation is durable

When you name a Playbook on a Ticket, the agent reads the Ticket and loads the Playbook itself. Treepad writes the bytes and never reads them back.

Consequences:

- **The routing decision survives a re-run.** The Ticket holds the name, not the CLI invocation. Typing `tp new` twice with the same Ref loads the same Playbook both times.
- **Bulk operations work without per-Ticket flags.** `tp from-spec-bulk` creates worktrees for a list of Tickets. Each Ticket names its own Playbook; the tool does not need a `--playbook` flag or config override.
- **The choice is visible and editable in the Tracker.** A team member can see which Playbook a Ticket uses, and change it if the shape of work evolved. No shell history to dig through.

This is why there is no `--playbook` flag and no `default_playbook` config key. The designation travels with the work, not with the invocation.

## Verbatim means verbatim

`tp playbook new` writes the body **exactly as you provide it**. Treepad composes nothing, interpolates nothing, and appends nothing.

Practical consequence: **template syntax in a Playbook is literal text.** There is no `{{.Branch}}`, no `{{.TicketURL}}`, no variable substitution of any kind.

```bash
tp playbook new go-service <<'EOF'
Use {{.CustomSkill}} to...
EOF
```

You have just written the literal string `{{.CustomSkill}}`. Treepad does not know about your custom Skills, does not expand variables, and does not guess. Write what you want the agent to read.

This constraint is deliberate — it keeps the Playbook format simple and treepad's surface small. If you need dynamic routing based on branch name, ticket ref, or other runtime data, that logic belongs in your agent, not in treepad.

## Limits and boundaries

Treepad writes Playbooks and propagates them. The agent reads them. Understand what each side can and cannot do.

**What treepad cannot do:**

- **Guarantee a Playbook was honoured.** A Ticket names a Playbook; the agent loads it when it reads the Ticket. If the Playbook does not exist, the load fails inside the agent, not as a treepad error. There is no pre-flight check.
- **Fire a Playbook by hand.** Playbooks are not Skills — they have no `/name` shorthand. You cannot run them directly from the CLI.
- **Retroactively name Tickets.** A Ticket cut before the Playbook existed carries no designation. Editing the Playbook afterward does not affect past runs.

**What `tp` does handle:**

- **Propagation.** Playbooks live in `.claude/playbooks/` and sync to every worktree through the existing `[sync]` machinery. Once written, they reach everywhere.

**What is out of scope:**

- `to-tickets` (the tool that cuts Tickets from a manifest) must learn to emit the `Playbook:` line if you want bulk-created Tickets to name one. That work is outside this repo.

## When not to write one

**One-off instructions.** A single Ticket has special needs. Add a comment on the Ticket, or include the instruction in the Spec itself. Do not write a Playbook for one use.

**Repo-wide standards.** The instruction applies to every Ticket, every day. Encode it in CLAUDE.md with a clear section heading, so every agent reads it without being told.

**A capability, not routing.** You have a tool, a linter, a test harness. Build it as a Skill and document it in the Skill's frontmatter. Mention it in a Playbook if only certain Tickets should use it.

**Speculation.** You think dashboard work *might* need `/impeccable` someday. Do not write a Playbook until you have shipped at least one Ticket using that shape and learned what actually helps.

---

For a complete walkthrough of naming and using a Playbook in a real project, see [from-spec.md](from-spec.md#walkthrough-shipping-a-ticket-end-to-end) § 2 and § 3.
