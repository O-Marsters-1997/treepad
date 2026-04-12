# Commands

## workspace

Syncs editor configs and generates `.code-workspace` files across all git worktrees. Works with VS Code, Cursor, and Windsurf out of the box.

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

# Include extra file patterns in the sync
treepad workspace --include ".prettierrc" --include "*.md"
```

### Configuration

See [configuration.md](configuration.md) for the full schema, defaults, and examples.
