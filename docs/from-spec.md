# Ticket-driven worktrees (`tp new --ticket`)

`tp new --ticket` automates the full setup for an agentic coding session. Given a Ticket, it creates a worktree and hands the Ticket URL to a configured agent command. When you run it, your agent is already inside a clean branch with the right files synced.

```
tp new [options] --ticket <ticket> <branch>
```

Terminology follows [CONTEXT.md](../CONTEXT.md): a **Ticket** is the unit of tracked work, a **Ref** is its Tracker-local identifier (`42`, `ENG-123`), a **Ticket URL** is its canonical web address, the **Spec** is the body it carries, and a **Playbook** is the prose saying which Skills the work should use.

## What it does

1. **Resolves the Ticket to a Ticket URL** — a URL is used verbatim; a bare Ref is rendered through `[from_spec] ticket_url`. This happens before anything is created, so an unresolvable Ticket leaves no orphan worktree behind.
2. **Creates a worktree** — runs `git worktree add -b <branch> <path> <base>`, syncs editor configs from the main worktree, and writes an artifact file (same as a plain `tp new`).
3. **Runs the agent** — executes the configured `agent_command` inside the new worktree (default: `claude {{.TicketURL}}`).
4. **Navigates into the worktree** — emits a `__TREEPAD_CD__` directive so the shell wrapper cd's you in automatically (skipped with `--current`).

**`tp` never reads the Tracker.** It holds no API token, makes no HTTP call, and shells out to no `gh`. It passes the Ticket URL and the agent retrieves the Spec itself. See [ADR 0001](adr/0001-treepad-does-not-read-trackers.md) for why.

**`tp` authors no prompt.** It composes no text, interpolates nothing into the work, and appends no instructions. See [ADR 0002](adr/0002-treepad-writes-playbooks-not-prompts.md).

## Prerequisites

**Shell integration** — required for automatic cd. Add to your `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(tp shell-init)"
```

**`ticket_url` configured** — required to pass bare Refs. Without it, only full Ticket URLs resolve. See [Configuration](#configuration).

**Agent access to the Tracker** — since `tp` cites rather than fetches, your agent needs its own route to the Ticket: a Linear or GitHub MCP server, the `gh` CLI, or plain web access. A missing connection surfaces as the agent failing mid-run, not as a `tp` error.

**Agent installed** — the default `agent_command` calls `claude`. Install Claude Code if you haven't:

```sh
npm install -g @anthropic-ai/claude-code
```

## Flags

| Flag        | Short | Description                                                                      |
| ----------- | ----- | ---------------------------------------------------------------------------------- |
| `--ticket`  | `-t`  | Ticket URL, or a bare Ref when `[from_spec] ticket_url` is configured             |
| `--base`    | `-b`  | Ref to branch the new worktree from (default: `main`)                            |
| `--current` | `-c`  | Stay in the current directory instead of cd-ing into the new worktree            |

`--ticket` cannot be combined with `--open`.

## Ticket resolution

Input starting with `http://` or `https://` is used verbatim, and its Ref is the last path segment. Anything else is treated as a Ref and rendered through the `[from_spec] ticket_url` Go template with `{{.Ref}}`:

```toml
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
```

With that set, `--ticket ENG-123` resolves to `https://linear.app/acme/issue/ENG-123`. A full URL always works and always overrides the configured Tracker, so a repo can pull a one-off Ticket from elsewhere.

Without `ticket_url`, a bare Ref is an error naming both fixes:

```
no ticket_url configured: cannot resolve "ENG-123".
  set [from_spec] ticket_url in .treepad.toml, or pass the full ticket URL.
```

## Playbooks

Skill routing — deciding which Skills a given unit of work should load, and saying why — is a Playbook's job, not a prompt's.

A **Playbook** is a plain Markdown document at `.claude/playbooks/<name>.md`. Write one with `tp playbook new`:

```sh
tp playbook new task-dashboard <<'EOF'
Use /impeccable for the visual layer — this is dashboard work, and the
default styling pass is not enough. Use /dataviz before writing any chart.
EOF
```

The body is written **verbatim**: treepad composes nothing and interpolates nothing. Then name the Playbook on the Ticket:

```
Playbook: task-dashboard
```

The agent already reads the Ticket, so it picks the name up for free. The designation is durable — it survives a re-run, works for `tp from-spec-bulk` without per-Ticket flags, and is visible and editable in the Tracker.

Playbooks propagate through the existing `[sync]` machinery. The built-in default already includes `.claude/`; a config that narrows it needs an explicit `".claude/playbooks/**"` entry.

## Configuration

Ticket-driven behaviour is controlled by the `[from_spec]` section in `.treepad.toml`:

```toml
[from_spec]
ticket_url    = "https://linear.app/acme/issue/{{.Ref}}"
agent_command = ["claude", "{{.TicketURL}}"]
```

### Fields

| Field           | Type     | Description                                                                                          |
| --------------- | -------- | ---------------------------------------------------------------------------------------------------- |
| `ticket_url`    | string   | Go `text/template` expanding a bare Ref into a Ticket URL, with `{{.Ref}}` as the only variable. No default — when empty, only full Ticket URLs resolve. |
| `agent_command` | string[] | Command to run once the worktree exists. Each element is a Go `text/template` string. When absent or empty, `tp new --ticket` creates the worktree and exits — useful for starting the agent by hand. |

**Default** (when no `[from_spec]` section is present):

```toml
[from_spec]
ticket_url    = ""
agent_command = ["claude", "{{.TicketURL}}"]
```

### Template variables in `agent_command`

Each element of `agent_command` is rendered as a Go `text/template` string before the command is executed:

| Variable            | Description                                     |
| ------------------- | ----------------------------------------------- |
| `{{.TicketURL}}`    | The resolved Ticket URL                         |
| `{{.WorktreePath}}` | Absolute path to the new worktree directory     |
| `{{.Branch}}`       | Branch name as passed to `tp new`               |
| `{{.Slug}}`         | Repository slug (sanitized repo directory name) |

A template referencing anything else — including the retired `{{.PromptPath}}` and `{{.Skills}}` — fails loudly with `execute agent_command template`.

## Hooks

`tp new --ticket` fires the same hooks as a plain `tp new`:

| Event       | When                                    |
| ----------- | --------------------------------------- |
| `pre_new`   | Before `git worktree add`               |
| `pre_sync`  | Before each file sync                   |
| `post_sync` | After each file sync                    |
| `post_new`  | After the artifact file is written      |

The agent is launched after all hooks have run. See [hooks.md](hooks.md) for the full reference.

## Examples

```bash
# Create a worktree from a Linear ticket and launch the agent
tp new feat/auth-refresh --ticket ENG-217

# Pass a full Ticket URL — works with no ticket_url configured, and overrides it when there is one
tp new feat/auth-refresh --ticket https://github.com/acme/api/issues/42

# Branch from a non-default base
tp new feat/new-thing --ticket ENG-99 --base develop

# Stay in the current directory (don't cd in)
tp new feat/background --ticket ENG-12 --current
```

## Walkthrough: shipping a ticket end to end

This section walks through a complete setup and session for a Go project using Claude Code.

### 1. Configure the project

In `.treepad.toml` at the repo root:

```toml
[sync]
include = [
  ".claude/",
  ".env",
  ".vscode/settings.json",
]

[from_spec]
ticket_url    = "https://linear.app/acme/issue/{{.Ref}}"
agent_command = ["claude", "{{.TicketURL}}"]
```

`ticket_url` declares this repo's Tracker by giving the URL shape its Refs expand into. That one line is all `tp` knows about Linear — swap it for `https://github.com/acme/api/issues/{{.Ref}}` and the same repo tracks work on GitHub instead.

`agent_command` launches `claude` with the Ticket URL as its initial input.

### 2. Write a spec on your Tracker

Create a Ticket — or use an existing one. Write the Spec as you would any task description: acceptance criteria, constraints, context links. `tp` never reads it, so nothing about its formatting matters to `tp`; it matters to the agent that fetches it.

Example Spec:

```markdown
Add a `/health` endpoint to the HTTP server.

- Return `{"status":"ok","version":"<git-sha>"}` as JSON
- Use the short commit SHA from `git rev-parse --short HEAD`
- Endpoint must respond in < 5ms under no load (add a benchmark)
- Wire it up in `server.go`, not a separate file

Playbook: go-service
```

Say this was filed as `ENG-57`.

### 3. Run `tp new --ticket`

```sh
tp new feat/health-endpoint --ticket ENG-57
```

`tp` will:

1. Resolve `ENG-57` to `https://linear.app/acme/issue/ENG-57`
2. Create worktree at `../myrepo-feat-health-endpoint` branched from `main`
3. Sync `.claude/`, `.env`, and `.vscode/settings.json` into it — including `.claude/playbooks/go-service.md`
4. Run `claude https://linear.app/acme/issue/ENG-57` inside the new worktree
5. cd your shell into `../myrepo-feat-health-endpoint`

Claude Code fetches `ENG-57` through its Linear connection, reads the `Playbook: go-service` line, loads `.claude/playbooks/go-service.md`, and implements the ticket.

### 4. Steer the agent

To add constraints, edit the Ticket or the Playbook — both are durable and both survive a re-run. `tp` offers no flag for one-off prose, deliberately: instructions typed at the CLI are lost the moment the shell history rolls over.

### 5. Start the agent by hand

Set `agent_command = []` (or omit it entirely) to create the worktree without launching an agent:

```toml
[from_spec]
ticket_url    = "https://linear.app/acme/issue/{{.Ref}}"
agent_command = []
```

Then:

```sh
tp new feat/health-endpoint --ticket ENG-57
# tp creates the worktree and exits
cd ../myrepo-feat-health-endpoint
claude https://linear.app/acme/issue/ENG-57
```

### 6. Use a custom agent command

`agent_command` is a template slice, so you can pass arbitrary flags or wrap the invocation:

```toml
[from_spec]
# Run claude in a new tmux window, not inline
agent_command = [
  "tmux", "new-window", "-c", "{{.WorktreePath}}",
  "claude {{.TicketURL}}",
]
```

Or pass additional Claude Code flags:

```toml
[from_spec]
agent_command = ["claude", "--allowedTools", "Edit,Write,Bash", "{{.TicketURL}}"]
```

### 7. Skip the agent for bulk prep

Use `tp from-spec-bulk` when you want to prepare multiple worktrees without launching agents immediately. See the [commands reference](commands.md#from-spec-bulk) for details.
