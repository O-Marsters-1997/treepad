# .treepad.toml

`tp` works with zero config. Configure it when a repo needs extra files synced, a
non-VS-Code editor artifact, or work done at lifecycle events.

## Contents

- Resolution order
- `[sync]` — what gets copied into worktrees
- `[artifact]` — the per-worktree file generated
- `[open]`, `[diff]`, `[exec]`
- `[from_spec]`
- `[hooks]` — lifecycle commands
- Editor recipes

## Resolution order

First match wins, section by section:

1. `.treepad.toml` in the main worktree root (the directory containing `.git`)
2. Global config — `$TREEPAD_CONFIG`, then `$XDG_CONFIG_HOME/treepad/config.toml`, then `~/.config/treepad/config.toml`
3. Built-in defaults

`tp config show` prints the resolved result and which sources contributed. Run it before
editing anything, so you change the file that is actually winning.

`tp config init` writes a documented default file to the repo; `--global` targets the global
path; `--inherit` seeds from the global config instead of the built-in defaults, and
`--inherit --hooks-only` takes just `[hooks]` from it.

## `[sync]`

Gitignored files copied from the source worktree into every other worktree. Entries are
paths or globs relative to the repo root; a trailing `/` copies a directory tree.

```toml
[sync]
include = [
  ".claude/settings.local.json",
  ".agents/skills/",
  ".env",
  ".vscode/settings.json",
  ".vscode/*.code-snippets",
]
```

Setting `include` replaces the defaults rather than adding to them, so carry over the
entries you still want. `tp sync --include <pattern>` appends for a single run without
touching the file.

## `[artifact]`

The per-worktree file `sync` and `new` generate — by default a VS Code `.code-workspace`
listing every worktree as a folder, written to `~/<repo-slug>-workspaces/`.

An absent or empty `[artifact]` section falls back to those defaults rather than switching
generation off, so a JetBrains or Neovim user who wants no `.code-workspace` files has to
override `filename` and `content` with something harmless; `tp sync --sync-only` skips
generation for a single sync run.

```toml
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
```

Template data for `filename` and `content`:

| Variable | Meaning |
| --- | --- |
| `{{.Slug}}` | Repo slug, e.g. `myrepo` |
| `{{.Branch}}` | Branch with slashes replaced by dashes, e.g. `feature-x` |
| `{{.OutputDir}}` | Absolute artifact output directory |
| `{{.Worktrees}}` | Every worktree; each has `.Name`, `.Path`, `.RelPath`, `.Branch` |

`tp status` reads the artifact's mtime for its `TOUCHED` column and `last_touched` field, so
a repo with no `[artifact]` section shows `—` there. `tp doctor` reports `artifact-missing`
when a configured artifact is absent — `tp sync` regenerates it.

## `[open]`, `[diff]`, `[exec]`

```toml
[open]
command = ["open", "{{.ArtifactPath}}"]   # run by tp new --open; {{.ArtifactPath}} falls
                                          # back to the worktree dir with no [artifact]

[diff]
base = "origin/main"                      # default base for tp diff

[exec]
runner = "just"                           # force a runner instead of auto-detecting
```

## `[from_spec]`

```toml
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
agent_command = ["claude", "{{.TicketURL}}"]
```

`ticket_url` is what lets `--ticket ENG-123` work with a bare ref; without it, pass full
URLs. `agent_command` elements are Go templates over `.TicketURL`, `.Branch`, `.Slug` and
`.WorktreePath` — nothing else. With no `agent_command`, `tp new --ticket` creates the
worktree and exits. Details are in [from-spec.md](from-spec.md).

## `[hooks]`

Commands run at lifecycle points. Each entry is rendered as a Go template and passed to
`sh -c`.

| Event | Fires | Aborts on failure |
| --- | --- | --- |
| `pre_new` | Before `git worktree add` | Yes |
| `post_new` | After the artifact is written | No — logged as a warning |
| `pre_remove` | Before `git worktree remove` | Yes |
| `post_remove` | After `git branch -d` | No |
| `pre_sync` | Before each worktree's file sync | Yes |
| `post_sync` | After each worktree's file sync | No |
| `post_config_init` | Once, after `tp config init` writes the file | No |

Sync events fire once per worktree. `post_config_init` reads its hook list from the file it
just wrote, so it only does anything with `--inherit`, and never fires for `--global`.

Template variables: `{{.Branch}}` (raw, e.g. `feature/auth`), `{{.WorktreePath}}`,
`{{.Slug}}`, `{{.HookType}}`, `{{.OutputDir}}`.

```toml
[[hooks.post_new]]
command = "cd {{.WorktreePath}} && pnpm install --frozen-lockfile"
only = ["feat/*", "fix/*"]

[[hooks.pre_remove]]
command = "git -C {{.WorktreePath}} diff --exit-code HEAD"
except = ["throwaway/*"]

[[hooks.post_config_init]]
command = "npx skills add my-org/my-skills"
interactive = true
```

Three behaviours that decide whether a hook works:

- **Output is discarded by default.** stdin is `/dev/null`, stdout and stderr are captured
  and dropped. Anything that prompts sees a non-TTY and silently degrades. Set
  `interactive = true` to hand it the real terminal — and only then, since `post_sync` fires
  per worktree and will interleave with `tp`'s own output.
- **`only`/`except` are globs on the raw branch name.** `*` stays within a path segment,
  `**` crosses separators. Both set means both must pass. Neither set means always run.
- **Working directory is `tp`'s, not the worktree's.** Use `{{.WorktreePath}}` explicitly.
  A non-zero exit stops the remaining entries in that list; a failed pre-hook aborts the
  whole operation.

Hooks are unsupported on Windows — `tp` errors if one is configured there.

## Editor recipes

**JetBrains / Neovim / Helix** — these want a directory, not a workspace file. Point
`[open]` at the editor and keep `[artifact]` minimal:

```toml
[open]
command = ["idea", "{{.ArtifactPath}}"]
```

`docs/configuration.md` in the treepad repo carries full worked examples for JetBrains, Zed,
Neovim, Helix, and Sublime if a user needs one verbatim.
