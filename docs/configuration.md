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

**Default patterns** (used when no `[sync]` section or empty `include` array):

- `.claude/`
- `node_modules/`
- `.env`
- `.env.docker-compose`
- `.vscode/settings.json`
- `.vscode/tasks.json`
- `.vscode/launch.json`
- `.vscode/extensions.json`
- `.vscode/*.code-snippets`

### `[artifact]` section

Per-worktree file to generate (e.g., VS Code `.code-workspace` files, JetBrains `.idea` config, etc.). Both fields are Go text/template strings evaluated against the template context. Leave the `[artifact]` section out entirely to skip artifact generation.

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

Each field is a list of shell command strings (Go `text/template` strings rendered before execution). An empty or absent list is a no-op.

| Field         | Type     | Description                                                          |
| ------------- | -------- | -------------------------------------------------------------------- |
| `pre_new`     | string[] | Run before `git worktree add`; non-zero exit aborts the operation    |
| `post_new`    | string[] | Run after artifact file is written; failure logs a warning           |
| `pre_remove`  | string[] | Run before `git worktree remove`; non-zero exit aborts the operation |
| `post_remove` | string[] | Run after `git branch -d`; failure logs a warning                    |
| `pre_sync`    | string[] | Run before each worktree's file sync; non-zero exit aborts that sync |
| `post_sync`   | string[] | Run after each worktree's file sync; failure logs a warning          |

```toml
[hooks]
post_new   = ["direnv allow {{.WorktreePath}}"]
pre_remove = ["git -C {{.WorktreePath}} diff --exit-code HEAD"]
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
