# Configuration

`tp` works with zero configuration. To customize behavior, add a `.treepad.toml` file to your repo root or write a global config file.

## Configuration Format

Configuration uses TOML format. A minimal config looks like:

```toml
[sync]
files = [".env", ".vscode/settings.json"]

[artifact]
filename = "myrepo-{{.Branch}}.code-workspace"
content = "..."

[open]
command = ["open", "{{.ArtifactPath}}"]
```

## Schema

### `[sync]` section

File patterns to copy from the source worktree to all other worktrees.

| Field     | Type     | Description                                                     |
| --------- | -------- | --------------------------------------------------------------- |
| `include` | string[] | Gitignore-style patterns of files/dirs to sync across worktrees |

Patterns use gitignore syntax: `**` matches across directories, a trailing `/` matches a directory and all its contents, and a `!` prefix negates (excludes) a pattern.

When `sync.include` is set, it **replaces** the defaults entirely. Use the `--include` flag to append additional patterns to whatever `sync.include` resolves to.

Playbooks live at `.claude/playbooks/**` and are propagated by this mechanism — the default `.claude/` pattern already covers them, but a config that narrows `.claude/` must add `".claude/playbooks/**"` explicitly.

**Default patterns** (used when no `[sync]` section or empty `include` array):

- `.claude/`
- `.agents/`
- `node_modules/`
- `.env`
- `.env.docker-compose`
- `.vscode/settings.json`
- `.vscode/tasks.json`
- `.vscode/launch.json`
- `.vscode/extensions.json`
- `.vscode/*.code-snippets`

### `[artifact]` section

Per-worktree file to generate (e.g., VS Code `.code-workspace` files, JetBrains `.idea` config, etc.). Both fields are Go text/template strings evaluated against the template context.

Leaving the section out does **not** skip generation — an absent or empty `[artifact]` section falls back to the built-in VS Code defaults below. To generate something other than a `.code-workspace`, override `filename` and `content`; to skip generation for a single run, use `tp sync --sync-only`.

| Field      | Type   | Description                                                       |
| ---------- | ------ | ----------------------------------------------------------------- |
| `filename` | string | Template for the artifact filename (relative to output directory) |
| `content`  | string | Template for the artifact file content                            |

**Default** (when no `[artifact]` section is present):

```toml
[artifact]
filename = "{{.Slug}}-{{.Branch}}.code-workspace"
content = """{
  "folders": [
    {{- range $i, $w := .Worktrees}}
    {{- if $i}},{{end}}
    {"name": "{{$w.Branch}}", "path": "{{$w.RelPath}}"}
    {{- end}}
  ]
}
"""
```

### `[hooks]` section

Shell commands to run at lifecycle points in `tp` operations. See [hooks.md](hooks.md) for the full reference.

Each event is declared as an array of hook entries using TOML array-of-tables syntax (`[[hooks.<event>]]`). Each entry has a `command` field (required) and optional `only`/`except` branch-filter arrays. An empty or absent list is a no-op.

| Field         | When it fires                              | Blocks on failure |
| ------------- | ------------------------------------------ | ----------------- |
| `pre_new`     | Before `git worktree add`                  | Yes               |
| `post_new`    | After artifact file is written             | No (warning)      |
| `pre_remove`  | Before `git worktree remove`               | Yes               |
| `post_remove` | After `git branch -d`                      | No (warning)      |
| `pre_sync`    | Before each worktree's file sync           | Yes               |
| `post_sync`   | After each worktree's file sync            | No (warning)      |
| `post_config_init` | After `tp config init` writes the config | No (warning)  |

```toml
[[hooks.post_new]]
command = "direnv allow {{.WorktreePath}}"

[[hooks.pre_remove]]
command = "git -C {{.WorktreePath}} diff --exit-code HEAD"
```

### `[open]` section

Command to run when `tp new --open` is used. Each element is a Go text/template string evaluated against the open context.

| Field     | Type     | Description                                                    |
| --------- | -------- | -------------------------------------------------------------- |
| `command` | string[] | Command template slice (e.g., `["open", "{{.ArtifactPath}}"]`) |

**Default**:

```toml
[open]
command = ["open", "{{.ArtifactPath}}"]
```

### `[diff]` section

Configuration for the `tp diff` command and the `d` key in `tp ui`. Controls the base ref used for diffing.

| Field | Type | Description |
|-------|------|-------------|
| `base` | string | Git ref to diff against (default: `origin/main`); used by `tp diff` when `--base` is not specified, and by the TUI `d` key binding |

**Default** (when no `[diff]` section is present):

```toml
[diff]
base = "origin/main"
```

**Examples:**

```toml
[diff]
base = "main"
```

```toml
[diff]
base = "develop"
```

### `[from_spec]` section

Configuration for ticket-driven worktrees — `tp new --ticket` and `tp from-spec-bulk`. See [from-spec.md](from-spec.md) for the full workflow.

| Field | Type | Description |
|-------|------|-------------|
| `ticket_url` | string | Go text/template expanding a bare Ref into a Ticket URL, with `{{.Ref}}` as the only variable — e.g. `"https://linear.app/acme/issue/{{.Ref}}"`. No default: when empty, only full Ticket URLs resolve |
| `agent_command` | string[] | Command to invoke once the worktree exists. Each element is a Go text/template string. Available variables: `.Branch`, `.Slug`, `.WorktreePath`, `.TicketURL`. Empty or absent means create the worktree and exit |

Treepad hands the agent the Ticket URL and nothing else: it never reads the Tracker, so the agent retrieves the Spec itself. See [ADR 0001](adr/0001-treepad-does-not-read-trackers.md) and [ADR 0002](adr/0002-treepad-writes-playbooks-not-prompts.md).

**Default** (when no `[from_spec]` section is present):

```toml
[from_spec]
ticket_url = ""
agent_command = ["claude", "{{.TicketURL}}"]
```

If `agent_command` is not configured, `tp new --ticket` creates the worktree and exits so you can invoke an agent manually.

## Template Context

Templates in `[artifact]` and `[open]` sections have access to the following variables:

### Artifact context (filename and content templates)

Available in `[artifact]` templates:

- `{{.Slug}}` — Repository slug (sanitized repo directory name, e.g., `myrepo`)
- `{{.Branch}}` — Sanitized branch name for this artifact (slashes replaced with dashes, e.g., `feature-x` from `feature/x`)
- `{{.Worktrees}}` — Slice of worktrees (each has `.Name`, `.Path`, `.RelPath`, `.Branch`)
  - `.Name` — Sanitized branch name (safe for filenames)
  - `.Path` — Absolute path on disk
  - `.RelPath` — Path relative to the artifact output directory
  - `.Branch` — Raw branch name
- `{{.OutputDir}}` — Absolute path of the artifact output directory (e.g., `~/myrepo-workspaces`)

### Open context

Available in `[open].command` templates:

- `{{.ArtifactPath}}` — Absolute path of the generated artifact file (or the worktree path if no `[artifact]` section is configured)

## Configuration Resolution

Configuration is resolved in the following order (first match wins):

1. **Local config** — `.treepad.toml` in the main worktree root (the directory containing `.git`)
2. **Global config** — checked in this order:
   - `$TREEPAD_CONFIG` environment variable (if set)
   - `$XDG_CONFIG_HOME/treepad/config.toml` (if `$XDG_CONFIG_HOME` is set)
   - `~/.config/treepad/config.toml` (fallback)
3. **Built-in defaults** — used when no config files are present

Use `tp config show` to see which configuration source is being used.

## Example Configurations

### VS Code (default)

This is the default configuration. It generates `.code-workspace` files that integrate with VS Code, Cursor, and Windsurf.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
  ".vscode/settings.json",
  ".vscode/tasks.json",
  ".vscode/launch.json",
  ".vscode/extensions.json",
  ".vscode/*.code-snippets",
]

[artifact]
filename = "{{.Slug}}-{{.Branch}}.code-workspace"
content = """
{
  "folders": [
    {{- range $i, $w := .Worktrees}}
    {{- if $i}},{{end}}
    {"name": "{{$w.Branch}}", "path": "{{$w.RelPath}}"}
    {{- end}}
  ]
}
"""

[open]
command = ["open", "{{.ArtifactPath}}"]
```

Run `tp config init` to write this configuration.

### JetBrains IDEs (IntelliJ IDEA, GoLand, WebStorm, etc.)

JetBrains IDEs store workspace configuration in `.idea/`, so skip artifact generation. Use the IDE's CLI for opening.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[open]
command = ["idea", "{{.ArtifactPath}}"]
```

Or use `goland`, `webstorm`, `clion`, etc., depending on your IDE.

### Zed

Zed supports multi-root workspaces via `.zed/workspaces.json`. Skip artifact generation and open the worktree directory.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[open]
command = ["zed", "{{.ArtifactPath}}"]
```

### Neovim

Neovim doesn't require workspace files. Sync configs and open the directory or a terminal.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[open]
command = ["nvim", "{{.ArtifactPath}}"]
```

Or use your preferred terminal/shell, e.g., `["kitty", "{{.ArtifactPath}}"]`.

### Helix

Similar to Neovim, Helix doesn't need workspace files.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[open]
command = ["hx", "{{.ArtifactPath}}"]
```

### Sublime Text

Sublime Text uses `.sublime-project` files for workspace configuration.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[artifact]
filename = "{{.Slug}}-{{.Branch}}.sublime-project"
content = """
{
  "folders": [
    {{- range $i, $w := .Worktrees}}
    {{- if $i}},{{end}}
    {"path": "{{$w.RelPath}}", "name": "{{$w.Branch}}"}
    {{- end}}
  ]
}
"""

[open]
command = ["subl", "{{.ArtifactPath}}"]
```

### No artifact generation

To skip artifact generation and only sync files (useful for terminal-based workflows), omit the `[artifact]` section.

```toml
[sync]
files = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".env.docker-compose",
]

[open]
command = ["tmux", "new-session", "-c", "{{.ArtifactPath}}"]
```
