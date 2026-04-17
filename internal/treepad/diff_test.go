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

func testDiffDeps(runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}, logBuf *bytes.Buffer) Deps {
	return Deps{
		Runner: runner,
		Out:    &bytes.Buffer{},
		Log:    ui.New(logBuf),
		In:     strings.NewReader(""),
	}
}

func TestDiff_emptyBranch(t *testing.T) {
	var logBuf bytes.Buffer
	d := testDiffDeps(fakeRunner{}, &logBuf)
	err := Diff(context.Background(), d, DiffInput{})
	if err == nil || !strings.Contains(err.Error(), "branch name is required") {
		t.Fatalf("want branch required error, got %v", err)
	}
}

func TestDiff_unknownBranch(t *testing.T) {
	var logBuf bytes.Buffer
	d := testDiffDeps(fakeRunner{output: twoWorktreePorcelain}, &logBuf)
	err := Diff(context.Background(), d, DiffInput{Branch: "nonexistent"})
	if err == nil || !strings.Contains(err.Error(), `no worktree found for branch "nonexistent"`) {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestDiff_prunableBranch(t *testing.T) {
	main := t.TempDir()
	prune := t.TempDir()
	porcelain := twoWorktreePorcelainWithPrunable(main, prune)
	var logBuf bytes.Buffer
	d := testDiffDeps(fakeRunner{output: porcelain}, &logBuf)
	err := Diff(context.Background(), d, DiffInput{Branch: "stale-branch"})
	if err == nil || !strings.Contains(err.Error(), "prunable") {
		t.Fatalf("want prunable error, got %v", err)
	}
}

func TestDiff_stream(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		extraArgs []string
		wantArgs  []string
	}{
		{
			name:     "default base is main",
			wantArgs: []string{"diff", "main...HEAD"},
		},
		{
			name:     "custom base",
			base:     "dev",
			wantArgs: []string{"diff", "dev...HEAD"},
		},
		{
			name:      "extra args forwarded",
			extraArgs: []string{"--stat"},
			wantArgs:  []string{"diff", "main...HEAD", "--stat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			porcelain := worktreePorcelainWithPath("feat", dir)
			pt := &fakePassthroughRunner{}
			var logBuf bytes.Buffer
			d := testDiffDeps(fakeRunner{output: porcelain}, &logBuf)

			err := Diff(context.Background(), d, DiffInput{
				Branch:    "feat",
				Base:      tt.base,
				ExtraArgs: tt.extraArgs,
				Runner:    pt,
			})
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
}

func TestDiff_fileOutput(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.patch")
	patchContent := []byte("diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n")

	porcelain := worktreePorcelainWithPath("feat", dir)
	rec := &recordingRunner{inner: &seqRunner{responses: []runResponse{
		{output: porcelain},
		{output: patchContent},
	}}}

	var logBuf bytes.Buffer
	d := testDiffDeps(rec, &logBuf)

	err := Diff(context.Background(), d, DiffInput{
		Branch:     "feat",
		Base:       "main",
		OutputFile: outFile,
	})
	if err != nil {
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
	gitCall := rec.calls[1]
	joined := strings.Join(gitCall, " ")
	if !strings.Contains(joined, "--no-color") {
		t.Errorf("expected --no-color in git call: %v", gitCall)
	}
	if !strings.Contains(joined, "main...HEAD") {
		t.Errorf("expected main...HEAD in git call: %v", gitCall)
	}

	if !strings.Contains(logBuf.String(), "wrote diff to") {
		t.Errorf("expected success log, got: %q", logBuf.String())
	}
}
