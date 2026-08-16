# from-spec

`tp from-spec` automates the full setup for an agentic coding session. Given a Ticket, it creates a worktree, writes a structured `PROMPT.md` into it, and hands off to a configured agent command. When you run it, your agent is already inside a clean branch with the right files synced and the prompt ready.

```
tp from-spec [options] <branch>
```

Terminology follows [CONTEXT.md](../CONTEXT.md): a **Ticket** is the unit of tracked work, a **Ref** is its Tracker-local identifier (`42`, `ENG-123`), a **Ticket URL** is its canonical web address, and the **Spec** is the body it carries.

## What it does

1. **Resolves the Ticket to a Ticket URL** — a URL is used verbatim; a bare Ref is rendered through `[from_spec] ticket_url`. This happens before anything is created, so an unresolvable Ticket leaves no orphan worktree behind.
2. **Creates a worktree** — runs `git worktree add -b <branch> <path> <base>`, syncs editor configs from the main worktree, and writes an artifact file (same as `tp new`).
3. **Writes `PROMPT.md`** — assembles a structured prompt citing the Ticket URL, any configured skills, and optional custom instructions, then writes it to the worktree root.
4. **Runs the agent** — executes the configured `agent_command` inside the new worktree (default: `claude PROMPT.md`).
5. **Navigates into the worktree** — emits a `__TREEPAD_CD__` directive so the shell wrapper cd's you in automatically (skipped with `--current`).

If `PROMPT.md` already exists in the target worktree, it is used as-is and step 3 is skipped.

**`tp` never reads the Tracker.** It holds no API token, makes no HTTP call, and shells out to no `gh`. `PROMPT.md` cites the Ticket URL and the agent retrieves the Spec itself. See [ADR 0001](adr/0001-treepad-does-not-read-trackers.md) for why.

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

| Flag        | Short | Description                                                                          |
| ----------- | ----- | ------------------------------------------------------------------------------------ |
| `--ticket`  | `-t`  | Ticket URL, or a bare Ref when `[from_spec] ticket_url` is configured (required)     |
| `--base`    | `-b`  | Ref to branch the new worktree from (default: `main`)                                |
| `--current` | `-c`  | Stay in the current directory instead of cd-ing into the new worktree                |
| `--prompt`  | `-p`  | Custom instructions appended to the prompt body (replaces "Implement the ticket.") |

`--ticket` is required.

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

## Prompt structure

`PROMPT.md` always follows this shape:

```markdown
# <branch>

## Spec
Read the ticket at:
<ticket-url>

## Skills
- /skill-name-1
- /skill-name-2

Implement the ticket.
```

The `## Spec` section is a citation, not the Spec — retrieving the body is the agent's job. The `## Skills` section is omitted when no skills are configured.

When `--prompt` is supplied, the closing line becomes:

```markdown
Implement the ticket according to the following instructions:

<your --prompt text>
```

## Configuration

`from-spec` behavior is controlled by the `[from_spec]` section in `.treepad.toml`:

```toml
[from_spec]
ticket_url    = "https://linear.app/acme/issue/{{.Ref}}"
skills        = ["golang-patterns", "golang-testing"]
agent_command = ["claude", "{{.PromptPath}}"]
```

### Fields

| Field           | Type     | Description                                                                                          |
| --------------- | -------- | ---------------------------------------------------------------------------------------------------- |
| `ticket_url`    | string   | Go `text/template` expanding a bare Ref into a Ticket URL, with `{{.Ref}}` as the only variable. No default — when empty, only full Ticket URLs resolve. |
| `skills`        | string[] | Skill names written into `PROMPT.md` under `## Skills`. Omitted when empty.                         |
| `agent_command` | string[] | Command to run after `PROMPT.md` is written. Each element is a Go `text/template` string. When absent or empty, `tp from-spec` writes `PROMPT.md` and exits — useful for inspecting the prompt before running an agent. |

**Default** (when no `[from_spec]` section is present):

```toml
[from_spec]
ticket_url    = ""
skills        = []
agent_command = ["claude", "{{.PromptPath}}"]
```

### Template variables in `agent_command`

Each element of `agent_command` is rendered as a Go `text/template` string before the command is executed:

| Variable           | Description                                                  |
| ------------------ | ------------------------------------------------------------ |
| `{{.PromptPath}}`  | Absolute path to `PROMPT.md` in the new worktree             |
| `{{.WorktreePath}}`| Absolute path to the new worktree directory                  |
| `{{.Branch}}`      | Branch name as passed to `tp from-spec`                      |
| `{{.Slug}}`        | Repository slug (sanitized repo directory name)              |
| `{{.Spec}}`        | The `## Spec` body — the Ticket URL citation, not the Spec itself |
| `{{.Skills}}`      | Slice of skill names from config (same as `from_spec.skills`)|
| `{{.Prompt}}`      | Fully rendered prompt body (the text written to `PROMPT.md`) |

## Hooks

`from-spec` fires the same hooks as `tp new`:

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
tp from-spec feat/auth-refresh --ticket ENG-217

# Pass a full Ticket URL — works with no ticket_url configured, and overrides it when there is one
tp from-spec feat/auth-refresh --ticket https://github.com/acme/api/issues/42

# Append custom instructions (overrides "Implement the ticket.")
tp from-spec fix/rate-limiter --ticket ENG-88 --prompt "focus on the Redis path, ignore the in-memory fallback"

# Branch from a non-default base
tp from-spec feat/new-thing --ticket ENG-99 --base develop

# Stay in the current directory (don't cd in)
tp from-spec feat/background --ticket ENG-12 --current
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
skills        = ["golang-patterns", "golang-testing"]
agent_command = ["claude", "{{.PromptPath}}"]
```

`ticket_url` declares this repo's Tracker by giving the URL shape its Refs expand into. That one line is all `tp` knows about Linear — swap it for `https://github.com/acme/api/issues/{{.Ref}}` and the same repo tracks work on GitHub instead.

`skills` tells `tp from-spec` to include `/golang-patterns` and `/golang-testing` in every generated prompt. The agent will invoke those skills automatically when it reads the prompt.

`agent_command` launches `claude` with `PROMPT.md` as its initial input, which is the standard way to hand off a fully-formed prompt to Claude Code.

### 2. Write a spec on your Tracker

Create a Ticket — or use an existing one. Write the Spec as you would any task description: acceptance criteria, constraints, context links. `tp` never reads it, so nothing about its formatting matters to `tp`; it matters to the agent that fetches it.

Example Spec:

```markdown
Add a `/health` endpoint to the HTTP server.

- Return `{"status":"ok","version":"<git-sha>"}` as JSON
- Use the short commit SHA from `git rev-parse --short HEAD`
- Endpoint must respond in < 5ms under no load (add a benchmark)
- Wire it up in `server.go`, not a separate file
```

Say this was filed as `ENG-57`.

### 3. Run `tp from-spec`

```sh
tp from-spec feat/health-endpoint --ticket ENG-57
```

`tp` will:

1. Resolve `ENG-57` to `https://linear.app/acme/issue/ENG-57`
2. Create worktree at `../myrepo-feat-health-endpoint` branched from `main`
3. Sync `.claude/`, `.env`, and `.vscode/settings.json` into it
4. Write `PROMPT.md`:

   ```markdown
   # feat/health-endpoint

   ## Spec
   Read the ticket at:
   https://linear.app/acme/issue/ENG-57

   ## Skills
   - /golang-patterns
   - /golang-testing

   Implement the ticket.
   ```

5. Run `claude PROMPT.md` inside the new worktree
6. cd your shell into `../myrepo-feat-health-endpoint`

Claude Code reads `PROMPT.md`, fetches `ENG-57` through its Linear connection, invokes `/golang-patterns` and `/golang-testing` (which load domain-specific guidance about idiomatic Go and table-driven tests), then implements the ticket.

### 4. Steer the agent with `--prompt`

If you want to add constraints without editing the Ticket, pass them via `--prompt`:

```sh
tp from-spec feat/health-endpoint --ticket ENG-57 \
  --prompt "the version field must come from a build-time ldflags injection, not runtime git"
```

The generated closing block becomes:

```markdown
Implement the ticket according to the following instructions:

the version field must come from a build-time ldflags injection, not runtime git
```

### 5. Inspect the prompt before running the agent

Set `agent_command = []` (or omit it entirely) to write `PROMPT.md` without launching an agent:

```toml
[from_spec]
ticket_url    = "https://linear.app/acme/issue/{{.Ref}}"
skills        = ["golang-patterns", "golang-testing"]
agent_command = []
```

Then:

```sh
tp from-spec feat/health-endpoint --ticket ENG-57
# tp creates the worktree and writes PROMPT.md, then exits
cat ../myrepo-feat-health-endpoint/PROMPT.md
# review it, then:
cd ../myrepo-feat-health-endpoint
claude PROMPT.md
```

What you are reviewing is the branch name, the skills list, your `--prompt` instructions, and which Ticket is cited — not the Spec, which lives on the Tracker. Hand-editing `PROMPT.md` at this point is supported: an existing `PROMPT.md` is used as-is, so you can paste the Spec inline yourself and re-run without losing the edit.

### 6. Use a custom agent command

`agent_command` is a template slice, so you can pass arbitrary flags or wrap the invocation:

```toml
[from_spec]
# Run claude in a new tmux window, not inline
agent_command = [
  "tmux", "new-window", "-c", "{{.WorktreePath}}",
  "claude {{.PromptPath}}",
]
```

Or pass additional Claude Code flags:

```toml
[from_spec]
agent_command = ["claude", "--allowedTools", "Edit,Write,Bash", "{{.PromptPath}}"]
```

### 7. Skip the agent for bulk prep

Use `tp from-spec-bulk` when you want to prepare multiple worktrees without launching agents immediately. See the [commands reference](commands.md#from-spec-bulk) for details.
