package treepad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"treepad/internal/slug"
)

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

func TestStatusWatch(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	commitOut := fmt.Appendf(nil, "abc1234\x00init\x002024-06-01T12:00:00Z\n")

	tests := []struct {
		name          string
		isTerminal    bool
		pumpTicks     int
		wantErr       string
		wantRenders   int
		wantAltScreen bool
	}{
		{
			name:       "rejects non-TTY",
			isTerminal: false,
			wantErr:    "requires a TTY",
		},
		{
			name:          "renders once then exits on ctx cancel",
			isTerminal:    true,
			pumpTicks:     0,
			wantRenders:   1,
			wantAltScreen: true,
		},
		{
			name:          "renders on each pumped tick",
			isTerminal:    true,
			pumpTicks:     2,
			wantRenders:   3,
			wantAltScreen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var responses []runResponse
			if tt.isTerminal {
				responses = append(responses, runResponse{output: mainWorktreePorcelain(mainPath)})
				for i := 0; i < tt.wantRenders; i++ {
					responses = append(responses,
						runResponse{output: []byte("")},
						runResponse{err: errors.New("no upstream")},
						runResponse{output: commitOut},
					)
				}
			}
			runner := &seqRunner{responses: responses}
			sleepCh := make(chan time.Time) // unbuffered: pump blocks until goroutine receives
			var buf strings.Builder
			deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{},
				withIsTerminal(func(io.Writer) bool { return tt.isTerminal }),
				withSleep(func(time.Duration) <-chan time.Time { return sleepCh }),
			)
			deps.Out = &buf

			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)
			var gotErr error
			go func() {
				defer wg.Done()
				gotErr = StatusWatch(ctx, deps, StatusInput{})
			}()

			for i := 0; i < tt.pumpTicks; i++ {
				sleepCh <- time.Time{} // sync: blocks until goroutine is at select (render complete)
			}
			cancel()
			wg.Wait()

			if tt.wantErr != "" {
				if gotErr == nil || !strings.Contains(gotErr.Error(), tt.wantErr) {
					t.Errorf("got error %v, want containing %q", gotErr, tt.wantErr)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}
			out := buf.String()
			if tt.wantAltScreen {
				if !strings.Contains(out, "\x1b[?1049h") {
					t.Error("output missing alt-screen enter")
				}
				if !strings.Contains(out, "\x1b[?1049l") {
					t.Error("output missing alt-screen exit")
				}
			}
			if got := strings.Count(out, "BRANCH"); got != tt.wantRenders {
				t.Errorf("got %d renders (BRANCH occurrences), want %d", got, tt.wantRenders)
			}
		})
	}

	t.Run("mid-loop error degrades inline and continues", func(t *testing.T) {
		// Tick 1: dirty probe fails → error rendered inline.
		// Tick 2 (after pump): dirty succeeds → table rendered.
		commitOut := fmt.Appendf(nil, "abc1234\x00init\x002024-06-01T12:00:00Z\n")
		runner := &seqRunner{responses: []runResponse{
			{output: mainWorktreePorcelain(mainPath)}, // list
			{err: errors.New("status failed")},        // dirty tick 1 → error
			{output: []byte("")},                      // dirty tick 2
			{err: errors.New("no upstream")},          // rev-parse tick 2
			{output: commitOut},                       // log tick 2
		}}
		sleepCh := make(chan time.Time)
		var buf strings.Builder
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{},
			withIsTerminal(func(io.Writer) bool { return true }),
			withSleep(func(time.Duration) <-chan time.Time { return sleepCh }),
		)
		deps.Out = &buf

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		var gotErr error
		go func() {
			defer wg.Done()
			gotErr = StatusWatch(ctx, deps, StatusInput{})
		}()

		sleepCh <- time.Time{} // pump tick 2
		cancel()
		wg.Wait()

		if gotErr != nil {
			t.Fatalf("unexpected error: %v", gotErr)
		}
		out := buf.String()
		if !strings.Contains(out, "error:") || !strings.Contains(out, "status failed") {
			t.Errorf("expected inline error message, got:\n%s", out)
		}
		if !strings.Contains(out, "BRANCH") {
			t.Errorf("expected table render after error, got:\n%s", out)
		}
	})
}
