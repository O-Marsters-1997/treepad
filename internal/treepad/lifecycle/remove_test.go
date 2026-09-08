package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/hook"
	"github.com/O-Marsters-1997/treepad/internal/slug"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

func TestRemove(t *testing.T) {
	mainPath := makeMainWorktree(t)
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	porcelain := treepadtest.TwoWorktreePorcelainWithMain(mainPath, featPath)

	t.Run("removes worktree, artifact file, and branch", func(t *testing.T) {
		// Default config template: <slug>-<branch>.code-workspace
		wsFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list
			{},                  // git merge-base --is-ancestor
			{},                  // git worktree remove
			{},                  // git branch -d
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
			t.Error("artifact file should have been deleted")
		}
		if runner.Idx != 4 {
			t.Errorf("runner called %d times, want 4", runner.Idx)
		}
	})

	t.Run("artifact file missing is not an error", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{},
			{},
			{},
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fires PreRemove and PostRemove hooks", func(t *testing.T) {
		toml := "[[hooks.pre_remove]]\ncommand = \"marker-pre\"\n\n[[hooks.post_remove]]\ncommand = \"marker-post\"\n"
		writeTOML(t, mainPath, toml)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{},
			{},
			{},
		}}
		hr := &treepadtest.FakeHookRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.Calls) != 2 {
			t.Fatalf("hook runner called %d times, want 2", len(hr.Calls))
		}
		if got := hr.Calls[0].Data.HookType; got != "pre_remove" {
			t.Errorf("calls[0].HookType = %q, want pre_remove", got)
		}
		if got := hr.Calls[1].Data.HookType; got != "post_remove" {
			t.Errorf("calls[1].HookType = %q, want post_remove", got)
		}
		if got := hr.Calls[0].Data.Branch; got != "feat" {
			t.Errorf("hook data Branch = %q, want feat", got)
		}
	})

	t.Run("PreRemove failure aborts before git worktree remove", func(t *testing.T) {
		toml := "[[hooks.pre_remove]]\ncommand = \"fail\"\n"
		writeTOML(t, mainPath, toml)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{}, // git merge-base --is-ancestor
		}}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("pre remove aborted")}
		deps := deps.Deps{Runner: rr.Inner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "pre remove aborted") {
			t.Errorf("got error %v, want error containing 'pre remove aborted'", err)
		}
		for _, call := range rr.Calls {
			if len(call) >= 3 && call[1] == "worktree" && call[2] == "remove" {
				t.Error("git worktree remove should not be called when pre_remove hook fails")
			}
		}
	})

	t.Run("PostRemove failure logs warning but does not abort", func(t *testing.T) {
		toml := "[[hooks.post_remove]]\ncommand = \"fail\"\n"
		writeTOML(t, mainPath, toml)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{},
			{},
			{},
		}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("post remove failed")}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		if err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir}); err != nil {
			t.Errorf("PostRemove hook failure should not abort operation, got error: %v", err)
		}
	})

	errorTests := []struct {
		name    string
		runner  *treepadtest.SeqRunner
		branch  string
		wantErr string
	}{
		{
			name:   "git worktree list fails",
			branch: "feat",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Err: errors.New("git not found")},
			}},
			wantErr: "git not found",
		},
		{
			name:   "branch not found in worktree list",
			branch: "feat",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: treepadtest.MainWorktreePorcelain(mainPath)},
			}},
			wantErr: `no worktree found for branch "feat"`,
		},
		{
			name:   "git worktree remove fails",
			branch: "feat",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{}, // git merge-base --is-ancestor
				{Err: errors.New("locked worktree")},
			}},
			wantErr: "locked worktree",
		},
		{
			name:   "git branch -d fails",
			branch: "feat",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{}, // git merge-base --is-ancestor
				{},
				{Err: errors.New("branch not found")},
			}},
			wantErr: "branch not found",
		},
		{
			name:   "refuses to remove main worktree",
			branch: "main",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: treepadtest.MainWorktreePorcelain(mainPath)},
			}},
			wantErr: "main worktree",
		},
		{
			name:   "plain remove refuses a branch that is not an ancestor of main",
			branch: "feat",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{Err: errors.New("not an ancestor")}, // git merge-base --is-ancestor
			}},
			wantErr: `branch "feat" is not merged into main`,
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := deps.Deps{Runner: tt.runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
			err := Remove(context.Background(), deps, RemoveInput{Branch: tt.branch, OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("force remove passes --force and -D to git", func(t *testing.T) {
		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list
			{},                  // git worktree remove --force
			{},                  // git branch -D
		}}}
		deps := deps.Deps{
			Runner: rr,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
		}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir, Force: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var foundForce, foundD bool
		for _, call := range rr.Calls {
			if len(call) >= 4 && call[1] == "worktree" && call[2] == "remove" && call[3] == "--force" {
				foundForce = true
			}
			if len(call) >= 4 && call[1] == "branch" && call[2] == "-D" {
				foundD = true
			}
		}
		if !foundForce {
			t.Error("expected git worktree remove --force")
		}
		if !foundD {
			t.Error("expected git branch -D")
		}
	})

	t.Run("refuses Force and Merged set together", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		in := RemoveInput{Branch: "feat", OutputDir: outputDir, Force: true, Merged: true}
		err := Remove(context.Background(), deps, in)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("got error %v, want error containing %q", err, "mutually exclusive")
		}
		if runner.Idx != 1 {
			t.Errorf("runner called %d times, want 1 (list only)", runner.Idx)
		}
	})

	t.Run("merged remove skips the ancestor check but still passes plain worktree remove and -D", func(t *testing.T) {
		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},             // git worktree list
			{},                              // git status --porcelain (clean)
			{Output: []byte("origin/feat")}, // git rev-parse @{upstream}
			{Output: []byte("0\t0")},        // git rev-list --left-right --count
			{},                              // git worktree remove
			{},                              // git branch -D
		}}}
		deps := deps.Deps{Runner: rr, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir, Merged: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var foundPlainRemove, foundD bool
		for _, call := range rr.Calls {
			if len(call) == 4 && call[1] == "worktree" && call[2] == "remove" {
				foundPlainRemove = true
			}
			if len(call) >= 4 && call[1] == "branch" && call[2] == "-D" {
				foundD = true
			}
		}
		if !foundPlainRemove {
			t.Error("expected plain git worktree remove, not --force")
		}
		if !foundD {
			t.Error("expected git branch -D")
		}
	})

	t.Run("merged remove refuses a dirty worktree", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},                // git worktree list
			{Output: []byte(" M some/file\n")}, // git status --porcelain (dirty)
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir, Merged: true})
		if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
			t.Errorf("got error %v, want error containing %q", err, "uncommitted changes")
		}
		if runner.Idx != 2 {
			t.Errorf("runner called %d times, want 2 (list + status only)", runner.Idx)
		}
	})

	t.Run("merged remove refuses unpushed commits", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{},                              // git status --porcelain (clean)
			{Output: []byte("origin/feat")}, // git rev-parse @{upstream}
			{Output: []byte("2\t0")},        // git rev-list --left-right --count: 2 ahead
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir, Merged: true})
		if err == nil || !strings.Contains(err.Error(), "2 unpushed commit") {
			t.Errorf("got error %v, want error containing %q", err, "2 unpushed commit")
		}
		if runner.Idx != 4 {
			t.Errorf("runner called %d times, want 4 (no mutation)", runner.Idx)
		}
	})

	t.Run("merged remove proceeds when the branch has no upstream to compare against", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{}, // git status --porcelain (clean)
			{Err: errors.New("no upstream configured")}, // git rev-parse @{upstream}
			{}, // git worktree remove
			{}, // git branch -D
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{Branch: "feat", OutputDir: outputDir, Merged: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("refuses to remove worktree user is currently in", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		err := Remove(context.Background(), deps, RemoveInput{
			Branch:    "feat",
			OutputDir: outputDir,
			Cwd:       featPath,
		})
		if err == nil || !strings.Contains(err.Error(), "currently in") {
			t.Errorf("got error %v, want error containing %q", err, "currently in")
		}
		if runner.Idx != 1 {
			t.Errorf("runner called %d times after guard, want 1 (list only)", runner.Idx)
		}
	})
}

func TestRemoveWorktreeAndArtifactSurfacesPostErr(t *testing.T) {
	mainPath := makeMainWorktree(t)
	featPath := mainPath + "-feat"
	writeTOML(t, mainPath, "[[hooks.post_remove]]\ncommand = \"fail\"\n")

	runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
		{}, // git worktree remove
		{}, // git branch -d
	}}
	var warnings strings.Builder
	d := deps.Deps{
		Runner:     runner,
		Syncer:     &treepadtest.FakeSyncer{},
		Opener:     &treepadtest.FakeOpener{},
		HookRunner: &treepadtest.FakeHookRunner{Err: errors.New("hook exploded")},
		Log:        treepadtest.NewPrinter(&warnings),
	}
	target := worktree.Worktree{Path: featPath, Branch: "feat"}
	main := worktree.Worktree{Path: mainPath, Branch: "main", IsMain: true}

	postErr, err := RemoveWorktreeAndArtifact(context.Background(), d, target, main, t.TempDir(), RemoveMode{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postErr == nil {
		t.Fatal("PostErr is nil, want the post-hook failure")
	}
	if postErr.Event != hook.PostRemove {
		t.Errorf("PostErr.Event = %q, want %q", postErr.Event, hook.PostRemove)
	}
	if got := strings.Count(warnings.String(), "[WARN]"); got != 1 {
		t.Errorf("logged %d [WARN] lines, want 1:\n%s", got, warnings.String())
	}
}
