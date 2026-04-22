package treepad

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/slug"
	"treepad/internal/worktree"
)

func TestRemove(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))

	mainWT := worktree.Worktree{Branch: "main", Path: mainPath, IsMain: true}
	featWT := worktree.Worktree{Branch: "feat", Path: featPath}

	newFake := func() *fakeRepoView {
		return &fakeRepoView{
			main:      mainWT,
			worktrees: []worktree.Worktree{mainWT, featWT},
			slug:      repoSlug,
			outputDir: outputDir,
		}
	}
	withFake := func(f *fakeRepoView) func(context.Context, string) (RepoView, error) {
		return func(_ context.Context, _ string) (RepoView, error) { return f, nil }
	}

	t.Run("removes worktree, artifact file, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{}, // git worktree remove
			{}, // git branch -d
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
			t.Error("artifact file should have been deleted")
		}
		if runner.idx != 2 {
			t.Errorf("runner called %d times, want 2 (worktree remove + branch -d)", runner.idx)
		}
	})

	t.Run("artifact file missing is not an error", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{{}, {}}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fires PreRemove and PostRemove hooks", func(t *testing.T) {
		toml := "[[hooks.pre_remove]]\ncommand = \"marker-pre\"\n\n[[hooks.post_remove]]\ncommand = \"marker-post\"\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{{}, {}}}
		hr := &fakeHookRunner{}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr
		deps.NewRepoView = withFake(newFake())

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.calls) != 2 {
			t.Fatalf("hook runner called %d times, want 2", len(hr.calls))
		}
		if got := hr.calls[0].data.HookType; got != "pre_remove" {
			t.Errorf("calls[0].HookType = %q, want pre_remove", got)
		}
		if got := hr.calls[1].data.HookType; got != "post_remove" {
			t.Errorf("calls[1].HookType = %q, want post_remove", got)
		}
		if got := hr.calls[0].data.Branch; got != "feat" {
			t.Errorf("hook data Branch = %q, want feat", got)
		}
	})

	t.Run("PreRemove failure aborts before git worktree remove", func(t *testing.T) {
		toml := "[[hooks.pre_remove]]\ncommand = \"fail\"\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		rr := &recordingRunner{inner: &seqRunner{responses: []runResponse{}}}
		hr := &fakeHookRunner{err: errors.New("pre remove aborted")}
		deps := testDeps(rr, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr
		deps.NewRepoView = withFake(newFake())

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "pre remove aborted") {
			t.Errorf("got error %v, want error containing 'pre remove aborted'", err)
		}
		for _, call := range rr.calls {
			if len(call) >= 3 && call[1] == "worktree" && call[2] == "remove" {
				t.Error("git worktree remove should not be called when pre_remove hook fails")
			}
		}
	})

	t.Run("PostRemove failure logs warning but does not abort", func(t *testing.T) {
		toml := "[[hooks.post_remove]]\ncommand = \"fail\"\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{{}, {}}}
		hr := &fakeHookRunner{err: errors.New("post remove failed")}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr
		deps.NewRepoView = withFake(newFake())

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Errorf("PostRemove hook failure should not abort operation, got error: %v", err)
		}
	})

	t.Run("refuses to remove the main worktree", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		err := Remove(context.Background(), deps, RemoveInput{Branch: "main", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "main worktree") {
			t.Errorf("got error %v, want error containing 'main worktree'", err)
		}
	})

	t.Run("branch not found returns clear error", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(&fakeRepoView{
			main:      mainWT,
			worktrees: []worktree.Worktree{mainWT}, // feat not present
			outputDir: outputDir,
		})

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), `no worktree found for branch "feat"`) {
			t.Errorf("got error %v, want error containing branch-not-found message", err)
		}
	})

	t.Run("git worktree remove failure propagates", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{err: errors.New("locked worktree")},
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "locked worktree") {
			t.Errorf("got error %v, want error containing 'locked worktree'", err)
		}
	})

	t.Run("git branch -d failure propagates", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{},
			{err: errors.New("branch not found")},
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "branch not found") {
			t.Errorf("got error %v, want error containing 'branch not found'", err)
		}
	})

	t.Run("refuses to remove worktree user is currently in", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		err := Remove(context.Background(), deps, RemoveInput{
			Branch:    "feat",
			OutputDir: outputDir,
			Cwd:       featPath,
		})
		if err == nil || !strings.Contains(err.Error(), "currently in") {
			t.Errorf("got error %v, want error containing 'currently in'", err)
		}
		if runner.idx != 0 {
			t.Errorf("runner called %d times, want 0 (no git calls after guard)", runner.idx)
		}
	})

	t.Run("NewRepoView failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("git not found")
		}
		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "git not found") {
			t.Errorf("got error %v, want error containing 'git not found'", err)
		}
	})
}
