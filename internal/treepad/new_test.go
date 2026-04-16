package treepad

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	outputDir := t.TempDir()
	porcelain := mainWorktreePorcelain(mainPath)

	t.Run("creates worktree and syncs config", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil}, // git worktree add
		}}
		syn := &fakeSyncer{}
		opener := &fakeOpener{}
		deps := testDeps(runner, syn, opener)

		err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.calls))
		}
		if syn.calls[0].SourceDir != mainPath {
			t.Errorf("SourceDir = %q, want %q", syn.calls[0].SourceDir, mainPath)
		}
		if len(opener.paths) != 0 {
			t.Errorf("opener called %d times, want 0", len(opener.paths))
		}
	})

	t.Run("opens artifact when Open is true", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		opener := &fakeOpener{}
		deps := testDeps(runner, &fakeSyncer{}, opener)

		err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			Open:      true,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(opener.paths) != 1 {
			t.Fatalf("opener called %d times, want 1", len(opener.paths))
		}
		// Default artifact template produces a .code-workspace file.
		if !strings.HasSuffix(opener.paths[0], ".code-workspace") {
			t.Errorf("opened path %q, expected a .code-workspace file", opener.paths[0])
		}
	})

	t.Run("emits cd directive by default", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "__TREEPAD_CD__\t") {
			t.Errorf("output missing cd directive; got:\n%s", buf.String())
		}
	})

	t.Run("suppresses cd directive when Current is true", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			Current:   true,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(buf.String(), "__TREEPAD_CD__") {
			t.Errorf("cd directive should be absent when Current=true; got:\n%s", buf.String())
		}
	})

	t.Run("fires PreNew and PostNew hooks", func(t *testing.T) {
		toml := "[hooks]\npre_new = [\"marker-pre\"]\npost_new = [\"marker-post\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		hr := &fakeHookRunner{}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

		if err := New(context.Background(), deps, NewInput{Branch: "feature/auth", Base: "main", OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.calls) != 2 {
			t.Fatalf("hook runner called %d times, want 2", len(hr.calls))
		}
		if got := hr.calls[0].data.HookType; got != "pre_new" {
			t.Errorf("calls[0].HookType = %q, want pre_new", got)
		}
		if got := hr.calls[1].data.HookType; got != "post_new" {
			t.Errorf("calls[1].HookType = %q, want post_new", got)
		}
		if got := hr.calls[0].data.Branch; got != "feature/auth" {
			t.Errorf("hook data Branch = %q, want feature/auth", got)
		}
	})

	t.Run("PreNew failure aborts before git worktree add", func(t *testing.T) {
		toml := "[hooks]\npre_new = [\"fail\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		rr := &recordingRunner{inner: &seqRunner{responses: []runResponse{
			{output: porcelain},
		}}}
		hr := &fakeHookRunner{err: errors.New("hook aborted")}
		deps := testDeps(rr, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

		err := New(context.Background(), deps, NewInput{Branch: "feature/auth", Base: "main", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "hook aborted") {
			t.Errorf("got error %v, want error containing 'hook aborted'", err)
		}
		for _, call := range rr.calls {
			if len(call) >= 3 && call[1] == "worktree" && call[2] == "add" {
				t.Error("git worktree add should not be called when pre_new hook fails")
			}
		}
	})

	t.Run("PostNew failure logs warning but does not abort", func(t *testing.T) {
		toml := "[hooks]\npost_new = [\"fail\"]\n"
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { os.Remove(filepath.Join(mainPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		hr := &fakeHookRunner{err: errors.New("post hook failed")}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.HookRunner = hr

		if err := New(context.Background(), deps, NewInput{Branch: "feature/auth", Base: "main", OutputDir: outputDir}); err != nil {
			t.Errorf("PostNew hook failure should not abort operation, got error: %v", err)
		}
	})

	errorTests := []struct {
		name    string
		runner  *seqRunner
		syncer  *fakeSyncer
		wantErr string
	}{
		{
			name: "git worktree list fails",
			runner: &seqRunner{responses: []runResponse{
				{err: errors.New("git not found")},
			}},
			syncer:  &fakeSyncer{},
			wantErr: "git not found",
		},
		{
			name: "git worktree add fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{err: errors.New("branch already exists")},
			}},
			syncer:  &fakeSyncer{},
			wantErr: "branch already exists",
		},
		{
			name: "sync fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{output: nil},
			}},
			syncer:  &fakeSyncer{err: errors.New("sync failed")},
			wantErr: "sync failed",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{Runner: tt.runner, Syncer: tt.syncer, Opener: &fakeOpener{}, Out: io.Discard, In: strings.NewReader("")}
			err := New(context.Background(), deps, NewInput{
				Branch:    "feature/auth",
				Base:      "main",
				OutputDir: outputDir,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
