# Configuration

`treepad` works with zero configuration. To customise behaviour, add a `.treepad.json` file to your repo root.

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
