# Commands

## workspace

Syncs editor configs and generates artifact files across all git worktrees. By default, generates VS Code `.code-workspace` files.

```
treepad workspace [options] [source-path]
```

By default, uses the main worktree (the one with a `.git` directory) as the config source. Configs from `.vscode/`, `.claude/`, and `.env` files are copied to every other worktree. The artifact file generated is controlled by `.treepad.toml` and can be customized for any editor.

**Source resolution precedence:**
1. Explicit `source-path` argument
2. `--use-current` flag (current directory)
3. Auto-detected main worktree

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--use-current` | `-c` | Use current directory as config source instead of the main worktree |
| `--sync-only` | | Sync configs only; skip artifact file generation |
| `--output-dir` | `-o` | Directory for generated artifact files (default: `~/<repo-slug>-workspaces/`) |
| `--include` | | Additional file patterns to sync (appended to `sync.files` in `.treepad.toml`) |

### Examples

```bash
# Generate artifact files and sync configs from the main worktree
treepad workspace

# Sync configs only (no artifact files generated)
treepad workspace --sync-only

# Use the current directory as the config source
treepad workspace --use-current

# Write artifact files to a custom directory
treepad workspace --output-dir ~/my-workspaces

# Use an explicit repo path as the config source
treepad workspace /path/to/repo

# Include extra file patterns in the sync
treepad workspace --include ".prettierrc" --include "*.md"
```

### Configuration

See [configuration.md](configuration.md) for the full schema, defaults, and examples.

## create

Create a new git worktree, sync configs from the main worktree, and generate an artifact file for it.

```
treepad create [options] <branch>
```

Creates a new worktree branched from a specified ref (default: `main`), syncs editor configs from the main worktree, and generates an artifact file as configured in `.treepad.toml`.

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--base` | | Ref to branch the new worktree from (default: `main`) |
| `--open` | `-o` | Open the generated artifact file (using the command specified in `[open].command`) |

### Examples

```bash
# Create a new worktree for branch 'feature-x' branched from main
treepad create feature-x

# Create a worktree from a different base ref
treepad create bugfix-y --base develop

# Create a worktree and open the generated artifact file
treepad create feature-z --open

# Shorthand with flags combined
treepad create my-branch --base main --open
```

## config

Manage treepad configuration files.

```
treepad config <subcommand>
```

### config init

Write a config file with default values.

```
treepad config init [--global]
```

By default, writes `.treepad.toml` to the main worktree root (the directory containing `.git`). Use the `--global` flag to write to the global config path instead.

#### Flags

| Flag | Description |
|------|-------------|
| `--global` | Write to the global config path instead of `.treepad.toml` in the main worktree |

#### Examples

```bash
# Write default config to the main worktree root
treepad config init

# Write default config to the global config path
treepad config init --global
```

### config show

Print the resolved config and which sources contributed.

```
treepad config show
```

Displays the final configuration that would be used, along with information about which source(s) contributed to it. Resolution order is:
1. Local `.treepad.toml` in the main worktree
2. Global config file (from `$TREEPAD_CONFIG`, `$XDG_CONFIG_HOME/treepad/config.toml`, or `~/.config/treepad/config.toml`)
3. Built-in defaults

#### Examples

```bash
# Show the resolved config and its sources
treepad config show
```

This will output something like:

```
Sources:
  local:  /path/to/repo/.treepad.toml

Config:
[sync]
files = [".claude/settings.local.json", ".env"]

[artifact]
filename = "myrepo-{{.Branch}}.code-workspace"
content = "..."
```

See [configuration.md](configuration.md) for details on the configuration schema and defaults.

## remove

Remove a git worktree, delete its artifact file, and delete the local branch.

```
treepad remove <branch>
```

Removes the worktree for the specified branch, cleans up its associated artifact file (if any), and deletes the branch locally. Includes pre-flight safety guards to prevent accidental data loss.

### Pre-flight guards

- Refuses to remove the main worktree
- Refuses to remove a worktree if you are currently inside it (must `cd` elsewhere first)

### Examples

```bash
# Remove a completed feature branch
treepad remove feature-x

# Remove after switching out of the worktree
cd ../main-repo  # or any other location
treepad remove feature-x
```

### Errors

Attempting to remove the main worktree or the worktree you're currently in will return an error:

```
cannot remove the main worktree
cannot remove the worktree you are currently in; cd elsewhere first
```
