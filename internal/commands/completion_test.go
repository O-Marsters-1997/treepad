package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"treepad/internal/worktree"
	"treepad/internal/worktree/worktreetest"
)

func TestWriteBranches(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	featPath := t.TempDir()

	porcelain := fmt.Sprintf(
		"worktree %s\nbranch refs/heads/main\n\nworktree %s\nbranch refs/heads/feat\n\n",
		mainPath, featPath,
	)
	detachedPorcelain := fmt.Sprintf(
		"worktree %s\ndetached\n\n",
		featPath,
	)

	tests := []struct {
		name      string
		porcelain string
		include   func(worktree.Worktree) bool
		want      string
	}{
		{
			name:      "all non-detached branches",
			porcelain: porcelain,
			include:   func(wt worktree.Worktree) bool { return wt.Branch != "(detached)" },
			want:      "main\nfeat\n",
		},
		{
			name:      "skip main worktree",
			porcelain: porcelain,
			include:   func(wt worktree.Worktree) bool { return !wt.IsMain && wt.Branch != "(detached)" },
			want:      "feat\n",
		},
		{
			name:      "detached worktree excluded by predicate",
			porcelain: detachedPorcelain,
			include:   func(wt worktree.Worktree) bool { return wt.Branch != "(detached)" },
			want:      "",
		},
		{
			name:      "runner error produces no output",
			porcelain: "",
			include:   func(worktree.Worktree) bool { return true },
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runner worktree.CommandRunner
			if tt.porcelain == "" {
				runner = worktreetest.StaticRunner{Err: fmt.Errorf("git unavailable")}
			} else {
				runner = worktreetest.StaticRunner{Output: []byte(tt.porcelain)}
			}

			var buf bytes.Buffer
			writeBranches(context.Background(), runner, &buf, tt.include)

			if got := buf.String(); got != tt.want {
				t.Errorf("got %q; want %q", got, tt.want)
			}
		})
	}
}
