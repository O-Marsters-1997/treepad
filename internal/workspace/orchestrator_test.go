package workspace

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

// --- fakes ---

type fakeRunner struct {
	output []byte
	err    error
}

func (f fakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
}

type fakeSyncer struct {
	calls []internalsync.Config
	err   error
}

func (f *fakeSyncer) Sync(_ []string, cfg internalsync.Config) error {
	f.calls = append(f.calls, cfg)
	return f.err
}

// twoWorktreePorcelain produces two worktrees; IsMain will be false for both
// in tests since the paths don't exist on disk. Tests use SourcePath to
// bypass the main-worktree lookup.
var twoWorktreePorcelain = []byte(`worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /repo/feat
HEAD def456
branch refs/heads/feat

`)

// --- orchestrator tests ---

func newTestOrchestrator(t *testing.T, runner worktree.CommandRunner, syncer internalsync.Syncer) *Orchestrator {
	t.Helper()
	return NewOrchestrator(runner, syncer, io.Discard)
}

func TestOrchestratorRun(t *testing.T) {
	t.Run("syncs non-source worktrees", func(t *testing.T) {
		syn := &fakeSyncer{}
		o := newTestOrchestrator(t, fakeRunner{output: twoWorktreePorcelain}, syn)

		err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main", SyncOnly: true})
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
		input   RunInput
		wantErr string
	}{
		{
			name:    "propagates syncer error",
			runner:  fakeRunner{output: twoWorktreePorcelain},
			syncer:  &fakeSyncer{err: errors.New("sync failed")},
			input:   RunInput{SourcePath: "/repo/main", SyncOnly: true},
			wantErr: "sync failed",
		},
		{
			name:    "no worktrees",
			runner:  fakeRunner{output: []byte{}},
			syncer:  &fakeSyncer{},
			input:   RunInput{SourcePath: "/repo/main"},
			wantErr: "no git worktrees found",
		},
		{
			name:    "runner error",
			runner:  fakeRunner{err: errors.New("git not found")},
			syncer:  &fakeSyncer{},
			input:   RunInput{},
			wantErr: "git not found",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			o := newTestOrchestrator(t, tt.runner, tt.syncer)
			err := o.Run(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// --- ResolveSourceDir tests ---

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
