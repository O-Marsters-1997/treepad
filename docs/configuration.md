# Configuration

`treepad` works with zero configuration. To customise behaviour, add a `.treepad.json` file to your repo root or write a global config file.

## Schema

```json
{
  "sync": {
    "files": [".claude/settings.local.json", ".env", ".env.docker-compose"]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `sync.files` | string[] | Glob patterns of files to sync across worktrees |

When `sync.files` is set, it **replaces** the defaults entirely. The `--include` flag appends additional patterns to whatever `sync.files` resolves to.

## Configuration Resolution

Configuration is resolved in the following order (first match wins):

1. **Local config** — `.treepad.json` in the main worktree root
2. **Global config** — checked in this order:
   - `$TREEPAD_CONFIG` environment variable (if set)
   - `$XDG_CONFIG_HOME/treepad/config.json` (if `$XDG_CONFIG_HOME` is set)
   - `~/.config/treepad/config.json` (fallback)
3. **Built-in defaults** — used when no config files are present

Use `treepad config show` to see which configuration source is being used.

## Default synced files

Used when no `.treepad.json` is present or `sync.files` is unset:

- `.claude/settings.local.json`
- `.env`
- `.env.docker-compose`
- `.vscode/settings.json`
- `.vscode/tasks.json`
- `.vscode/launch.json`
- `.vscode/extensions.json`
- `.vscode/*.code-snippets`
