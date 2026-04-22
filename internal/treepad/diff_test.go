package treepad

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/ui"
)

// makeMainWorktree creates a temp dir with a .git subdirectory so that
// worktree.isMainWorktree recognises it as the primary worktree.
func makeMainWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiff(t *testing.T) {
	t.Run("requires branch", func(t *testing.T) {
		d := Deps{
			Runner: fakeRunner{},
			Syncer: &fakeSyncer{},
			Out:    &bytes.Buffer{},
			Log:    ui.New(&bytes.Buffer{}),
			In:     strings.NewReader(""),
		}
		err := Diff(context.Background(), d, DiffInput{})
		if err == nil || !strings.Contains(err.Error(), "branch name is required") {
			t.Fatalf("want branch required error, got %v", err)
		}
	})

	t.Run("unknown branch", func(t *testing.T) {
		d := Deps{
			Runner: fakeRunner{output: twoWorktreePorcelain},
			Syncer: &fakeSyncer{},
			Out:    &bytes.Buffer{},
			Log:    ui.New(&bytes.Buffer{}),
			In:     strings.NewReader(""),
		}
		err := Diff(context.Background(), d, DiffInput{Branch: "nonexistent"})
		if err == nil || !strings.Contains(err.Error(), `no worktree found for branch "nonexistent"`) {
			t.Fatalf("want not-found error, got %v", err)
		}
	})

	t.Run("prunable branch", func(t *testing.T) {
		porcelain := twoWorktreePorcelainWithPrunable(t.TempDir(), t.TempDir())
		d := Deps{
			Runner: fakeRunner{output: porcelain},
			Syncer: &fakeSyncer{},
			Out:    &bytes.Buffer{},
			Log:    ui.New(&bytes.Buffer{}),
			In:     strings.NewReader(""),
		}
		err := Diff(context.Background(), d, DiffInput{Branch: "stale-branch"})
		if err == nil || !strings.Contains(err.Error(), "prunable") {
			t.Fatalf("want prunable error, got %v", err)
		}
	})

	streamTests := []struct {
		name      string
		base      string
		extraArgs []string
		wantArgs  []string
	}{
		{
			name:     "default base is origin/main",
			wantArgs: []string{"diff", "origin/main...HEAD"},
		},
		{
			name:     "custom base",
			base:     "dev",
			wantArgs: []string{"diff", "dev...HEAD"},
		},
		{
			name:      "extra args forwarded",
			extraArgs: []string{"--stat"},
			wantArgs:  []string{"diff", "origin/main...HEAD", "--stat"},
		},
	}
	for _, tt := range streamTests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pt := &fakePassthroughRunner{}
			d := Deps{
				Runner: fakeRunner{output: worktreePorcelainWithPath("feat", dir)},
				Syncer: &fakeSyncer{},
				Out:    &bytes.Buffer{},
				Log:    ui.New(&bytes.Buffer{}),
				In:     strings.NewReader(""),
			}

			err := Diff(context.Background(), d, DiffInput{Branch: "feat", Base: tt.base, ExtraArgs: tt.extraArgs, Runner: pt})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pt.calls) == 0 {
				t.Fatal("expected passthrough runner call, got none")
			}
			call := pt.calls[0]
			if call.dir != dir {
				t.Errorf("dir = %q, want %q", call.dir, dir)
			}
			if call.name != "git" {
				t.Errorf("name = %q, want git", call.name)
			}
			if !equalStringSlice(call.args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", call.args, tt.wantArgs)
			}
		})
	}

	t.Run("base from config overrides default", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		contents := []byte("[diff]\nbase = \"master\"\n")
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), contents, 0o644); err != nil {
			t.Fatal(err)
		}
		featPath := t.TempDir()
		pt := &fakePassthroughRunner{}
		d := Deps{
			Runner: fakeRunner{output: twoWorktreePorcelainWithMain(mainPath, featPath)},
			Syncer: &fakeSyncer{},
			Out:    &bytes.Buffer{},
			Log:    ui.New(&bytes.Buffer{}),
			In:     strings.NewReader(""),
		}

		if err := Diff(context.Background(), d, DiffInput{Branch: "feat", Runner: pt}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pt.calls) == 0 {
			t.Fatal("expected passthrough runner call, got none")
		}
		wantArgs := []string{"diff", "master...HEAD"}
		if !equalStringSlice(pt.calls[0].args, wantArgs) {
			t.Errorf("args = %v, want %v", pt.calls[0].args, wantArgs)
		}
	})

	t.Run("file output writes plain patch", func(t *testing.T) {
		dir := t.TempDir()
		outFile := filepath.Join(dir, "out.patch")
		patchContent := []byte("diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n")

		rec := &recordingRunner{inner: &seqRunner{responses: []runResponse{
			{output: worktreePorcelainWithPath("feat", dir)},
			{output: patchContent},
		}}}
		var logBuf bytes.Buffer
		d := Deps{Runner: rec, Syncer: &fakeSyncer{}, Out: &bytes.Buffer{}, Log: ui.New(&logBuf), In: strings.NewReader("")}

		if err := Diff(context.Background(), d, DiffInput{Branch: "feat", Base: "main", OutputFile: outFile}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		if !bytes.Equal(got, patchContent) {
			t.Errorf("file contents = %q, want %q", got, patchContent)
		}

		if len(rec.calls) < 2 {
			t.Fatalf("expected at least 2 runner calls, got %d", len(rec.calls))
		}
		joined := strings.Join(rec.calls[1], " ")
		if !strings.Contains(joined, "--no-color") {
			t.Errorf("expected --no-color in git call: %v", rec.calls[1])
		}
		if !strings.Contains(joined, "main...HEAD") {
			t.Errorf("expected main...HEAD in git call: %v", rec.calls[1])
		}
		if !strings.Contains(logBuf.String(), "wrote diff to") {
			t.Errorf("expected success log, got: %q", logBuf.String())
		}
	})
}
