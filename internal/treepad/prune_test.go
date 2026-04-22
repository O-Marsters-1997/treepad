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
	"treepad/internal/worktree"
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

	mainWT := worktree.Worktree{Branch: "main", Path: mainPath, IsMain: true}
	featWT := worktree.Worktree{Branch: "feat", Path: featPath}
	otherWT := worktree.Worktree{Branch: "other", Path: otherPath}

	twoWTs := []worktree.Worktree{mainWT, featWT}
	threeWTs := []worktree.Worktree{mainWT, featWT, otherWT}

	newFake := func(wts []worktree.Worktree, opts ...func(*fakeRepoView)) *fakeRepoView {
		f := &fakeRepoView{
			main:      mainWT,
			worktrees: wts,
			slug:      repoSlug,
			outputDir: outputDir,
		}
		for _, o := range opts {
			o(f)
		}
		return f
	}
	withFake := func(f *fakeRepoView) func(context.Context, string) (RepoView, error) {
		return func(_ context.Context, _ string) (RepoView, error) { return f, nil }
	}

	t.Run("dry-run lists candidates without removing", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 0 {
			t.Errorf("runner called %d times, want 0 (no removes in dry-run)", runner.idx)
		}
	})

	t.Run("default removes merged+clean worktree, artifact, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree remove
			{}, // git branch -d
			{}, // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3 (remove + branch-d + prune)", runner.idx)
		}
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})

	t.Run("skips unmerged worktrees and prunes metadata", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(twoWTs)) // no merged branches

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times, want 1 (prune only)", runner.idx)
		}
	})

	t.Run("skips target when cwd is inside it, continues others", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree remove (other)
			{}, // git branch -d (other)
			{}, // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(threeWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat", "other"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{
			Base:      "main",
			OutputDir: outputDir,
			Cwd:       featPath,
			Yes:       true,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3 (skip feat, remove other + prune)", runner.idx)
		}
	})

	t.Run("per-target failure does not stop remaining removals", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{err: errors.New("locked worktree")}, // git worktree remove feat fails
			{},                                   // git worktree remove other
			{},                                   // git branch -d other
			{},                                   // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(threeWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat", "other"}}
		}))

		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true})
		if err == nil {
			t.Fatal("expected error summarising failures, got nil")
		}
		if !strings.Contains(err.Error(), "feat") {
			t.Errorf("error %q should mention failed branch", err)
		}
		if runner.idx != 4 {
			t.Errorf("runner called %d times, want 4", runner.idx)
		}
	})

	t.Run("skips dirty worktree with warning and does not remove it", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree prune (no candidates remain)
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.Log = ui.New(&buf)
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
			f.dirtyByBranch = map[string]bool{"feat": true}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "skipping feat") || !strings.Contains(out, "uncommitted") {
			t.Errorf("expected skip warning for dirty worktree; got:\n%s", out)
		}
	})

	t.Run("skips worktree with unpushed commits", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.Log = ui.New(&buf)
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
			f.aheadBehindByBranch = map[string]fakeAheadBehind{
				"feat": {A: 2, B: 0, HasUpstream: true},
			}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, Yes: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "skipping feat") || !strings.Contains(out, "unpushed") {
			t.Errorf("expected skip warning for unpushed commits; got:\n%s", out)
		}
	})

	t.Run("confirmation prompt aborts on n", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.In = strings.NewReader("n\n")
		deps.Log = ui.New(&buf)
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aborted") {
			t.Errorf("expected 'aborted' in output; got:\n%s", buf.String())
		}
		if runner.idx != 0 {
			t.Errorf("runner called %d times after abort, want 0", runner.idx)
		}
	})

	t.Run("confirmation prompt proceeds on y", func(t *testing.T) {
		var buf strings.Builder
		runner := &seqRunner{responses: []runResponse{{}, {}, {}}} // remove + branch-d + prune
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.In = strings.NewReader("y\n")
		deps.Log = ui.New(&buf)
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3 (remove + branch-d + prune)", runner.idx)
		}
	})

	t.Run("dry-run logs git worktree prune when there are candidates", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Log = ui.New(&buf)
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(twoWTs, func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir, DryRun: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "git worktree prune") {
			t.Errorf("dry-run output should mention 'git worktree prune'; got:\n%s", buf.String())
		}
	})

	t.Run("prunable worktrees skipped and git worktree prune called", func(t *testing.T) {
		prunablePath := mainPath + "-stale"
		prunableWT := worktree.Worktree{
			Branch: "stale-branch", Path: prunablePath, Prunable: true,
			PrunableReason: "gitdir file points to non-existent location",
		}
		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree prune
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake([]worktree.Worktree{mainWT, prunableWT}))

		if err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times, want 1 (prune only; prunable not removed)", runner.idx)
		}
	})

	t.Run("NewRepoView failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("git not found")
		}
		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "git not found") {
			t.Errorf("got error %v, want error containing 'git not found'", err)
		}
	})

	t.Run("MergedInto failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("unknown branch")
		}
		err := Prune(context.Background(), deps, PruneInput{Base: "main", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "unknown branch") {
			t.Errorf("got error %v, want error containing 'unknown branch'", err)
		}
	})

	// --all tests

	t.Run("--all errors when not on main worktree", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(twoWTs))

		err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       featPath, // not the main worktree
		})
		if err == nil || !strings.Contains(err.Error(), "--all must be run from the main worktree") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("--all dry-run lists all non-main worktrees without removing", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Log = ui.New(&buf)
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(threeWTs))

		if err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			DryRun:    true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
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
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.In = strings.NewReader("n\n")
		deps.Log = ui.New(&buf)
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(twoWTs))

		if err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aborted") {
			t.Errorf("output should contain 'aborted'; got:\n%s", buf.String())
		}
	})

	t.Run("--all force-removes on yes and passes --force and -D flags", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		rec := &recordingRunner{inner: &seqRunner{responses: []runResponse{
			{}, // git worktree remove --force
			{}, // git branch -D
			{}, // git worktree prune
		}}}
		deps := testDeps(rec, &fakeSyncer{}, &fakeOpener{})
		deps.In = strings.NewReader("y\n")
		deps.Out = io.Discard
		deps.NewRepoView = withFake(newFake(twoWTs))

		if err := Prune(context.Background(), deps, PruneInput{
			All:       true,
			OutputDir: outputDir,
			Cwd:       mainPath,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.inner.idx != 3 {
			t.Errorf("runner called %d times, want 3", rec.inner.idx)
		}
		if len(rec.calls) < 1 || rec.calls[0][3] != "--force" {
			t.Errorf("expected 'git worktree remove --force ...', got calls: %v", rec.calls)
		}
		if len(rec.calls) < 2 || rec.calls[1][2] != "-D" {
			t.Errorf("expected 'git branch -D ...', got calls: %v", rec.calls)
		}
		if _, statErr := os.Stat(wsFile); !os.IsNotExist(statErr) {
			t.Error("artifact file should have been deleted")
		}
	})
}
