package batch

// RestackAction is what Reconcile's restack step does to one member's
// worktree after `git fetch origin <branch>`.
type RestackAction int

const (
	RestackNone        RestackAction = iota // nothing to do: behind == 0
	RestackFastForward                      // clean, plainly behind: git merge --ff-only origin/<branch>
	RestackReset                            // clean, diverged, patch-equivalent: git reset --hard origin/<branch>
	RestackStale                            // dirty, or diverged with a genuinely local commit: wait for a human
)

// RestackDecision applies the restack safety predicate (issue #140, correcting
// ADR 0003's "fast-forward only" mechanism: a server-side rebase leaves the
// branch diverged from origin, not merely behind, and `merge --ff-only`
// refuses a diverged branch).
//
// behind == 0 means origin has nothing this branch lacks — whether the
// branch is ahead with unpushed commits or fully in sync, there is nothing
// to restack, regardless of clean. Only when behind > 0 does dirty become
// disqualifying: a dirty worktree, or a diverged one holding a commit
// git cherry says is not yet upstream, never auto-repairs — treepad never
// stashes or discards work on an agent's behalf.
func RestackDecision(clean bool, ahead, behind int, patchEquivalent bool) RestackAction {
	if behind == 0 {
		return RestackNone
	}
	if !clean {
		return RestackStale
	}
	if ahead == 0 {
		return RestackFastForward
	}
	if patchEquivalent {
		return RestackReset
	}
	return RestackStale
}
