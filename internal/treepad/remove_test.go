package treepad

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/slug"
)

func TestRemove(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	porcelain := twoWorktreePorcelainWithMain(mainPath, featPath)

	t.Run("removes worktree, artifact file, and branch", func(t *testing.T) {
		// Default config template: <slug>-<branch>.code-workspace
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain}, // git worktree list
			{},                  // git worktree remove
			{},                  // git branch -d
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
			t.Error("artifact file should have been deleted")
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3", runner.idx)
		}
	})

	t.Run("artifact file missing is not an error", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{},
			{},
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fires PreRemove and PostRemove hooks", func(t *testing.T) {
		toml := "[hooks]\npre_remove = [\"marker-pre\"]\npost_remove = [\"marker-post\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{},
			{},
		}}
		hr := &fakeHookRunner{}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

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
		toml := "[hooks]\npre_remove = [\"fail\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		rr := &recordingRunner{inner: &seqRunner{responses: []runResponse{
			{output: porcelain},
		}}}
		hr := &fakeHookRunner{err: errors.New("pre remove aborted")}
		deps := testDeps(rr, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

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
		toml := "[hooks]\npost_remove = [\"fail\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{},
			{},
		}}
		hr := &fakeHookRunner{err: errors.New("post remove failed")}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Errorf("PostRemove hook failure should not abort operation, got error: %v", err)
		}
	})

	errorTests := []struct {
		name    string
		runner  *seqRunner
		branch  string
		wantErr string
	}{
		{
			name:   "git worktree list fails",
			branch: "feat",
			runner: &seqRunner{responses: []runResponse{
				{err: errors.New("git not found")},
			}},
			wantErr: "git not found",
		},
		{
			name:   "branch not found in worktree list",
			branch: "feat",
			runner: &seqRunner{responses: []runResponse{
				{output: mainWorktreePorcelain(mainPath)},
			}},
			wantErr: `no worktree found for branch "feat"`,
		},
		{
			name:   "git worktree remove fails",
			branch: "feat",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{err: errors.New("locked worktree")},
			}},
			wantErr: "locked worktree",
		},
		{
			name:   "git branch -d fails",
			branch: "feat",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{},
				{err: errors.New("branch not found")},
			}},
			wantErr: "branch not found",
		},
		{
			name:   "refuses to remove main worktree",
			branch: "main",
			runner: &seqRunner{responses: []runResponse{
				{output: mainWorktreePorcelain(mainPath)},
			}},
			wantErr: "main worktree",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(tt.runner, &fakeSyncer{}, &fakeOpener{})
			err := Remove(context.Background(), deps, RemoveInput{Branch: tt.branch, OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("refuses to remove worktree user is currently in", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
		}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Remove(context.Background(), deps, RemoveInput{
			Branch:    "feat",
			OutputDir: outputDir,
			Cwd:       featPath,
		})
		if err == nil || !strings.Contains(err.Error(), "currently in") {
			t.Errorf("got error %v, want error containing %q", err, "currently in")
		}
		if runner.idx != 1 {
			t.Errorf("runner called %d times after guard, want 1 (list only)", runner.idx)
		}
	})
}
