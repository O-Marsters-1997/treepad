package treepad

import (
	"testing"

	"treepad/internal/worktree"
)

func TestResolveSourceDir(t *testing.T) {
	withMain := []worktree.Worktree{
		{Path: "/repo/feat", Branch: "feat", IsMain: false},
		{Path: "/repo/main", Branch: "main", IsMain: true},
	}

	tests := []struct {
		name       string
		useCurrent bool
		sourcePath string
		cwd        string
		worktrees  []worktree.Worktree
		want       string
		wantErr    bool
	}{
		{
			name:       "use current flag",
			useCurrent: true,
			cwd:        "/home/user/repo",
			want:       "/home/user/repo",
		},
		{
			// Absolute path: filepath.Abs is a no-op, keeping the test hermetic.
			name:       "explicit path",
			sourcePath: "/tmp/other-repo",
			want:       "/tmp/other-repo",
		},
		{
			name:      "defaults to main worktree",
			worktrees: withMain,
			want:      "/repo/main",
		},
		{
			name:      "error when no main worktree",
			worktrees: []worktree.Worktree{{Path: "/repo/feat", Branch: "feat", IsMain: false}},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveSourceDir(tt.useCurrent, tt.sourcePath, tt.cwd, tt.worktrees)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
