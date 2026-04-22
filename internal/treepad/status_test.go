package treepad

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/slug"
)

var update = flag.Bool("update", false, "update golden files")

func TestFormatStatusRows(t *testing.T) {
	rows := []StatusRow{
		{Branch: "main", Path: "/repo/main", IsMain: true, HasUpstream: true, Ahead: 0, Behind: 1},
		{Branch: "feat-a", Path: "/repo/feat-a"},
		{Branch: "feat-b", Path: "/repo/feat-b", Dirty: true, HasUpstream: true, Ahead: 1, Behind: 2},
		{Branch: "stale", Path: "/repo/stale", Prunable: true, PrunableReason: "no gitdir"},
	}

	lines := formatStatusRows(rows)
	if lines == nil {
		t.Fatal("expected non-nil lines for non-empty rows")
	}
	got := strings.Join(lines, "\n")

	const golden = "testdata/status_table_basic.golden"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	wantBytes, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden file missing — run with -update to create: %v", err)
	}
	if got != string(wantBytes) {
		t.Errorf("formatStatusRows output differs from golden\ngot:\n%s\nwant:\n%s", got, string(wantBytes))
	}
}

func TestFormatStatusRows_Empty(t *testing.T) {
	if lines := formatStatusRows(nil); lines != nil {
		t.Errorf("expected nil for empty rows, got %v", lines)
	}
}

func TestStatus(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	porcelain := twoWorktreePorcelainWithMain(mainPath, featPath)

	commitOutput := func(sha, subject string) []byte {
		return fmt.Appendf(nil, "%s\x00%s\x002024-06-01T12:00:00Z\n", sha, subject)
	}

	t.Run("renders table for two worktrees", func(t *testing.T) {
		// main: clean, upstream (0 ahead, 1 behind)
		// feat: dirty, no upstream
		// artifact file for feat exists; main's does not
		featArtifact := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(featArtifact, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup artifact: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},                        // git worktree list
			{output: []byte("")},                       // dirty: main (clean)
			{output: []byte("origin/main\n")},          // rev-parse @{upstream}: main
			{output: []byte("0\t1\n")},                 // rev-list: main (0↑ 1↓)
			{output: commitOutput("abc1234", "init")},  // git log: main
			{output: []byte("M file.go\n")},            // dirty: feat
			{err: errors.New("no upstream")},           // rev-parse @{upstream}: feat (none)
			{output: commitOutput("def5678", "add x")}, // git log: feat
		}}

		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}
		err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 8 {
			t.Errorf("runner called %d times, want 8", runner.idx)
		}
		out := buf.String()
		for _, want := range []string{"BRANCH", "main", "feat", "clean", "dirty", "↑0 ↓1", "—", "abc1234", "def5678"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("json flag emits JSON array", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("")},
			{err: errors.New("no upstream")},
			{output: commitOutput("abc1234", "init")},
			{output: []byte("")},
			{err: errors.New("no upstream")},
			{output: commitOutput("def5678", "add x")},
		}}
		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}
		err := Status(context.Background(), deps, StatusInput{JSON: true, OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, "[") {
			t.Errorf("expected JSON array, got: %s", out)
		}
		for _, want := range []string{"main", "feat", `"dirty"`, `"branch"`} {
			if !strings.Contains(out, want) {
				t.Errorf("JSON output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("prunable worktree renders without git calls", func(t *testing.T) {
		prunablePath := mainPath + "-stale"
		porcelainWithPrunable := twoWorktreePorcelainWithPrunable(mainPath, prunablePath)

		runner := &seqRunner{responses: []runResponse{
			{output: porcelainWithPrunable},           // git worktree list
			{output: []byte("")},                      // dirty: main (clean)
			{output: []byte("origin/main\n")},         // rev-parse @{upstream}: main
			{output: []byte("0\t0\n")},                // rev-list: main
			{output: commitOutput("abc1234", "init")}, // git log: main
		}}

		var buf strings.Builder
		deps := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}
		err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only 5 runner calls: list + 4 for main. No calls for the prunable worktree.
		if runner.idx != 5 {
			t.Errorf("runner called %d times, want 5 (no git calls for prunable)", runner.idx)
		}
		out := buf.String()
		for _, want := range []string{"stale-branch", "prunable", "gitdir file points to non-existent location", "tp prune"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
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
			name: "dirty probe fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{err: errors.New("status failed")},
			}},
			wantErr: "status failed",
		},
		{
			name: "last commit probe fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{output: []byte("")},             // dirty: clean
				{err: errors.New("no upstream")}, // no upstream
				{err: errors.New("log failed")},  // log fails
			}},
			wantErr: "log failed",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(tt.runner, &fakeSyncer{}, &fakeOpener{})
			err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
