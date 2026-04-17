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
			{output: twoPorcelain},           // git worktree list
			{output: []byte("feat\n")},       // git branch --merged
			{output: []byte("")},             // dirty: feat (clean)
			{err: errors.New("no upstream")}, // rev-parse @{upstream}: feat (no upstream)
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times, want 4 (no removes in dry-run)", runner.idx)
		}
	})

	t.Run("default removes merged worktree, artifact, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},           // git worktree list
			{output: []byte("feat\n")},       // git branch --merged
			{output: []byte("")},             // dirty: feat (clean)
			{err: errors.New("no upstream")}, // rev-parse @{upstream}: feat
			{},                               // git worktree remove
			{},                               // git branch -d
			{},                               // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 7 {
			t.Errorf("runner called %d times, want 7", runner.idx)
		}
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})

	t.Run("skips unmerged worktrees", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("")}, // nothing merged
			{},                   // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3", runner.idx)
		}
	})

	t.Run("skips target when cwd is inside it, continues others", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: threePorcelain},
			{output: []byte("feat\nother\n")}, // both merged
			{output: []byte("")},              // dirty: other (clean)
			{err: errors.New("no upstream")},  // rev-parse @{upstream}: other
			{},                                // git worktree remove (other)
			{},                                // git branch -d (other)
			{},                                // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		// cwd is inside featPath — feat should be skipped, other should be removed
		err := Prune(context.Background(), deps, PruneInput{
			Base:      "main",
			OutputDir: outputDir,
			Cwd:       featPath,
			Yes:       true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 7 {
			t.Errorf("runner called %d times, want 7 (skip feat, remove other)", runner.idx)
		}
	})

	t.Run("per-target failure does not stop remaining removals", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: threePorcelain},
			{output: []byte("feat\nother\n")},
			{output: []byte("")},                 // dirty: feat (clean)
			{err: errors.New("no upstream")},     // rev-parse @{upstream}: feat
			{output: []byte("")},                 // dirty: other (clean)
			{err: errors.New("no upstream")},     // rev-parse @{upstream}: other
			{err: errors.New("locked worktree")}, // git worktree remove feat fails
			{},                                   // git worktree remove other
			{},                                   // git branch -d other
			{},                                   // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err == nil {
			t.Fatal("expected error summarising failures, got nil")
		}
		if !strings.Contains(err.Error(), "feat") {
			t.Errorf("error %q should mention failed branch", err)
		}
		if runner.idx != 10 {
			t.Errorf("runner called %d times, want 10", runner.idx)
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
			{},                     // git worktree prune
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
		if rec.inner.idx != 4 {
			t.Errorf("runner called %d times, want 4", rec.inner.idx)
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

	t.Run("prunable worktrees are skipped and git worktree prune is called", func(t *testing.T) {
		prunablePath := mainPath + "-stale"
		porcelainWithPrunable := twoWorktreePorcelainWithPrunable(mainPath, prunablePath)

		runner := &seqRunner{responses: []runResponse{
			{output: porcelainWithPrunable}, // git worktree list (main + prunable stale-branch)
			{output: []byte("")},            // git branch --merged (nothing merged)
			{},                              // git worktree prune (cleans stale metadata)
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3", runner.idx)
		}
	})

	t.Run("dry-run logs git worktree prune when there are candidates", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("feat\n")},
			{output: []byte("")},             // dirty: feat (clean)
			{err: errors.New("no upstream")}, // rev-parse @{upstream}: feat
		}}
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("")}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "git worktree prune") {
			t.Errorf("dry-run output should mention 'git worktree prune'; got:\n%s", buf.String())
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times in dry-run, want 4", runner.idx)
		}
	})

	t.Run("skips dirty worktree with warning and does not remove it", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("feat\n")},   // git branch --merged
			{output: []byte("M f.go\n")}, // dirty: feat (dirty)
			{},                           // git worktree prune (no candidates remain)
		}}
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("")}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "skipping feat") || !strings.Contains(buf.String(), "uncommitted") {
			t.Errorf("expected skip warning for dirty worktree; got:\n%s", buf.String())
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times, want 4 (list, merged, dirty, prune)", runner.idx)
		}
	})

	t.Run("skips worktree with unpushed commits", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("feat\n")},        // git branch --merged
			{output: []byte("")},              // dirty: feat (clean)
			{output: []byte("origin/feat\n")}, // rev-parse @{upstream}: has upstream
			{output: []byte("2\t0\n")},        // rev-list: 2 ahead, 0 behind
			{},                                // git worktree prune
		}}
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("")}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "skipping feat") || !strings.Contains(buf.String(), "unpushed") {
			t.Errorf("expected skip warning for unpushed commits; got:\n%s", buf.String())
		}
		if runner.idx != 6 {
			t.Errorf("runner called %d times, want 6", runner.idx)
		}
	})

	t.Run("confirmation prompt aborts on n", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("feat\n")},
			{output: []byte("")},             // dirty: feat (clean)
			{err: errors.New("no upstream")}, // rev-parse: feat (no upstream)
		}}
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("n\n")}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aborted") {
			t.Errorf("expected 'aborted' in output; got:\n%s", buf.String())
		}
		// Must not reach removal or prune step.
		if runner.idx != 4 {
			t.Errorf("runner called %d times after abort, want 4", runner.idx)
		}
	})

	t.Run("confirmation prompt proceeds on y", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{output: twoPorcelain},
			{output: []byte("feat\n")},
			{output: []byte("")},             // dirty: feat (clean)
			{err: errors.New("no upstream")}, // rev-parse
			{},                               // git worktree remove
			{},                               // git branch -d
			{},                               // git worktree prune
		}}
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, Log: ui.New(&buf), In: strings.NewReader("y\n")}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 7 {
			t.Errorf("runner called %d times, want 7", runner.idx)
		}
	})
}
