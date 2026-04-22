package treepad

import (
	"context"
	"errors"
	"strings"
	"testing"

	"treepad/internal/worktree"
)

func TestGenerate(t *testing.T) {
	mainWT := worktree.Worktree{Branch: "main", Path: "/repo/main", IsMain: true}
	featWT := worktree.Worktree{Branch: "feat", Path: "/repo/feat"}
	otherWT := worktree.Worktree{Branch: "other", Path: "/repo/other"}

	twoFake := &fakeRepoView{
		main:      mainWT,
		worktrees: []worktree.Worktree{mainWT, featWT},
	}
	threeFake := &fakeRepoView{
		main:      mainWT,
		worktrees: []worktree.Worktree{mainWT, featWT, otherWT},
	}

	withFake := func(f *fakeRepoView) func(context.Context, string) (RepoView, error) {
		return func(_ context.Context, _ string) (RepoView, error) { return f, nil }
	}

	t.Run("syncs non-source worktrees", func(t *testing.T) {
		syn := &fakeSyncer{}
		deps := testDeps(fakeRunner{}, syn, nil)
		deps.NewRepoView = withFake(twoFake)

		if err := Generate(context.Background(), deps, GenerateInput{SourcePath: "/repo/main", SyncOnly: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.calls))
		}
		if syn.calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.calls[0].TargetDir)
		}
	})

	t.Run("Branch filters to one target", func(t *testing.T) {
		syn := &fakeSyncer{}
		deps := testDeps(fakeRunner{}, syn, nil)
		deps.NewRepoView = withFake(threeFake)

		if err := Generate(context.Background(), deps, GenerateInput{
			SourcePath: "/repo/main",
			SyncOnly:   true,
			Branch:     "feat",
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 1 {
			t.Fatalf("syncer called %d times, want 1 for branch filter", len(syn.calls))
		}
		if syn.calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.calls[0].TargetDir)
		}
	})

	t.Run("unknown Branch returns clear error", func(t *testing.T) {
		syn := &fakeSyncer{}
		deps := testDeps(fakeRunner{}, syn, nil)
		deps.NewRepoView = withFake(threeFake)

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
		syn := &fakeSyncer{}
		deps := testDeps(fakeRunner{}, syn, nil)
		deps.NewRepoView = withFake(threeFake)

		if err := Generate(context.Background(), deps, GenerateInput{
			SourcePath: "/repo/main",
			SyncOnly:   true,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 2 {
			t.Errorf("syncer called %d times, want 2 for fleet sync", len(syn.calls))
		}
	})

	t.Run("propagates syncer error", func(t *testing.T) {
		syn := &fakeSyncer{err: errors.New("sync failed")}
		deps := testDeps(fakeRunner{}, syn, nil)
		deps.NewRepoView = withFake(twoFake)

		err := Generate(context.Background(), deps, GenerateInput{SourcePath: "/repo/main", SyncOnly: true})
		if err == nil || !strings.Contains(err.Error(), "sync failed") {
			t.Errorf("got error %v, want containing 'sync failed'", err)
		}
	})

	t.Run("NewRepoView failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, nil)
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("git not found")
		}
		err := Generate(context.Background(), deps, GenerateInput{SourcePath: "/repo/main"})
		if err == nil || !strings.Contains(err.Error(), "git not found") {
			t.Errorf("got error %v, want containing 'git not found'", err)
		}
	})
}
