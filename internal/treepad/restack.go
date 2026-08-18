package treepad

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

// restack repairs each materialised member's worktree in place after a
// possible upstream rewrite (ADR 0003, corrected by issue #140): merging a
// lower pull request rewrites every branch above it server-side, so a
// worktree above the merge diverges from origin/<branch> rather than merely
// falling behind, and plain `merge --ff-only` cannot repair that. It fetches
// only origin/<branch> — `gh stack rebase`/`sync` are forbidden against a
// fleet (ADR 0003) — and applies batch.RestackDecision's verdict, marking
// anything it cannot safely repair StackStale for a human. Treepad never
// stashes on an agent's behalf.
//
// A member with no worktree on disk yet (blocked, gh-required, or a
// --dry-run tick) is skipped: there is nothing to fetch into.
func restack(ctx context.Context, d deps.Deps, in ReconcileInput, report Report) {
	if in.DryRun {
		return
	}
	for i := range report.Members {
		e := &report.Members[i]
		if e.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(e.WorktreePath); err != nil {
			continue
		}
		restackOne(ctx, d, e)
	}
}

func restackOne(ctx context.Context, d deps.Deps, e *ReportEntry) {
	if err := fetchBranch(ctx, d.Runner, e.WorktreePath, e.Branch); err != nil {
		d.Log.Warn("restack %s: %s", e.Branch, err)
		return
	}
	dirty, err := worktree.Dirty(ctx, d.Runner, e.WorktreePath)
	if err != nil {
		d.Log.Warn("restack %s: %s", e.Branch, err)
		return
	}
	ahead, behind, hasUpstream, err := worktree.AheadBehind(ctx, d.Runner, e.WorktreePath)
	if err != nil {
		d.Log.Warn("restack %s: %s", e.Branch, err)
		return
	}
	if !hasUpstream {
		return
	}

	var patchEquivalent bool
	if ahead > 0 && behind > 0 {
		patchEquivalent, err = patchEquivalentToOrigin(ctx, d.Runner, e.WorktreePath, e.Branch)
		if err != nil {
			d.Log.Warn("restack %s: %s", e.Branch, err)
			return
		}
	}

	switch batch.RestackDecision(!dirty, ahead, behind, patchEquivalent) {
	case batch.RestackFastForward:
		if err := mergeFastForward(ctx, d.Runner, e.WorktreePath, e.Branch); err != nil {
			d.Log.Warn("restack %s: %s", e.Branch, err)
		}
	case batch.RestackReset:
		if err := resetHardToOrigin(ctx, d.Runner, e.WorktreePath, e.Branch); err != nil {
			d.Log.Warn("restack %s: %s", e.Branch, err)
		}
	case batch.RestackStale:
		e.StackStale = true
	case batch.RestackNone:
		// nothing to do: not behind origin/<branch>
	}
}

func fetchBranch(ctx context.Context, r worktree.CommandRunner, path, branch string) error {
	if _, err := r.Run(ctx, "git", "-C", path, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("git fetch origin %s: %w", branch, err)
	}
	return nil
}

func mergeFastForward(ctx context.Context, r worktree.CommandRunner, path, branch string) error {
	if _, err := r.Run(ctx, "git", "-C", path, "merge", "--ff-only", "origin/"+branch); err != nil {
		return fmt.Errorf("git merge --ff-only origin/%s: %w", branch, err)
	}
	return nil
}

func resetHardToOrigin(ctx context.Context, r worktree.CommandRunner, path, branch string) error {
	if _, err := r.Run(ctx, "git", "-C", path, "reset", "--hard", "origin/"+branch); err != nil {
		return fmt.Errorf("git reset --hard origin/%s: %w", branch, err)
	}
	return nil
}

// patchEquivalentToOrigin reports whether every commit unique to branch is
// already present upstream in rewritten form. `git cherry` compares by patch
// id: a `+`-prefixed line names a commit whose patch is genuinely absent
// upstream, so any such line means a reset would discard real work.
func patchEquivalentToOrigin(ctx context.Context, r worktree.CommandRunner, path, branch string) (bool, error) {
	out, err := r.Run(ctx, "git", "-C", path, "cherry", "origin/"+branch, branch)
	if err != nil {
		return false, fmt.Errorf("git cherry origin/%s %s: %w", branch, branch, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "+") {
			return false, nil
		}
	}
	return true, nil
}
