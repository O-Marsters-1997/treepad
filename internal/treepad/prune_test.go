package treepad

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/slug"
	"treepad/internal/ui"
)

func TestPrune(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	otherPath := mainPath + "-other"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	twoPorcelain := twoWorktreePorcelainWithMain(mainPath, featPath)
	threePorcelain := threeWorktreePorcelainWithMain(mainPath, featPath, otherPath)

	t.Run("dry-run lists candidates without removing", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},     // git worktree list
			{output: []byte("feat\n")}, // git branch --merged
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 2 {
			t.Errorf("runner called %d times, want 2 (no removes in dry-run)", runner.idx)
		}
	})

	t.Run("default removes merged worktree, artifact, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},     // git worktree list
			{output: []byte("feat\n")}, // git branch --merged
			{},                         // git worktree remove
			{},                         // git branch -d
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times, want 4", runner.idx)
		}
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})

	t.Run("skips unmerged worktrees", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("")}, // nothing merged
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 2 {
			t.Errorf("runner called %d times, want 2", runner.idx)
		}
	})

	t.Run("skips target when cwd is inside it, continues others", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: threePorcelain},
			{output: []byte("feat\nother\n")}, // both merged
			{},                                // git worktree remove (other)
			{},                                // git branch -d (other)
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		// cwd is inside featPath — feat should be skipped, other should be removed
		err := Prune(context.Background(), deps, PruneInput{
			Base:      "main",
			OutputDir: outputDir,
			Cwd:       featPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times, want 4 (skip feat, remove other)", runner.idx)
		}
	})

	t.Run("per-target failure does not stop remaining removals", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: threePorcelain},
			{output: []byte("feat\nother\n")},
			{err: errors.New("locked worktree")}, // git worktree remove feat fails
			{},                                   // git worktree remove other
			{},                                   // git branch -d other
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err == nil {
			t.Fatal("expected error summarising failures, got nil")
		}
		if !strings.Contains(err.Error(), "feat") {
			t.Errorf("error %q should mention failed branch", err)
		}
		if runner.idx != 5 {
			t.Errorf("runner called %d times, want 5", runner.idx)
		}
	})

	errorTests := []struct {
		name    string
		runner  *seqRunner
		wantErr string
	}{
		{
			name: "git worktree list fails",
			runner: &seqRunner{responses: []runResponse{
				{err: errors.New("git not found")},
			}},
			wantErr: "git not found",
		},
		{
			name: "git branch --merged fails",
			runner: &seqRunner{responses: []runResponse{
				{output: twoPorcelain},
				{err: errors.New("unknown branch")},
			}},
			wantErr: "unknown branch",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(tt.runner, &fakeSyncer{}, &fakeOpener{})
			err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("--all errors when not on main worktree", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain}, // git worktree list
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       featPath, // not the main worktree
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "--all must be run from the main worktree") {
			t.Errorf("unexpected error: %v", err)
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times, want 1 (list only)", runner.idx)
		}
	})

	t.Run("--all dry-run lists all non-main worktrees without removing", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: threePorcelain}, // git worktree list
		}}
		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("")}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			DryRun:    true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times, want 1 (list only, no merged check)", runner.idx)
		}
		out := buf.String()
		if !strings.Contains(out, "would remove: feat") {
			t.Errorf("output missing feat worktree; got:\n%s", out)
		}
		if !strings.Contains(out, "would remove: other") {
			t.Errorf("output missing other worktree; got:\n%s", out)
		}
	})

	t.Run("--all aborts on negative confirmation", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain}, // git worktree list
		}}
		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("n\n")}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times after abort, want 1 (list only)", runner.idx)
		}
		if !strings.Contains(buf.String(), "aborted") {
			t.Errorf("output should contain 'aborted'; got:\n%s", buf.String())
		}
	})

	t.Run("--all force-removes on yes", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		rec := &recordingRunner{inner: &seqRunner{responses: []runResponse{
			{output: twoPorcelain}, // git worktree list
			{},                     // git worktree remove --force
			{},                     // git branch -D
		}}}
		deps := Deps{Runner: rec, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: io.Discard, In: strings.NewReader("y\n")}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.inner.idx != 3 {
			t.Errorf("runner called %d times, want 3", rec.inner.idx)
		}
		// Verify --force flag was passed to git worktree remove.
		if len(rec.calls) < 2 || rec.calls[1][3] != "--force" {
			t.Errorf("expected 'git worktree remove --force ...', got calls: %v", rec.calls)
		}
		// Verify -D flag was passed to git branch.
		if len(rec.calls) < 3 || rec.calls[2][2] != "-D" {
			t.Errorf("expected 'git branch -D ...', got calls: %v", rec.calls)
		}
		// Artifact file should have been deleted.
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})
}
