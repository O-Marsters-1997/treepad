package treepad

import (
	"context"
	"errors"
	"strings"
	"testing"

	internalsync "treepad/internal/sync"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
	"treepad/internal/worktree"
)

func TestGenerate(t *testing.T) {
	t.Run("syncs non-source worktrees", func(t *testing.T) {
		syn := &treepadtest.FakeSyncer{}
		deps := deps.Deps{Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.TwoWorktreePorcelain}}}, Syncer: syn}

		err := Generate(context.Background(), deps, GenerateInput{SourcePath: "/repo/main", SyncOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.Calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.Calls))
		}
		if syn.Calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.Calls[0].TargetDir)
		}
	})

	t.Run("Branch filters to one target", func(t *testing.T) {
		syn := &treepadtest.FakeSyncer{}
		deps := deps.Deps{Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.ThreeWorktreePorcelain}}}, Syncer: syn}

		err := Generate(context.Background(), deps, GenerateInput{
			SourcePath: "/repo/main",
			SyncOnly:   true,
			Branch:     "feat",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.Calls) != 1 {
			t.Fatalf("syncer called %d times, want 1 for branch filter", len(syn.Calls))
		}
		if syn.Calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.Calls[0].TargetDir)
		}
	})

	t.Run("unknown Branch returns clear error", func(t *testing.T) {
		syn := &treepadtest.FakeSyncer{}
		deps := deps.Deps{Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.ThreeWorktreePorcelain}}}, Syncer: syn}

		err := Generate(context.Background(), deps, GenerateInput{
			SourcePath: "/repo/main",
			SyncOnly:   true,
			Branch:     "nonexistent",
		})
		if err == nil || !strings.Contains(err.Error(), "no worktree found") {
			t.Errorf("got error %v, want containing \"no worktree found\"", err)
		}
	})

	t.Run("empty Branch syncs all targets", func(t *testing.T) {
		syn := &treepadtest.FakeSyncer{}
		deps := deps.Deps{Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.ThreeWorktreePorcelain}}}, Syncer: syn}

		err := Generate(context.Background(), deps, GenerateInput{
			SourcePath: "/repo/main",
			SyncOnly:   true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.Calls) != 2 {
			t.Errorf("syncer called %d times, want 2 for fleet sync", len(syn.Calls))
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
			runner:  &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.TwoWorktreePorcelain}}},
			syncer:  &treepadtest.FakeSyncer{Err: errors.New("sync failed")},
			input:   GenerateInput{SourcePath: "/repo/main", SyncOnly: true},
			wantErr: "sync failed",
		},
		{
			name:    "no worktrees",
			runner:  &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: []byte{}}}},
			syncer:  &treepadtest.FakeSyncer{},
			input:   GenerateInput{SourcePath: "/repo/main"},
			wantErr: "no git worktrees found",
		},
		{
			name:    "runner error",
			runner:  &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Err: errors.New("git not found")}}},
			syncer:  &treepadtest.FakeSyncer{},
			input:   GenerateInput{},
			wantErr: "git not found",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := deps.Deps{Runner: tt.runner, Syncer: tt.syncer}
			err := Generate(context.Background(), deps, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
