package batch

import "testing"

func TestReadyToMaterialise(t *testing.T) {
	chain := func(n int) []Member {
		members := make([]Member, n)
		base := "main"
		for i := 0; i < n; i++ {
			branch := "feat/pos" + string(rune('0'+i))
			members[i] = Member{Branch: branch, Base: base, Position: i}
			base = branch
		}
		return members
	}

	tests := []struct {
		name     string
		chain    []Member
		existing map[string]bool
		prs      map[string]PR
		want     []string // branches expected in the ready result, in order
	}{
		{
			name:  "position 0 is always ready even with no PR data",
			chain: chain(1),
			want:  []string{"feat/pos0"},
		},
		{
			name:  "parent's PR open: position 1 is ready",
			chain: chain(2),
			prs:   map[string]PR{"feat/pos0": {State: "OPEN"}},
			want:  []string{"feat/pos0", "feat/pos1"},
		},
		{
			name:  "parent has no PR: position 1 is not ready",
			chain: chain(2),
			prs:   map[string]PR{},
			want:  []string{"feat/pos0"},
		},
		{
			name:  "parent's PR closed: position 1 is not ready",
			chain: chain(2),
			prs:   map[string]PR{"feat/pos0": {State: "CLOSED"}},
			want:  []string{"feat/pos0"},
		},
		{
			name:  "parent's PR merged: position 1 is ready",
			chain: chain(2),
			prs:   map[string]PR{"feat/pos0": {State: "MERGED"}},
			want:  []string{"feat/pos0", "feat/pos1"},
		},
		{
			// pos1's own PR is open, but pos0's is not, so pos1 never becomes ready.
			name:  "readiness does not skip past a blocked member: position 2 not ready when position 1 is blocked",
			chain: chain(3),
			prs:   map[string]PR{"feat/pos1": {State: "OPEN"}},
			want:  []string{"feat/pos0"},
		},
		{
			name:  "full chain ready when every parent has an open PR",
			chain: chain(3),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				"feat/pos1": {State: "OPEN"},
			},
			want: []string{"feat/pos0", "feat/pos1", "feat/pos2"},
		},
		{
			name:     "an already-materialised member stays ready even if its parent's PR later closes",
			chain:    chain(2),
			existing: map[string]bool{"feat/pos1": true},
			prs:      map[string]PR{"feat/pos0": {State: "CLOSED"}},
			want:     []string{"feat/pos0", "feat/pos1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadyToMaterialise(tt.chain, tt.existing, tt.prs)
			if len(got) != len(tt.want) {
				t.Fatalf("ReadyToMaterialise() = %v, want %v", branchesOf(got), tt.want)
			}
			for i, m := range got {
				if m.Branch != tt.want[i] {
					t.Errorf("ReadyToMaterialise()[%d] = %q, want %q", i, m.Branch, tt.want[i])
				}
			}
		})
	}
}

func branchesOf(members []Member) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = m.Branch
	}
	return out
}
