package cd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
)

func TestBase(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"

	porcelain := treepadtest.TwoWorktreePorcelainWithMain(mainPath, featPath)

	tests := []struct {
		name        string
		cwd         string
		porcelain   []byte
		wantCD      string
		wantErr     bool
		wantErrText string
	}{
		{
			name:      "emits cd to main when cwd is a linked worktree",
			cwd:       featPath,
			porcelain: porcelain,
			wantCD:    "__TREEPAD_CD__\t" + mainPath + "\n→ cd: " + mainPath + "\n",
		},
		{
			name:        "errors when already on the default worktree",
			cwd:         mainPath,
			porcelain:   porcelain,
			wantErr:     true,
			wantErrText: "already on the default worktree",
		},
		{
			// twoWorktreePorcelain paths don't exist on disk so neither is main
			name:        "errors when no main worktree can be found",
			cwd:         featPath,
			porcelain:   treepadtest.TwoWorktreePorcelain,
			wantErr:     true,
			wantErrText: "could not find main worktree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			deps := deps.Deps{
				Runner: treepadtest.StaticRunner{Output: tt.porcelain},
				Syncer: &treepadtest.FakeSyncer{},
				Out:    &out,
				In:     strings.NewReader(""),
			}

			err := Base(context.Background(), deps, BaseInput{Cwd: tt.cwd})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrText)
				}
				if out.Len() > 0 {
					t.Errorf("no output expected on error, got: %q", out.String())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := out.String(); got != tt.wantCD {
				t.Errorf("output = %q, want %q", got, tt.wantCD)
			}
		})
	}
}
