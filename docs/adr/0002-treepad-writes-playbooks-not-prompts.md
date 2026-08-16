# 2. Treepad writes Playbooks, but does not author Prompts

Date: 2026-08-12

## Status

Accepted

## Context

[ADR 0001](./0001-treepad-does-not-read-trackers.md) established that treepad should not be a
worse client for something the agent does better, and removed Tracker reading on that basis.
The same argument reaches one level further up. `PROMPT.md` rendering, `from_spec.skills`, and
`--prompt` amount to a bespoke prompt paradigm competing with Skills, `CLAUDE.md`, and the
harness's own context assembly — roughly 130 lines of `text/template` sustaining a paradigm
that `tp cd` plus a text editor already covers. No competitor authors prompts either, so this
is not differentiation.

But the deletion argument, taken alone, kills something real by accident. Two separable jobs
were tangled together in `PROMPT.md`:

- **Prompt authoring** — composing `# <branch>` / `## Spec` / `Implement the ticket.` into a
  file. A worse duplicate of what the harness already does.
- **Skill routing** — deciding which Skills a given unit of work should load, and saying why.
  Nothing in the harness does this *per unit of work*. Description matching in Skill frontmatter
  is the only mechanism, and it is probabilistic.

`from_spec.skills` was routing delivered through a prompt file — indicting the delivery
mechanism convicted the payload too. Preserving it as-is is not the answer either: it is
repo-scoped and static, so it can express only "the same Skills for every task", which is
exactly the behaviour to avoid. A dashboard Ticket wants a dashboard Skill; every other Ticket
in the repo does not.

Two further observations shaped the design. First, a bare Skill name is a weaker signal than a
Skill name with a reason — "use `/impeccable` for the visual layer because this is dashboard
work" outperforms a `- /impeccable` bullet. So the payload is prose, not a list. Second, the
same prose applies to every Ticket of a recurring shape, so it wants to be written once and
named.

Prose, named, reusable, describing which Skills to use and why — that is the shape of a
document the agent reads, and the harness already does exactly this in the `implement` Skill
("Use /tdd where possible… use /code-review to review the work").

## Decision

Treepad **writes** Playbooks and **propagates** them. It does not author Prompts, does not
compose them, and does not read them back.

- A **Playbook** is a plain Markdown document at `.claude/playbooks/<name>.md`. No frontmatter,
  no substitution, no discovery rules to satisfy. It is invisible to Skill matching, so it
  cannot interfere with any other Skill.
- `tp playbook new <name>` writes the body **verbatim** under that name and adds nothing.
  Treepad composes no text, interpolates no variables, and appends no instructions. This is
  the line: scaffolding a file is not authoring a Prompt.
- Propagation reuses the existing `[sync]` machinery — `include = [".claude/playbooks/**"]`.
  No new fan-out mechanism.
- A **Ticket** names the Playbook that applies to it. The agent reads the Ticket already
  (ADR 0001), so it picks the name up for free. Treepad gains no `--playbook` flag and no
  config key: the designation travels with the work, not with the invocation.
- `PROMPT.md` rendering, `resolveOrBuildPrompt`, `writePromptFile`, `renderPrompt`,
  `buildPrompt`, and `from_spec.skills` are all deleted.

## Consequences

**Good.** Skill routing becomes per-unit-of-work rather than per-repo, which is what was
actually wanted, and it carries the reasoning that makes it effective. The designation is
durable: recorded on the Ticket, so it survives a re-run, works for `from-spec-bulk` without
per-Ticket flags, and is visible and editable in the Tracker. Treepad's surface *shrinks* — one
new command and one config line replace ~130 lines of templating and a config key. A Playbook
is versioned, reviewable, and improvable centrally once better wording is discovered.

**Bad.** The routing decision now lives in the harness, so treepad cannot guarantee a Playbook
was honoured — a Ticket naming a Playbook that does not exist fails inside the agent, not as a
treepad error, mirroring ADR 0001's missing-MCP failure mode. A Playbook cannot be fired by
hand as `/name`, because it is not a Skill. Tickets cut before a Playbook existed carry no
designation. And `to-tickets` must learn to emit the `Playbook:` line, which is work outside
this repo.

**Also.** Treepad now writes into `.claude/`, a directory owned by the harness — a boundary it
previously only read from via `[sync]`. And `tp playbook new` is a scaffolding command whose
output treepad never consumes, which is unusual enough to be worth this ADR.

## Alternatives considered

**Keep `from_spec.skills` as-is.** Rejected: repo-scoped and static, so it can only express
Standing Skills — the "same Skills for every task" behaviour being replaced. The author's own
global config has `skills = []`, so the mechanism was already unused.

**Playbook as a Skill with `disable-model-invocation: true`.** The first shape considered, and
it would allow `/task-dashboard` by hand. Rejected on contradiction: that flag removes the Skill
from the model's list, so `implement` could not delegate to it. Keeping it model-invocable puts
it back in the matching pool — the interference being avoided.

**Prose templates in `[from_spec.directives]`.** Reusable and carries the reasoning, but it is a
prose-authoring layer relocated from `PROMPT.md` into `config.toml`, with no frontmatter, no
progressive disclosure, and no way to test it. Rejected as the same mistake in a new file.

**Nothing in treepad — a `[sync]` line and an editor.** The smallest option, and defensible: the
agent that worked out the routing is well placed to write the file. Rejected for ergonomics —
without a command the convention is undiscoverable and the path easy to get wrong.

**Overload `tp sync <name> <body>`.** Authoring and propagation atomically coupled in one
keystroke. Rejected because positional arguments would silently change the verb, making it
impossible to re-sync without re-authoring or to fix a Playbook typo without a sync.

**`--playbook` flag plus `default_playbook` config.** Explicit at launch and visible in shell
history. Rejected once the Ticket became the carrier: the flag records nothing durable and would
have to be typed per-Ticket for bulk runs.
