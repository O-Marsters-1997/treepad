// Package treepadtest provides shared test helpers for packages that depend
// on treepad sub-package dependencies. Mirrors the pattern established by
// worktreetest.
package treepadtest

import (
	"context"
	"errors"
	"fmt"
	"io"

	"treepad/internal/artifact"
	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
	"treepad/internal/ui"
	"treepad/internal/worktree/worktreetest"
)

// StaticRunner is re-exported from worktreetest for callers that only need a
// single canned response.
type StaticRunner = worktreetest.StaticRunner

// RunResponse is one entry in a SeqRunner's response queue.
type RunResponse struct {
	Output []byte
	Err    error
}

// SeqRunner returns responses in order across successive Run calls.
type SeqRunner struct {
	Responses []RunResponse
	Idx       int
}

func (s *SeqRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if s.Idx >= len(s.Responses) {
		return nil, fmt.Errorf("unexpected runner call %d", s.Idx)
	}
	r := s.Responses[s.Idx]
	s.Idx++
	return r.Output, r.Err
}

// RecordingRunner records every Run call and delegates to an inner SeqRunner.
type RecordingRunner struct {
	Inner *SeqRunner
	Calls [][]string // each entry is [name, args...]
}

func (r *RecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	entry := append([]string{name}, args...)
	r.Calls = append(r.Calls, entry)
	return r.Inner.Run(ctx, name, args...)
}

// FakeSyncer records Sync calls and returns a canned error.
type FakeSyncer struct {
	Calls []internalsync.Config
	Err   error
}

func (f *FakeSyncer) Sync(_ []string, cfg internalsync.Config) error {
	f.Calls = append(f.Calls, cfg)
	return f.Err
}

// FakeOpener records Open calls and returns a canned error.
type FakeOpener struct {
	Paths []string
	Err   error
}

func (f *FakeOpener) Open(_ context.Context, _ artifact.OpenSpec, data artifact.OpenData) error {
	f.Paths = append(f.Paths, data.ArtifactPath)
	return f.Err
}

// FakeHookCall records a single invocation of FakeHookRunner.Run.
type FakeHookCall struct {
	Hooks []hook.HookEntry
	Data  hook.Data
}

// FakeHookRunner records hook calls and returns a canned error.
type FakeHookRunner struct {
	Calls []FakeHookCall
	Err   error
}

func (f *FakeHookRunner) Run(_ context.Context, hooks []hook.HookEntry, data hook.Data) error {
	f.Calls = append(f.Calls, FakeHookCall{Hooks: hooks, Data: data})
	return f.Err
}

// ErrExitNonZero is a sentinel for tests that expect a non-zero exit.
var ErrExitNonZero = errors.New("exit status 1")

// NewPrinter returns a Printer backed by w, for asserting on log output.
func NewPrinter(w io.Writer) *ui.Printer {
	return ui.New(w)
}

// fakePassthroughRunner records calls and returns a canned exit code.
type FakePassthroughRunner struct {
	Calls    []ptCall
	ExitCode int
	Err      error
}

type ptCall struct {
	Dir  string
	Name string
	Args []string
}

func (f *FakePassthroughRunner) Run(_ context.Context, dir, name string, args ...string) (int, error) {
	f.Calls = append(f.Calls, ptCall{Dir: dir, Name: name, Args: args})
	return f.ExitCode, f.Err
}
