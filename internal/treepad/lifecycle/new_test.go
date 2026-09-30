package lifecycle

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

func TestNew(t *testing.T) {
	mainPath := makeMainWorktree(t)
	outputDir := t.TempDir()
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)

	t.Run("creates worktree and syncs config", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git -C <path> checkout HEAD -- .
		}}
		syn := &treepadtest.FakeSyncer{}
		opener := &treepadtest.FakeOpener{}
		deps := deps.Deps{Runner: runner, Syncer: syn, Opener: opener}

		cdPath, err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.Calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.Calls))
		}
		if syn.Calls[0].SourceDir != mainPath {
			t.Errorf("SourceDir = %q, want %q", syn.Calls[0].SourceDir, mainPath)
		}
		if len(opener.Paths) != 0 {
			t.Errorf("opener called %d times, want 0", len(opener.Paths))
		}
		if cdPath != mainPath {
			t.Errorf("cdPath = %q, want %q", cdPath, mainPath)
		}
	})

	t.Run("links sync.link entries into the new worktree", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		if err := os.WriteFile(filepath.Join(mainPath, ".env.shared"), []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTOML(t, mainPath, "[sync]\nlink = [\".env.shared\", \"tracked\", \"absent\"]\n")
		if err := os.WriteFile(filepath.Join(mainPath, "tracked"), []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		wtPath := filepath.Join(filepath.Dir(mainPath), filepath.Base(mainPath)+"-feature-auth")
		if err := os.MkdirAll(wtPath, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(wtPath) })

		runner := &treepadtest.DispatchRunner{
			Classify: func(_ string, args []string) string {
				if slices.Contains(args, "ls-files") {
					return "ls-files:" + args[len(args)-1]
				}
				return ""
			},
			Routes: map[string][]treepadtest.RunResponse{
				"ls-files:.env.shared": {{Err: errors.New("not tracked")}},
				"ls-files:tracked":     {{Output: []byte("tracked")}},
			},
			Fallback: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: treepadtest.MainWorktreePorcelain(mainPath)},
				{}, {},
			}},
		}
		d := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{},
			Log: treepadtest.NewPrinter(io.Discard)}

		if _, err := New(context.Background(), d, NewInput{Branch: "feature/auth", Base: "main", OutputDir: t.TempDir()}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.Readlink(filepath.Join(wtPath, ".env.shared"))
		if err != nil || got != filepath.Join(mainPath, ".env.shared") {
			t.Errorf("Readlink = %q, %v; want link to main", got, err)
		}
		for _, name := range []string{"tracked", "absent"} {
			if _, err := os.Lstat(filepath.Join(wtPath, name)); err == nil {
				t.Errorf("%s should not be linked", name)
			}
		}
	})

	t.Run("opens artifact when Open is true", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}
		opener := &treepadtest.FakeOpener{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: opener}

		cdPath, err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			Open:      true,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(opener.Paths) != 1 {
			t.Fatalf("opener called %d times, want 1", len(opener.Paths))
		}
		// Default artifact template produces a .code-workspace file.
		if !strings.HasSuffix(opener.Paths[0], ".code-workspace") {
			t.Errorf("opened path %q, expected a .code-workspace file", opener.Paths[0])
		}
		if cdPath != mainPath {
			t.Errorf("cdPath = %q, want %q", cdPath, mainPath)
		}
	})

	t.Run("emits cd directive by default", func(t *testing.T) {
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}
		var buf strings.Builder
		deps := deps.Deps{
			Runner: runner, Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{}, Out: &buf, In: strings.NewReader(""),
		}

		_, err := New(context.Background(), deps, NewInput{
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
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}
		var buf strings.Builder
		deps := deps.Deps{
			Runner: runner, Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{}, Out: &buf, In: strings.NewReader(""),
		}

		_, err := New(context.Background(), deps, NewInput{
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
		toml := "[[hooks.pre_new]]\ncommand = \"marker-pre\"\n\n[[hooks.post_new]]\ncommand = \"marker-post\"\n"
		writeTOML(t, mainPath, toml)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}
		hr := &treepadtest.FakeHookRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		_, err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.Calls) != 2 {
			t.Fatalf("hook runner called %d times, want 2", len(hr.Calls))
		}
		if got := hr.Calls[0].Data.HookType; got != "pre_new" {
			t.Errorf("calls[0].HookType = %q, want pre_new", got)
		}
		if got := hr.Calls[1].Data.HookType; got != "post_new" {
			t.Errorf("calls[1].HookType = %q, want post_new", got)
		}
		if got := hr.Calls[0].Data.Branch; got != "feature/auth" {
			t.Errorf("hook data Branch = %q, want feature/auth", got)
		}
	})

	t.Run("PreNew failure aborts before git worktree add", func(t *testing.T) {
		toml := "[[hooks.pre_new]]\ncommand = \"fail\"\n"
		writeTOML(t, mainPath, toml)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
		}}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("hook aborted")}
		deps := deps.Deps{Runner: rr.Inner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		_, err := New(context.Background(), deps, NewInput{Branch: "feature/auth", Base: "main", OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "hook aborted") {
			t.Errorf("got error %v, want error containing 'hook aborted'", err)
		}
		for _, call := range rr.Calls {
			if len(call) >= 3 && call[1] == "worktree" && call[2] == "add" {
				t.Error("git worktree add should not be called when pre_new hook fails")
			}
		}
	})

	t.Run("PostNew failure logs warning but does not abort", func(t *testing.T) {
		toml := "[[hooks.post_new]]\ncommand = \"fail\"\n"
		writeTOML(t, mainPath, toml)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("post hook failed")}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr

		_, err := New(context.Background(), deps, NewInput{
			Branch:    "feature/auth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Errorf("PostNew hook failure should not abort operation, got error: %v", err)
		}
	})

	t.Run("uses --no-checkout then checkout concurrently with sync", func(t *testing.T) {
		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: nil}, // git worktree add --no-checkout
			{Output: nil}, // git checkout
		}}}
		deps := deps.Deps{Runner: rr, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}

		_, err := New(context.Background(), deps, NewInput{Branch: "feature/auth", Base: "main", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var sawNoCheckout, sawCheckout bool
		for _, call := range rr.Calls {
			if len(call) >= 4 && call[1] == "worktree" && call[2] == "add" && call[3] == "--no-checkout" {
				sawNoCheckout = true
			}
			if len(call) >= 3 && call[1] == "-C" && slices.Contains(call, "checkout") {
				sawCheckout = true
			}
		}
		if !sawNoCheckout {
			t.Error("git worktree add should be called with --no-checkout")
		}
		if !sawCheckout {
			t.Error("git -C <path> checkout should be called after worktree add")
		}
	})

	errorTests := []struct {
		name    string
		runner  *treepadtest.SeqRunner
		syncer  *treepadtest.FakeSyncer
		wantErr string
	}{
		{
			name: "git worktree list fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Err: errors.New("git not found")},
			}},
			syncer:  &treepadtest.FakeSyncer{},
			wantErr: "git not found",
		},
		{
			name: "git worktree add fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{Err: errors.New("branch already exists")},
			}},
			syncer:  &treepadtest.FakeSyncer{},
			wantErr: "branch already exists",
		},
		{
			name: "sync fails",
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{Output: nil}, // git worktree add --no-checkout
				{Output: nil}, // git checkout (runs concurrently with sync)
			}},
			syncer:  &treepadtest.FakeSyncer{Err: errors.New("sync failed")},
			wantErr: "sync failed",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			deps := deps.Deps{
				Runner: tt.runner, Syncer: tt.syncer,
				Opener: &treepadtest.FakeOpener{}, Out: io.Discard, In: strings.NewReader(""),
			}
			_, err := New(context.Background(), deps, NewInput{
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
