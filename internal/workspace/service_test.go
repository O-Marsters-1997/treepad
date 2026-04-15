package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/codeworkspace"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

// --- fakes ---

type fakeRunner struct {
	output []byte
	err    error
}

func (f fakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
}

type fakeSyncer struct {
	calls []internalsync.Config
	err   error
}

func (f *fakeSyncer) Sync(_ []string, cfg internalsync.Config) error {
	f.calls = append(f.calls, cfg)
	return f.err
}

// seqRunner returns responses in order across successive Run calls.
type seqRunner struct {
	responses []runResponse
	idx       int
}

type runResponse struct {
	output []byte
	err    error
}

func (s *seqRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if s.idx >= len(s.responses) {
		return nil, fmt.Errorf("unexpected runner call %d", s.idx)
	}
	r := s.responses[s.idx]
	s.idx++
	return r.output, r.err
}

// recRunner records every call's args in order alongside its canned response.
type recRunner struct {
	responses []runResponse
	calls     [][]string
	idx       int
}

func (r *recRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.idx >= len(r.responses) {
		return nil, fmt.Errorf("unexpected runner call %d", r.idx)
	}
	resp := r.responses[r.idx]
	r.idx++
	return resp.output, resp.err
}

// fakeOpener records calls and returns a configured error.
type fakeOpener struct {
	calls []string
	err   error
}

func (f *fakeOpener) Open(_ context.Context, path string) error {
	f.calls = append(f.calls, path)
	return f.err
}

// twoWorktreePorcelain produces two worktrees; IsMain will be false for both
// in tests since the paths don't exist on disk. Tests use SourcePath to
// bypass the main-worktree lookup.
var twoWorktreePorcelain = []byte(`worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /repo/feat
HEAD def456
branch refs/heads/feat

`)

// mainWorktreePorcelain builds porcelain output where mainPath has a real .git dir.
func mainWorktreePorcelain(mainPath string) []byte {
	return fmt.Appendf(nil, "worktree %s\nHEAD abc123\nbranch refs/heads/main\n\n", mainPath)
}

func newTestService(t *testing.T, runner worktree.CommandRunner, syncer internalsync.Syncer, opener Opener) *Service {
	t.Helper()
	return NewService(runner, syncer, opener, io.Discard)
}

func TestServiceGenerate(t *testing.T) {
	t.Run("syncs non-source worktrees", func(t *testing.T) {
		syn := &fakeSyncer{}
		svc := newTestService(t, fakeRunner{output: twoWorktreePorcelain}, syn, nil)

		err := svc.Generate(context.Background(), GenerateInput{SourcePath: "/repo/main", SyncOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(syn.calls) != 1 {
			t.Fatalf("syncer called %d times, want 1", len(syn.calls))
		}
		if syn.calls[0].TargetDir != "/repo/feat" {
			t.Errorf("TargetDir = %q, want /repo/feat", syn.calls[0].TargetDir)
		}
	})

	errorTests := []struct {
		name    string
		runner  worktree.CommandRunner
		syncer  internalsync.Syncer
		input   GenerateInput
		wantErr string
	}{
		{
			name:    "propagates syncer error",
			runner:  fakeRunner{output: twoWorktreePorcelain},
			syncer:  &fakeSyncer{err: errors.New("sync failed")},
			input:   GenerateInput{SourcePath: "/repo/main", SyncOnly: true},
			wantErr: "sync failed",
		},
		{
			name:    "no worktrees",
			runner:  fakeRunner{output: []byte{}},
			syncer:  &fakeSyncer{},
			input:   GenerateInput{SourcePath: "/repo/main"},
			wantErr: "no git worktrees found",
		},
		{
			name:    "runner error",
			runner:  fakeRunner{err: errors.New("git not found")},
			syncer:  &fakeSyncer{},
			input:   GenerateInput{},
			wantErr: "git not found",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, tt.runner, tt.syncer, nil)
			err := svc.Generate(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestServiceCreate(t *testing.T) {
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
		svc := NewService(runner, syn, opener, io.Discard)

		err := svc.Create(context.Background(), CreateInput{
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
		if len(opener.calls) != 0 {
			t.Errorf("opener called %d times, want 0", len(opener.calls))
		}
	})

	t.Run("opens workspace when Open is true", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: nil},
		}}
		opener := &fakeOpener{}
		svc := NewService(runner, &fakeSyncer{}, opener, io.Discard)

		err := svc.Create(context.Background(), CreateInput{
			Branch:    "feature/auth",
			Base:      "main",
			Open:      true,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(opener.calls) != 1 {
			t.Fatalf("opener called %d times, want 1", len(opener.calls))
		}
		if !strings.HasSuffix(opener.calls[0], ".code-workspace") {
			t.Errorf("opened path %q, expected a .code-workspace file", opener.calls[0])
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
			svc := NewService(tt.runner, tt.syncer, &fakeOpener{}, io.Discard)
			err := svc.Create(context.Background(), CreateInput{
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

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func twoWorktreePorcelainWithMain(mainPath, featPath string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\nworktree %s\nHEAD def456\nbranch refs/heads/feat\n\n",
		mainPath, featPath,
	)
}

func TestServiceRemove(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))
	porcelain := twoWorktreePorcelainWithMain(mainPath, featPath)

	t.Run("removes worktree, workspace file, and branch", func(t *testing.T) {
		wsFile := filepath.Join(outputDir, codeworkspace.Filename(repoSlug, "feat"))
		if err := os.WriteFile(wsFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain}, // git worktree list
			{},                  // git worktree remove
			{},                  // git branch -d
		}}
		svc := NewService(runner, &fakeSyncer{}, &fakeOpener{}, io.Discard)

		err := svc.Remove(context.Background(), RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
			t.Error("workspace file should have been deleted")
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3", runner.idx)
		}
	})

	t.Run("--force uses git worktree remove --force and git branch -D", func(t *testing.T) {
		runner := &recRunner{responses: []runResponse{
			{output: porcelain},
			{}, // git worktree remove --force
			{}, // git branch -D
		}}
		svc := NewService(runner, &fakeSyncer{}, &fakeOpener{}, io.Discard)

		err := svc.Remove(context.Background(), RemoveInput{Branch: "feat", Force: true, OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.calls) != 3 {
			t.Fatalf("runner called %d times, want 3", len(runner.calls))
		}
		wtArgs := runner.calls[1]
		if len(wtArgs) < 2 || wtArgs[1] != "worktree" || !contains(wtArgs, "--force") {
			t.Errorf("expected 'git worktree remove --force ...', got %v", wtArgs)
		}
		branchArgs := runner.calls[2]
		if len(branchArgs) < 2 || !contains(branchArgs, "-D") {
			t.Errorf("expected 'git branch -D ...', got %v", branchArgs)
		}
	})

	t.Run("workspace file missing is not an error", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{},
			{},
		}}
		svc := NewService(runner, &fakeSyncer{}, &fakeOpener{}, io.Discard)

		err := svc.Remove(context.Background(), RemoveInput{Branch: "feat", OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
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
			svc := NewService(tt.runner, &fakeSyncer{}, &fakeOpener{}, io.Discard)
			err := svc.Remove(context.Background(), RemoveInput{Branch: tt.branch, OutputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("refuses to remove worktree user is currently in", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
		}}
		svc := NewService(runner, &fakeSyncer{}, &fakeOpener{}, io.Discard)

		err := svc.Remove(context.Background(), RemoveInput{
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

func TestResolveSourceDir(t *testing.T) {
	withMain := []worktree.Worktree{
		{Path: "/repo/feat", Branch: "feat", IsMain: false},
		{Path: "/repo/main", Branch: "main", IsMain: true},
	}

	tests := []struct {
		name       string
		useCurrent bool
		sourcePath string
		cwd        string
		worktrees  []worktree.Worktree
		want       string
		wantErr    bool
	}{
		{
			name:       "use current flag",
			useCurrent: true,
			cwd:        "/home/user/repo",
			want:       "/home/user/repo",
		},
		{
			// Absolute path: filepath.Abs is a no-op, keeping the test hermetic.
			name:       "explicit path",
			sourcePath: "/tmp/other-repo",
			want:       "/tmp/other-repo",
		},
		{
			name:      "defaults to main worktree",
			worktrees: withMain,
			want:      "/repo/main",
		},
		{
			name:      "error when no main worktree",
			worktrees: []worktree.Worktree{{Path: "/repo/feat", Branch: "feat", IsMain: false}},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveSourceDir(tt.useCurrent, tt.sourcePath, tt.cwd, tt.worktrees)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
