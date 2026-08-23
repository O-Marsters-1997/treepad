package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/hook"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

func TestCreateWorktreeWithSyncPostHookFailure(t *testing.T) {
	mainPath := makeMainWorktree(t)
	outputDir := t.TempDir()
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)

	tests := []struct {
		name  string
		toml  string
		event hook.Event
	}{
		{
			name:  "post_new",
			toml:  "[[hooks.post_new]]\ncommand = \"fail\"\n",
			event: hook.PostNew,
		},
		{
			name:  "post_sync",
			toml:  "[[hooks.post_sync]]\ncommand = \"fail\"\n",
			event: hook.PostSync,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeTOML(t, mainPath, tt.toml)

			runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{Output: nil}, // git worktree add --no-checkout
				{Output: nil}, // git checkout
			}}
			var warnings strings.Builder
			d := deps.Deps{
				Runner:     runner,
				Syncer:     &treepadtest.FakeSyncer{},
				Opener:     &treepadtest.FakeOpener{},
				HookRunner: &treepadtest.FakeHookRunner{Err: errors.New("hook exploded")},
				Log:        treepadtest.NewPrinter(&warnings),
			}

			res, err := CreateWorktreeWithSync(context.Background(), d, "feature/auth", "main", outputDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.PostErr == nil {
				t.Fatal("PostErr is nil, want the post-hook failure")
			}
			if res.PostErr.Event != tt.event {
				t.Errorf("PostErr.Event = %q, want %q", res.PostErr.Event, tt.event)
			}
			if !strings.Contains(res.PostErr.Error(), "hook exploded") {
				t.Errorf("PostErr = %q, want it to name the hook failure", res.PostErr)
			}
			if got := strings.Count(warnings.String(), "[WARN]"); got != 1 {
				t.Errorf("logged %d [WARN] lines, want 1:\n%s", got, warnings.String())
			}
		})
	}
}
