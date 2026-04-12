# Commands

## workspace

Syncs VS Code configs and generates `.code-workspace` files across all git worktrees.

```
treepad workspace [options] [source-path]
```

By default, uses the main worktree (the one with a `.git` directory) as the config source. Configs from `.vscode/`, `.claude/`, and `.env` files are copied to every other worktree.

**Source resolution precedence:**
1. Explicit `source-path` argument
2. `--use-current` flag (current directory)
3. Auto-detected main worktree

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--use-current` | `-c` | Use current directory as config source instead of the main worktree |
| `--sync-only` | | Sync configs only; skip `.code-workspace` file generation |
| `--output-dir` | `-o` | Directory for generated `.code-workspace` files (default: `~/<repo-slug>-workspaces/`) |
| `--editor` | | Editor adapter to use (default: `vscode`) |
| `--include` | | Additional file patterns to sync (appended to `sync.files` in `.treepad.json`) |

### Examples

```bash
# Generate .code-workspace files and sync configs from the main worktree
treepad workspace

# Sync configs only (no workspace files generated)
treepad workspace --sync-only

# Use the current directory as the config source
treepad workspace --use-current

# Write workspace files to a custom directory
treepad workspace --output-dir ~/my-workspaces

# Use an explicit repo path as the config source
treepad workspace /path/to/repo

# Use a specific editor adapter
treepad workspace --editor cursor

# Include extra file patterns in the sync
treepad workspace --include ".prettierrc" --include "*.md"
```

### Configuration

The optional `.treepad.json` file at the repo root controls default behaviour. All fields have defaults, so the file is never required.

```json
{
  "editor": "vscode",
  "sync": {
    "files": [".claude/settings.local.json", ".env", ".env.docker-compose"]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `editor` | string | Default editor adapter name (overridden by `--editor` flag) |
| `sync.files` | string[] | Glob patterns of files to sync across worktrees |

When `sync.files` is set, it **replaces** the defaults entirely. The `--include` flag appends additional patterns to whatever `sync.files` resolves to.

**Default synced files** (when no `.treepad.json` is present or `sync.files` is unset):

- `.claude/settings.local.json`
- `.env`
- `.env.docker-compose`
