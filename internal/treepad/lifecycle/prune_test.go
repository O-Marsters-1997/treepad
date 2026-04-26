package lifecycle

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/slug"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
	"treepad/internal/ui"
)

func TestPrune(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	otherPath := mainPath + "-other"
	twoPorcelain := treepadtest.TwoWorktreePorcelainWithMain(mainPath, featPath)
	threePorcelain := treepadtest.ThreeWorktreePorcelainWithMain(mainPath, featPath, otherPath)

	t.Run("dry-run lists candidates without removing", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},            // git worktree list
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Err: errors.New("no upstream")},  // rev-parse @{upstream}: feat (no upstream)
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("default removes merged worktree, artifact, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},            // git worktree list
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Err: errors.New("no upstream")},  // rev-parse @{upstream}: feat
			{},                                // git worktree remove
			{},                                // git branch -d
			{},                                // git worktree prune
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})

	t.Run("skips unmerged worktrees", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{Output: []byte("")},         // git for-each-ref --merged (empty)
			{},                           // git worktree prune
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if runner.Idx != 4 {
			t.Errorf("runner called %d times, want 4 (list, rev-parse, for-each-ref, prune)", runner.Idx)
		}
	})

	t.Run("skips fresh worktree whose tip equals base tip", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat aaa111\n")}, // feat tip == main tip; filtered out
			{},                                // git worktree prune
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if runner.Idx != 4 {
			t.Errorf("runner called %d times, want 4 (list, rev-parse, for-each-ref, prune)", runner.Idx)
		}
	})

	t.Run("skips target when cwd is inside it, continues others", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: threePorcelain},
			{Output: []byte("aaa111\n")},                    // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\nother ccc333\n")}, // both merged
			{Output: []byte("")},                            // dirty: other (clean)
			{Err: errors.New("no upstream")},                // rev-parse @{upstream}: other
			{},                                              // git worktree remove (other)
			{},                                              // git branch -d (other)
			{},                                              // git worktree prune
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

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
		if runner.Idx != 8 {
			t.Errorf("runner called %d times, want 8 (skip feat, remove other)", runner.Idx)
		}
	})

	t.Run("per-target failure does not stop remaining removals", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: threePorcelain},
			{Output: []byte("aaa111\n")},                    // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\nother ccc333\n")}, // git for-each-ref --merged
			{Output: []byte("")},                            // dirty: feat (clean)
			{Err: errors.New("no upstream")},                // rev-parse @{upstream}: feat
			{Output: []byte("")},                            // dirty: other (clean)
			{Err: errors.New("no upstream")},                // rev-parse @{upstream}: other
			{Err: errors.New("locked worktree")},            // git worktree remove feat fails
			{},                                              // git worktree remove other
			{},                                              // git branch -d other
			{},                                              // git worktree prune
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err == nil {
			t.Fatal("expected error summarising failures, got nil")
		}
		if !strings.Contains(err.Error(), "feat") {
			t.Errorf("error %q should mention failed branch", err)
		}
		if runner.Idx != 11 {
			t.Errorf("runner called %d times, want 11", runner.Idx)
		}
	})

	errorTests := []struct {
		name    string
		runner  *treepadtest.SeqRunner
		wantErr string
	}{
		{
			name: "git worktree list fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Err: errors.New("git not found")},
			}},
			wantErr: "git not found",
		},
		{
			name: "git rev-parse base fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: twoPorcelain},
				{Err: errors.New("unknown revision")},
			}},
			wantErr: "unknown revision",
		},
		{
			name: "git for-each-ref --merged fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: twoPorcelain},
				{Output: []byte("aaa111\n")},
				{Err: errors.New("for-each-ref failed")},
			}},
			wantErr: "for-each-ref failed",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := deps.Deps{Runner: tt.runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
			err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("--all errors when not on main worktree", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain}, // git worktree list
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

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
		if runner.Idx != 1 {
			t.Errorf("runner called %d times, want 1 (list only)", runner.Idx)
		}
	})

	t.Run("--all dry-run lists all non-main worktrees without removing", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: threePorcelain}, // git worktree list
		}}
		var buf strings.Builder
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			DryRun:    true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.Idx != 1 {
			t.Errorf("runner called %d times, want 1 (list only, no merged check)", runner.Idx)
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
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain}, // git worktree list
		}}
		var buf strings.Builder
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader("n\n"),
		}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.Idx != 1 {
			t.Errorf("runner called %d times after abort, want 1 (list only)", runner.Idx)
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

		rec := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain}, // git worktree list
			{},                     // git worktree remove --force
			{},                     // git branch -D
			{},                     // git worktree prune
		}}}
		deps := deps.Deps{
			Runner: rec,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    io.Discard,
			In:     strings.NewReader("y\n"),
		}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.Inner.Idx != 4 {
			t.Errorf("runner called %d times, want 4", rec.Inner.Idx)
		}
		// Verify --force flag was passed to git worktree remove.
		if len(rec.Calls) < 2 || rec.Calls[1][3] != "--force" {
			t.Errorf("expected 'git worktree remove --force ...', got calls: %v", rec.Calls)
		}
		// Verify -D flag was passed to git branch.
		if len(rec.Calls) < 3 || rec.Calls[2][2] != "-D" {
			t.Errorf("expected 'git branch -D ...', got calls: %v", rec.Calls)
		}
		// Artifact file should have been deleted.
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})

	t.Run("--all --yes skips confirmation prompt", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		rec := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain}, // git worktree list
			{},                     // git worktree remove --force
			{},                     // git branch -D
			{},                     // git worktree prune
		}}}
		deps := deps.Deps{
			Runner: rec,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    io.Discard,
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			Yes:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.Inner.Idx != 4 {
			t.Errorf("runner called %d times, want 4 (list, remove, branch-D, prune)", rec.Inner.Idx)
		}
	})

	t.Run("--all with no candidates runs git worktree prune", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: treepadtest.MainWorktreePorcelain(mainPath)}, // git worktree list (main only)
			{}, // git worktree prune
		}}
		var buf strings.Builder
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.Idx != 2 {
			t.Errorf("runner called %d times, want 2 (list + git worktree prune)", runner.Idx)
		}
		if !strings.Contains(buf.String(), "no worktrees to remove") {
			t.Errorf("output missing empty message; got:\n%s", buf.String())
		}
	})

	t.Run("prunable worktrees are skipped and git worktree prune is called", func(t *testing.T) {
		prunablePath := mainPath + "-stale"
		porcelainWithPrunable := treepadtest.TwoWorktreePorcelainWithPrunable(mainPath, prunablePath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelainWithPrunable}, // git worktree list (main + prunable stale-branch)
			{Output: []byte("aaa111\n")},    // git rev-parse main^{commit}
			{Output: []byte("")},            // git for-each-ref --merged (nothing merged)
			{},                              // git worktree prune (cleans stale metadata)
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.Idx != 4 {
			t.Errorf("runner called %d times, want 4", runner.Idx)
		}
	})

	t.Run("dry-run logs git worktree prune when there are candidates", func(t *testing.T) {
		var buf strings.Builder
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Err: errors.New("no upstream")},  // rev-parse @{upstream}: feat
		}}
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "git worktree prune") {
			t.Errorf("dry-run output should mention 'git worktree prune'; got:\n%s", buf.String())
		}
		if runner.Idx != 5 {
			t.Errorf("runner called %d times in dry-run, want 5", runner.Idx)
		}
	})

	t.Run("skips dirty worktree with warning and does not remove it", func(t *testing.T) {
		var buf strings.Builder
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("M f.go\n")},      // dirty: feat (dirty)
			{},                                // git worktree prune (no candidates remain)
		}}
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "skipping feat") || !strings.Contains(buf.String(), "uncommitted") {
			t.Errorf("expected skip warning for dirty worktree; got:\n%s", buf.String())
		}
		if runner.Idx != 5 {
			t.Errorf("runner called %d times, want 5 (list, rev-parse, for-each-ref, dirty, prune)", runner.Idx)
		}
	})

	t.Run("skips worktree with unpushed commits", func(t *testing.T) {
		var buf strings.Builder
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Output: []byte("origin/feat\n")}, // rev-parse @{upstream}: has upstream
			{Output: []byte("2\t0\n")},        // rev-list: 2 ahead, 0 behind
			{},                                // git worktree prune
		}}
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader(""),
		}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "skipping feat") || !strings.Contains(buf.String(), "unpushed") {
			t.Errorf("expected skip warning for unpushed commits; got:\n%s", buf.String())
		}
		if runner.Idx != 7 {
			t.Errorf("runner called %d times, want 7", runner.Idx)
		}
	})

	t.Run("confirmation prompt aborts on n", func(t *testing.T) {
		var buf strings.Builder
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Err: errors.New("no upstream")},  // rev-parse: feat (no upstream)
		}}
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader("n\n"),
		}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aborted") {
			t.Errorf("expected 'aborted' in output; got:\n%s", buf.String())
		}
		// Must not reach removal or prune step.
		if runner.Idx != 5 {
			t.Errorf("runner called %d times after abort, want 5", runner.Idx)
		}
	})

	t.Run("confirmation prompt proceeds on y", func(t *testing.T) {
		var buf strings.Builder
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: twoPorcelain},
			{Output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{Output: []byte("feat bbb222\n")}, // git for-each-ref --merged
			{Output: []byte("")},              // dirty: feat (clean)
			{Err: errors.New("no upstream")},  // rev-parse
			{},                                // git worktree remove
			{},                                // git branch -d
			{},                                // git worktree prune
		}}
		deps := deps.Deps{
			Runner: runner,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    ui.New(&buf),
			In:     strings.NewReader("y\n"),
		}

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.Idx != 8 {
			t.Errorf("runner called %d times, want 8", runner.Idx)
		}
	})
}
