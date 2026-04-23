# from-spec

`tp from-spec` automates the full setup for an agentic coding session. Given a GitHub issue number, it creates a worktree, writes a structured `PROMPT.md` into it, and hands off to a configured agent command. When you run it, your agent is already inside a clean branch with the right files synced and the prompt ready.

```
tp from-spec [options] <branch>
```

## What it does

1. **Resolves the spec** — fetches the issue body via the `gh` CLI.
2. **Creates a worktree** — runs `git worktree add -b <branch> <path> <base>`, syncs editor configs from the main worktree, and writes an artifact file (same as `tp new`).
3. **Writes `PROMPT.md`** — assembles a structured prompt from the spec body, any configured skills, and optional custom instructions, then writes it to the worktree root.
4. **Runs the agent** — executes the configured `agent_command` inside the new worktree (default: `claude PROMPT.md`).
5. **Navigates into the worktree** — emits a `__TREEPAD_CD__` directive so the shell wrapper cd's you in automatically (skipped with `--current`).

If `PROMPT.md` already exists in the target worktree, it is used as-is and steps 1 and 3 are skipped.

## Prerequisites

**Shell integration** — required for automatic cd. Add to your `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(tp shell-init)"
```

**`gh` CLI** — required when using `--issue`. Must be authenticated and have read access to the repo:

```sh
gh auth login
gh auth status
```

**Agent installed** — the default `agent_command` calls `claude`. Install Claude Code if you haven't:

```sh
npm install -g @anthropic-ai/claude-code
```

## Flags

| Flag        | Short | Description                                                                          |
| ----------- | ----- | ------------------------------------------------------------------------------------ |
| `--issue`   | `-i`  | GitHub issue number to use as the spec (required)                                    |
| `--base`    | `-b`  | Ref to branch the new worktree from (default: `main`)                                |
| `--current` | `-c`  | Stay in the current directory instead of cd-ing into the new worktree                |
| `--prompt`  | `-p`  | Custom instructions appended to the prompt body (replaces "Implement the ticket.") |

`--issue` is required.

## Prompt structure

`PROMPT.md` always follows this shape:

```markdown
# <branch>

## Spec
<issue body or file contents>

## Skills
- /skill-name-1
- /skill-name-2

Implement the ticket.
```

The `## Skills` section is omitted when no skills are configured.

When `--prompt` is supplied, the closing line becomes:

```markdown
Implement the ticket according to the following instructions:

<your --prompt text>
```

## Configuration

`from-spec` behavior is controlled by the `[from_spec]` section in `.treepad.toml`:

```toml
[from_spec]
skills        = ["golang-patterns", "golang-testing"]
agent_command = ["claude", "{{.PromptPath}}"]
```

### Fields

| Field           | Type     | Description                                                                                          |
| --------------- | -------- | ---------------------------------------------------------------------------------------------------- |
| `skills`        | string[] | Skill names written into `PROMPT.md` under `## Skills`. Omitted when empty.                         |
| `agent_command` | string[] | Command to run after `PROMPT.md` is written. Each element is a Go `text/template` string. When absent or empty, `tp from-spec` writes `PROMPT.md` and exits — useful for inspecting the prompt before running an agent. |

**Default** (when no `[from_spec]` section is present):

```toml
[from_spec]
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
| `{{.Spec}}`        | Raw spec body (issue or file contents)                       |
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
# Create a worktree from a GitHub issue and launch the agent
tp from-spec feat/auth-refresh --issue 42

# Append custom instructions (overrides "Implement the ticket.")
tp from-spec fix/rate-limiter --issue 88 --prompt "focus on the Redis path, ignore the in-memory fallback"

# Branch from a non-default base
tp from-spec feat/new-thing --issue 99 --base develop

# Stay in the current directory (don't cd in)
tp from-spec feat/background --issue 12 --current
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
skills = ["golang-patterns", "golang-testing"]
agent_command = ["claude", "{{.PromptPath}}"]
```

`skills` tells `tp from-spec` to include `/golang-patterns` and `/golang-testing` in every generated prompt. The agent will invoke those skills automatically when it reads the prompt.

`agent_command` launches `claude` with `PROMPT.md` as its initial input, which is the standard way to hand off a fully-formed prompt to Claude Code.

### 2. Write a spec as a GitHub issue

Create a GitHub issue — or use an existing one. The issue body becomes the `## Spec` section of `PROMPT.md` verbatim, so write it as you would any task description: acceptance criteria, constraints, context links. Markdown formatting is preserved.

Example issue body:

```markdown
Add a `/health` endpoint to the HTTP server.

- Return `{"status":"ok","version":"<git-sha>"}` as JSON
- Use the short commit SHA from `git rev-parse --short HEAD`
- Endpoint must respond in < 5ms under no load (add a benchmark)
- Wire it up in `server.go`, not a separate file
```

Say this was filed as issue `#57`.

### 3. Run `tp from-spec`

```sh
tp from-spec feat/health-endpoint --issue 57
```

`tp` will:

1. Fetch the issue body with `gh issue view 57 --json body`
2. Create worktree at `../myrepo-feat-health-endpoint` branched from `main`
3. Sync `.claude/`, `.env`, and `.vscode/settings.json` into it
4. Write `PROMPT.md`:

   ```markdown
   # feat/health-endpoint

   ## Spec
   Add a `/health` endpoint to the HTTP server.

   - Return `{"status":"ok","version":"<git-sha>"}` as JSON
   - Use the short commit SHA from `git rev-parse --short HEAD`
   - Endpoint must respond in < 5ms under no load (add a benchmark)
   - Wire it up in `server.go`, not a separate file

   ## Skills
   - /golang-patterns
   - /golang-testing

   Implement the ticket.
   ```

5. Run `claude PROMPT.md` inside the new worktree
6. cd your shell into `../myrepo-feat-health-endpoint`

Claude Code reads `PROMPT.md`, invokes `/golang-patterns` and `/golang-testing` (which load domain-specific guidance about idiomatic Go and table-driven tests), then implements the ticket.

### 4. Steer the agent with `--prompt`

If you want to add constraints without editing the issue, pass them via `--prompt`:

```sh
tp from-spec feat/health-endpoint --issue 57 \
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
skills        = ["golang-patterns", "golang-testing"]
agent_command = []
```

Then:

```sh
tp from-spec feat/health-endpoint --issue 57
# tp creates the worktree and writes PROMPT.md, then exits
cat ../myrepo-feat-health-endpoint/PROMPT.md
# review it, then:
cd ../myrepo-feat-health-endpoint
claude PROMPT.md
```

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
