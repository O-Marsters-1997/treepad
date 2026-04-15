# Configuration

`treepad` works with zero configuration. To customize behavior, add a `.treepad.toml` file to your repo root or write a global config file.

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

| Field | Type | Description |
|-------|------|-------------|
| `files` | string[] | Glob patterns of files to sync across worktrees |

When `sync.files` is set, it **replaces** the defaults entirely. Use the `--include` flag to append additional patterns to whatever `sync.files` resolves to.

**Default files** (used when no `[sync]` section or empty `files` array):
- `.claude/settings.local.json`
- `.env`
- `.env.docker-compose`
- `.vscode/settings.json`
- `.vscode/tasks.json`
- `.vscode/launch.json`
- `.vscode/extensions.json`
- `.vscode/*.code-snippets`

### `[artifact]` section

Per-worktree file to generate (e.g., VS Code `.code-workspace` files, JetBrains `.idea` config, etc.). Both fields are Go text/template strings evaluated against the template context. Leave the `[artifact]` section out entirely to skip artifact generation.

| Field | Type | Description |
|-------|------|-------------|
| `filename` | string | Template for the artifact filename (relative to output directory) |
| `content` | string | Template for the artifact file content |

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

### `[open]` section

Command to run when `treepad create --open` is used. Each element is a Go text/template string evaluated against the open context.

| Field | Type | Description |
|-------|------|-------------|
| `command` | string[] | Command template slice (e.g., `["open", "{{.ArtifactPath}}"]`) |

**Default**:
```toml
[open]
command = ["open", "{{.ArtifactPath}}"]
```

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

Use `treepad config show` to see which configuration source is being used.

## Migration from JSON

If you have a legacy `.treepad.json` file, `treepad` will show an error message:

```
found .treepad.json but treepad now uses TOML; move your config to .treepad.toml or re-run `treepad config init`
```

Run `treepad config init` to generate a new `.treepad.toml` with defaults, then manually add any custom settings from your old JSON file.

## Example Configurations

### VS Code (default)

This is the default configuration. It generates `.code-workspace` files that integrate with VS Code, Cursor, and Windsurf.

```toml
[sync]
files = [
  ".claude/settings.local.json",
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

Run `treepad config init` to write this configuration.

### JetBrains IDEs (IntelliJ IDEA, GoLand, WebStorm, etc.)

JetBrains IDEs store workspace configuration in `.idea/`, so skip artifact generation. Use the IDE's CLI for opening.

```toml
[sync]
files = [
  ".claude/settings.local.json",
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
  ".env",
  ".env.docker-compose",
]

[open]
command = ["tmux", "new-session", "-c", "{{.ArtifactPath}}"]
```
