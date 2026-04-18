package treepad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"treepad/internal/artifact"
	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
	"treepad/internal/ui"
	"treepad/internal/worktree"
)

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

type fakeOpener struct {
	paths []string
	err   error
}

func (f *fakeOpener) Open(_ context.Context, _ artifact.OpenSpec, data artifact.OpenData) error {
	f.paths = append(f.paths, data.ArtifactPath)
	return f.err
}

// recordingRunner records every Run call and delegates to an inner seqRunner.
type recordingRunner struct {
	inner *seqRunner
	calls [][]string // each entry is [name, args...]
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	entry := append([]string{name}, args...)
	r.calls = append(r.calls, entry)
	return r.inner.Run(ctx, name, args...)
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

func twoWorktreePorcelainWithMain(mainPath, featPath string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\nworktree %s\nHEAD def456\nbranch refs/heads/feat\n\n",
		mainPath, featPath,
	)
}

func twoWorktreePorcelainWithPrunable(mainPath, prunablePath string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\nworktree %s\nHEAD def456\nbranch refs/heads/stale-branch\nprunable gitdir file points to non-existent location\n\n",
		mainPath, prunablePath,
	)
}

func threeWorktreePorcelainWithMain(mainPath, feat1Path, feat2Path string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\nworktree %s\nHEAD def456\nbranch refs/heads/feat\n\nworktree %s\nHEAD ghi789\nbranch refs/heads/other\n\n",
		mainPath, feat1Path, feat2Path,
	)
}

type fakeHookCall struct {
	hooks []hook.HookEntry
	data  hook.Data
}

type fakeHookRunner struct {
	calls []fakeHookCall
	err   error
}

func (f *fakeHookRunner) Run(_ context.Context, hooks []hook.HookEntry, data hook.Data) error {
	f.calls = append(f.calls, fakeHookCall{hooks: hooks, data: data})
	return f.err
}

// testDeps builds a Deps value suitable for tests: discards output and reads
// from an empty stdin unless the caller substitutes those fields.
// HookRunner defaults to a no-op fakeHookRunner; override deps.HookRunner for tests
// that assert hook behavior.
func testDeps(runner worktree.CommandRunner, syncer internalsync.Syncer, opener artifact.Opener) Deps {
	return Deps{
		Runner:     runner,
		Syncer:     syncer,
		Opener:     opener,
		HookRunner: &fakeHookRunner{},
		Out:        io.Discard,
		In:         strings.NewReader(""),
	}
}

// newTestPrinter returns a Printer backed by w, for asserting on log output.
func newTestPrinter(w io.Writer) *ui.Printer {
	return ui.New(w)
}

// errExitNonZero simulates a runner returning a non-zero exit code.
var errExitNonZero = errors.New("exit status 1")
