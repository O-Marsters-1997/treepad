package batch

import "testing"

func TestLinkArgs(t *testing.T) {
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
		name  string
		chain []Member
		prs   map[string]PR
		want  []string
	}{
		{
			name:  "excludes a Chain member with no pull request: prefix stops there, not a filter around it",
			chain: chain(3),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				// feat/pos1 has no PR at all
				"feat/pos2": {State: "OPEN"},
			},
			want: nil, // prefix is [pos0] only, len < 2, so nil
		},
		{
			name:  "prefix stops at the first closed PR even though a later one is open",
			chain: chain(3),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				"feat/pos1": {State: "CLOSED"},
				"feat/pos2": {State: "OPEN"},
			},
			want: nil, // prefix is [pos0] only
		},
		{
			name:  "full prefix links when every branch has an open pull request",
			chain: chain(3),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				"feat/pos1": {State: "OPEN"},
				"feat/pos2": {State: "OPEN"},
			},
			want: []string{"feat/pos0", "feat/pos1", "feat/pos2"},
		},
		{
			name:  "partial prefix links only the leading open-PR members, bottom to top",
			chain: chain(3),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				"feat/pos1": {State: "OPEN"},
			},
			want: []string{"feat/pos0", "feat/pos1"},
		},
		{
			name:  "single-member Chain never links, even with an open pull request",
			chain: chain(1),
			prs:   map[string]PR{"feat/pos0": {State: "OPEN"}},
			want:  nil,
		},
		{
			name:  "head with no pull request links nothing",
			chain: chain(2),
			prs:   map[string]PR{},
			want:  nil,
		},
		{
			name:  "a merged PR does not extend the link prefix: only an open PR counts",
			chain: chain(2),
			prs: map[string]PR{
				"feat/pos0": {State: "OPEN"},
				"feat/pos1": {State: "MERGED"},
			},
			want: nil, // prefix is [pos0] only, len < 2, so nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinkArgs(tt.chain, tt.prs)
			if len(got) != len(tt.want) {
				t.Fatalf("LinkArgs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("LinkArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
