# 5. The library facade grows to List

Date: 2026-11-02

## Status

Accepted. Supersedes the sentence "The surface is `New` and `Remove` and nothing else" in
[ADR 0004](./0004-treepad-exposes-a-library-facade.md); the rest of ADR 0004 stands.

## Context

ADR 0004 kept the facade to `New` and `Remove` and left enumeration to
`git worktree list --porcelain`, on the grounds that git's porcelain format is a more stable
contract than a `v0` API.

Command-centre enumerates every worktree on every tick to reconcile its slots against disk. With
`New` and `Remove` in the library, that parse is the last reason it needs `git worktree` or `tp`
subprocesses (`docs/prds/cc-worktrees-via-library.md`). It also re-implements what treepad already
knows: which worktree is the main one, and the branch of a worktree that is mid-rebase, where the
porcelain output shows a detached HEAD.

## Decision

The facade gains `List`:

```go
type ListOptions struct{ RepoDir string }
func List(ctx context.Context, o ListOptions) ([]Worktree, error)
```

It returns the existing `Worktree` type, extended with a `Main` field. `Branch` resolves through a
mid-rebase. `BaseSHA` is empty in `List` results, since computing it needs a base ref that
enumeration does not have.

`List` runs through the same `ExecRunner{Dir: RepoDir}` as `New` and `Remove`, so it names the repo
it means. `Activity`, `status`, `doctor`, `sync`, `prune` and `batch` stay out: liveness remains
with the caller rather than becoming a second reading of the Activity file.

## Consequences

Command-centre drops its last `git worktree` subprocess. The golden `go doc -all` snapshot grows by
`List`, `ListOptions` and `Worktree.Main`, which is the reviewable signal ADR 0004 intended.

`Worktree.BaseSHA` is empty for `List` results but populated for `New`, so callers must not treat
the field as always set. This is the cost of reusing one type rather than adding a second.

## Alternatives considered

**Keep parsing `git worktree list --porcelain` in the caller.** Rejected: every consumer
re-implements main-worktree detection and mid-rebase branch resolution, and command-centre is
pursuing zero subprocesses.

**A separate `ListEntry` type without `BaseSHA`.** Rejected: a second public type to avoid one
field that is documented empty.
