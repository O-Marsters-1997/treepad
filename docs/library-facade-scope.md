# Library facade — scope

**Status:** scoped, not built.
**Goal:** treepad supports two first-class callers — the `tp` CLI and Go code importing it as
a library. The first library consumer is command-centre, which today is specced to reach
treepad by subprocess (`docs/command-centre-v1.md` decision 5).

That decision recorded a *capability* constraint — no importable surface existed — not a
preference for subprocess. This document scopes the surface so the constraint can be lifted.

The reasoning behind the shape below is recorded in
[ADR 0004](./adr/0004-treepad-exposes-a-library-facade.md); this document is the work item.

## What already holds

treepad is already library-shaped; nothing needs restructuring.

- `lifecycle.New(ctx, deps.Deps, NewInput) (string, error)` — `internal/treepad/lifecycle/new.go:23`
- `lifecycle.Remove(ctx, deps.Deps, RemoveInput) error` — `internal/treepad/lifecycle/remove.go:23`
- `lifecycle.CreateWorktreeWithSync(...) (CreateResult, error)` — `lifecycle.go:40`, the shared core
- `deps.DefaultDeps(out, errw, in) Deps` — `internal/treepad/deps/deps.go:42`, the composition root

`internal/commands/new.go` is 84 lines of `urfave/cli` flag wiring; `remove.go` is 26. There is
no business logic in the CLI layer to extract.

Go's `internal/` rule is import *visibility*, not linking. A package at the module root may
import all 17 internal packages in lifecycle's closure with zero moves. Only the facade's own
exported identifiers become public API.

`go list -deps ./internal/treepad/lifecycle` pulls `BurntSushi/toml`, `doublestar/v4`,
`x/sync/errgroup`, `x/sys`, `x/term`. No bubbletea, no urfave/cli — the TUI is off this path,
so importing treepad does not drag a terminal UI into a consumer's binary.

## Settled decisions

| # | Decision | Why |
|---|---|---|
| 1 | **Thin facade; `deps.Deps` stays internal.** Public surface is 2 funcs, 2 option structs, 1 result struct, 3 sentinels. | `Deps` names 8 interfaces across 8 packages plus concrete `*ui.Printer`. Exporting it freezes all of that. An injection seam is additive later (see *Deferred*). |
| 2 | **Facade is a sibling of the CLI, not its replacement.** Both adapt over `CreateWorktreeWithSync`. | The CLI needs `--open`, `--current`, `--ticket` and the `__TREEPAD_CD__` fd-3 emit. The library must not carry terminal affordances. Drift risk is near-zero: `lifecycle.New` is 47 lines and the real work is all in the shared core. |
| 3 | **`go.work` for local development, v-tags for CI.** | The workspace file lives at `ai-development/`, outside both git repos, so it is uncommitted by construction. Edits to treepad are live in command-centre with no tag; a fresh clone and CI resolve a real version. |
| 4 | **`New` + `Remove` only.** No `List`, no `Activity`. | The app keeps parsing `git worktree list --porcelain` for reconcile (command-centre decision 20). Git's porcelain format is a more stable contract than a v0 API. Exposing `Activity` would deepen the double-ownership already flagged at `command-centre/docs/assessment.md:44`. |
| 5 | **Two sentinels, wrap the rest.** | command-centre buckets every cut failure as `cut failed → re-run` (v1 §383), so `New` needs no taxonomy. Remove needs `ErrNotFound` (idempotent reconcile) and `ErrDirty`. Adding sentinels later is backward compatible; removing them is not. |
| 6 | **Identical cut semantics — sync and hooks both run — except interactive hooks are refused.** | An app-cut worktree must be indistinguishable from a hand-cut one. The one divergence closes a real hazard: `passthrough.OSRunner.Run` opens `/dev/tty` when stdio is not a terminal, so an `interactive = true` hook fired by a background tick would seize the user's terminal and block. Non-interactive hooks are safe (`hook/runner.go:53`). |
| 7 | **Golden API snapshot guards the boundary.** | `batch/api_test.go` bans internal imports outright, which the facade cannot satisfy by design. Go also leaks silently: an exported func returning an `internal/...` type compiles, and consumers can hold it with `:=` but never name it. A `go doc -all` golden file catches leakage *and* unplanned surface growth. |
| 8 | **`Force` covers the branch only; dirty is always refused.** | Makes command-centre decision 3 structural rather than a rule the app must remember. Also yields a reliable `ErrDirty` with no git-stderr string matching. **This diverges from `tp remove --force`, which does wipe dirty trees.** |

## Public API

```go
// Package treepad, at the module root:
//   import "github.com/O-Marsters-1997/treepad"
package treepad

type NewOptions struct {
    Branch  string
    Base    string
    RepoDir string    // required; the app manages several repos from one process
    Stderr  io.Writer // optional narrative; nil discards
}

type RemoveOptions struct {
    Branch  string
    RepoDir string
    Force   bool // git branch -D instead of -d; never wipes a dirty tree
}

type Worktree struct {
    Path    string
    Branch  string
    BaseSHA string
}

func New(ctx context.Context, o NewOptions) (Worktree, error)
func Remove(ctx context.Context, o RemoveOptions) error

var (
    ErrNotFound        = errors.New("worktree not found")
    ErrDirty           = errors.New("worktree has uncommitted changes")
    ErrInteractiveHook = errors.New("interactive hooks are not supported by the library API")
)
```

## Changes required in treepad

**1. `ExecRunner` gains a working directory. — 2 lines, `internal/worktree/worktree.go:30-35`**

The blocking defect. `CommandRunner.Run(ctx, name, args...)` (`worktree.go:18`) has no
directory parameter and `ExecRunner` never sets `cmd.Dir`, so every git command inherits the
*process* cwd. Fine for a CLI standing in the repo; unusable for the app, which manages
`services` and `support-app` from one long-lived process with a ticker and `max_agents = 4`.
`os.Chdir` is process-global and cannot be used under concurrency.

```go
type ExecRunner struct{ Dir string }   // +1 field
// in Run:  cmd.Dir = r.Dir            // +1 line; "" means inherit
```

`""` reproduces today's behaviour exactly, so the CLI is untouched. `DefaultDeps` already
threads one runner into `hook.ExecRunner` and `artifact.ExecOpener`, so this single field fixes
git, non-interactive hooks and the `open` command together.

Rejected: changing `CommandRunner.Run` to take a dir — 34 call sites across 7 non-test files,
5 identical interface declarations, 4+ test fakes, and one of those declarations is
`batch.Runner` (`batch/manifest.go:41`), which is published API. Rejected: a `git -C`-injecting
wrapper in the facade — fixes git only, leaves hooks and `open` running in the app's cwd.

**2. New root package `treepad`. — ~120 LOC**

- Builds `deps.DefaultDeps(io.Discard, o.Stderr, nil)`, then overrides
  `Runner: worktree.ExecRunner{Dir: o.RepoDir}` and a `TTYRunner` that returns
  `ErrInteractiveHook` instead of opening `/dev/tty`.
- `New` calls `CreateWorktreeWithSync` directly, **not** `lifecycle.New` — the latter emits the
  cd directive and returns `RC.Main.Path`, the main worktree, discarding the created path it
  already computed. `CreateResult.WorktreePath` (`lifecycle.go:32-37`) is the value the facade
  needs and it is already there.
- `BaseSHA` comes from one `git rev-parse <base>^{commit}` in the facade, so the CLI path gains
  no extra git call.
- `Remove` pre-checks `worktree.Dirty()` (`worktree.go:157`) and returns `ErrDirty` before
  touching anything, regardless of `Force`.
- Maps `worktree.ErrNotFound` — a *struct* (`worktree.go:227`), not a sentinel var — onto the
  exported `ErrNotFound`.

**3. Golden API test. — ~25 LOC + `testdata/api.golden`**

Captures `go doc -all` for the root package; any diff fails until the golden file is updated in
a reviewable commit.

**4. Facade e2e test. — ~120 LOC**

A plain Go test, not `testscript` — the existing `e2e/*.txtar` harness drives the `tp` binary and
cannot exercise a Go API. Against a real temp git repo with a real `.treepad.toml`:

- `New` returns a `Worktree` whose `Path` exists and whose `Branch` / `BaseSHA` are correct
- `[sync] include` files landed in the new worktree
- a hook with `interactive = true` returns `ErrInteractiveHook` and does not open `/dev/tty`
- `Remove` on a dirty tree returns `ErrDirty` even with `Force: true`
- `Remove` on an absent branch returns `ErrNotFound`

This validates the **headless** path — no TTY, real sync, real hooks — which the CLI's own test
suite never covers.

**5. Cut v-prefixed tags.**

Every tag today is `0.4.9`-style; there are **zero** `v`-prefixed tags. Go modules require
`vX.Y.Z`, so `go get github.com/O-Marsters-1997/treepad` currently resolves to a commit
pseudo-version rather than a release. Start at `v0.5.0`. Correct hygiene for a public Go module
regardless of this work.

## Changes required in command-centre

1. `go.work` at `ai-development/` listing `./treepad` and `./command-center`.
2. `go.mod`: `require github.com/O-Marsters-1997/treepad v0.5.0`. Currently zero requires.
   `go 1.26.6` against treepad's `go 1.26.1` — consumer newer, no toolchain issue.
3. `internal/tp`: two function bodies. `plans/command-centre-phase-1.md:127` already specs it as
   `New(branch, baseRef)`, `Remove(branch, force)`, "thin". **No app call site moves** — only the
   bodies change, from `exec.Command` to a function call.
4. Delete `e2e/faketp` (160 LOC) and its test. Its header describes it as implementing "the two
   commands the app uses — new and remove — by delegating to real git". Replaced by a real
   `.treepad.toml` fixture, which is more faithful but slower and not stubbable.
5. Amend decision 5 in `docs/command-centre-v1.md` and the ~16 subprocess references in
   `plans/command-centre-phase-1.md`.
6. Decision 3 (never delete a dirty worktree) becomes enforced by treepad rather than only by
   app discipline — but the *unpushed commits* half of it remains entirely the app's, since
   treepad checks dirtiness only.

## Deliberately out of scope

- `List`, `Activity`, `status`, `doctor`, `sync`, `batch` — decision 4.
- `deps` and the 8 dependency packages stay under `internal/` — decision 1.
- An injection seam for tests. A `Git Git` field on the option structs (nil → real git) is a
  backward-compatible addition whenever it is wanted; it does not need deciding now.
- Interactive hooks in the library path — decision 6.

## Why the surface can be narrow without regret

At `v0.x` the module protocol carries no compatibility promise, and almost every deferred choice
here is additive: new sentinels, new option-struct fields, new result-struct fields, new
functions. The only non-additive changes are renaming or removing what has shipped. So the
discipline required is just: do not ship a field you are unsure about.
