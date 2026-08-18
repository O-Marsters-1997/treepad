package batch

import "testing"

func TestRestackDecision(t *testing.T) {
	tests := []struct {
		name            string
		clean           bool
		ahead, behind   int
		patchEquivalent bool
		want            RestackAction
	}{
		// in-sync: nothing to restack, clean or dirty, patch-equivalent or not.
		{"clean in-sync", true, 0, 0, false, RestackNone},
		{"clean in-sync, patch-equivalent flag ignored", true, 0, 0, true, RestackNone},
		{"dirty in-sync", false, 0, 0, false, RestackNone},
		{"dirty in-sync, patch-equivalent flag ignored", false, 0, 0, true, RestackNone},

		// ahead-only: unpushed local commits, origin has nothing new. Never a
		// restack concern, clean or dirty.
		{"clean ahead-only", true, 3, 0, false, RestackNone},
		{"dirty ahead-only", false, 2, 0, false, RestackNone},

		// behind-only: plain fast-forward candidate, but only when clean.
		{"clean behind-only fast-forwards", true, 0, 4, false, RestackFastForward},
		{"dirty behind-only never auto-repairs", false, 0, 4, false, RestackStale},

		// diverged, patch-equivalent (the rewritten-upstream case): reset only
		// when clean; dirty never auto-repairs even here.
		{"clean diverged patch-equivalent resets", true, 2, 3, true, RestackReset},
		{"dirty diverged patch-equivalent never auto-repairs", false, 2, 3, true, RestackStale},

		// diverged, genuinely local commit: never auto-repairs, clean or dirty.
		{"clean diverged non-patch-equivalent never auto-repairs", true, 1, 1, false, RestackStale},
		{"dirty diverged non-patch-equivalent never auto-repairs", false, 1, 1, false, RestackStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestackDecision(tt.clean, tt.ahead, tt.behind, tt.patchEquivalent)
			if got != tt.want {
				t.Errorf("RestackDecision(clean=%v, ahead=%d, behind=%d, patchEquivalent=%v) = %v, want %v",
					tt.clean, tt.ahead, tt.behind, tt.patchEquivalent, got, tt.want)
			}
		})
	}
}

// TestRestackDecisionDirtyNeverAutoRepairs is the load-bearing assertion from
// issue #140: across every ahead/behind/patch-equivalent combination, a dirty
// worktree must never come back as an action that mutates it.
func TestRestackDecisionDirtyNeverAutoRepairs(t *testing.T) {
	for _, ahead := range []int{0, 1, 5} {
		for _, behind := range []int{0, 1, 5} {
			for _, patchEquivalent := range []bool{false, true} {
				got := RestackDecision(false, ahead, behind, patchEquivalent)
				if got == RestackFastForward || got == RestackReset {
					t.Errorf("dirty worktree auto-repaired: RestackDecision(false, %d, %d, %v) = %v",
						ahead, behind, patchEquivalent, got)
				}
			}
		}
	}
}

// TestRestackDecisionNonPatchEquivalentNeverAutoRepairs is the second
// load-bearing assertion: a diverged branch holding a commit git cherry says
// is genuinely not upstream must never auto-repair, clean or dirty.
func TestRestackDecisionNonPatchEquivalentNeverAutoRepairs(t *testing.T) {
	for _, clean := range []bool{true, false} {
		for _, ahead := range []int{1, 5} {
			for _, behind := range []int{1, 5} {
				got := RestackDecision(clean, ahead, behind, false)
				if got != RestackStale {
					t.Errorf("diverged non-patch-equivalent auto-repaired: RestackDecision(%v, %d, %d, false) = %v, want RestackStale",
						clean, ahead, behind, got)
				}
			}
		}
	}
}
