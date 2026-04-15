package treepad

import (
	"context"
	"errors"
	"strings"
	"testing"

	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

func TestGenerate(t *testing.T) {
	t.Run("syncs non-source worktrees", func(t *testing.T) {
		syn := &fakeSyncer{}
		deps := testDeps(fakeRunner{output: twoWorktreePorcelain}, syn, nil)

		err := Generate(context.Background(), deps, GenerateInput{SourcePath: "/repo/main", SyncOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.calls))
		}
		if syn.calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.calls[0].TargetDir)
		}
	})

	errorTests := []struct {
		name    string
		runner  worktree.CommandRunner
		syncer  internalsync.Syncer
		input   GenerateInput
		wantErr string
	}{
		{
			name:    "propagates syncer error",
			runner:  fakeRunner{output: twoWorktreePorcelain},
			syncer:  &fakeSyncer{err: errors.New("sync failed")},
			input:   GenerateInput{SourcePath: "/repo/main", SyncOnly: true},
			wantErr: "sync failed",
		},
		{
			name:    "no worktrees",
			runner:  fakeRunner{output: []byte{}},
			syncer:  &fakeSyncer{},
			input:   GenerateInput{SourcePath: "/repo/main"},
			wantErr: "no git worktrees found",
		},
		{
			name:    "runner error",
			runner:  fakeRunner{err: errors.New("git not found")},
			syncer:  &fakeSyncer{},
			input:   GenerateInput{},
			wantErr: "git not found",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(tt.runner, tt.syncer, nil)
			err := Generate(context.Background(), deps, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
