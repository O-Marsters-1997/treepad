package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"treepad/internal/editor"
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

type fakeEditor struct {
	opts editor.Options
	err  error
}

func (f *fakeEditor) Name() string { return "fake" }
func (f *fakeEditor) Configure(_ []worktree.Worktree, opts editor.Options) error {
	f.opts = opts
	return f.err
}

type fakeSyncer struct {
	calls []internalsync.Config
	err   error
}

func (f *fakeSyncer) Sync(_ []string, cfg internalsync.Config) error {
	f.calls = append(f.calls, cfg)
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

// --- orchestrator tests ---

func TestOrchestrator_editorConfiguredWithSourceDir(t *testing.T) {
	ed := &fakeEditor{}
	o := NewOrchestrator(fakeRunner{output: twoWorktreePorcelain}, ed, &fakeSyncer{})

	err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ed.opts.SourceDir != "/repo/main" {
		t.Errorf("SourceDir = %q, want /repo/main", ed.opts.SourceDir)
	}
}

func TestOrchestrator_syncerCalledForNonSourceWorktrees(t *testing.T) {
	syn := &fakeSyncer{}
	o := NewOrchestrator(fakeRunner{output: twoWorktreePorcelain}, &fakeEditor{}, syn)

	err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(syn.calls) != 1 {
		t.Fatalf("syncer called %d times, want 1", len(syn.calls))
	}
	if syn.calls[0].TargetDir != "/repo/feat" {
		t.Errorf("TargetDir = %q, want /repo/feat", syn.calls[0].TargetDir)
	}
}

func TestOrchestrator_syncOnlyPassedToEditor(t *testing.T) {
	ed := &fakeEditor{}
	o := NewOrchestrator(fakeRunner{output: twoWorktreePorcelain}, ed, &fakeSyncer{})

	_ = o.Run(context.Background(), RunInput{SourcePath: "/repo/main", SyncOnly: true})
	if !ed.opts.SyncOnly {
		t.Error("SyncOnly not propagated to editor")
	}
}

func TestOrchestrator_propagatesEditorError(t *testing.T) {
	o := NewOrchestrator(
		fakeRunner{output: twoWorktreePorcelain},
		&fakeEditor{err: errors.New("editor failed")},
		&fakeSyncer{},
	)
	err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main"})
	if err == nil || !strings.Contains(err.Error(), "editor failed") {
		t.Errorf("expected editor error, got: %v", err)
	}
}

func TestOrchestrator_propagatesSyncerError(t *testing.T) {
	o := NewOrchestrator(
		fakeRunner{output: twoWorktreePorcelain},
		&fakeEditor{},
		&fakeSyncer{err: errors.New("sync failed")},
	)
	err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main"})
	if err == nil || !strings.Contains(err.Error(), "sync failed") {
		t.Errorf("expected sync error, got: %v", err)
	}
}

func TestOrchestrator_noWorktrees(t *testing.T) {
	o := NewOrchestrator(fakeRunner{output: []byte{}}, &fakeEditor{}, &fakeSyncer{})
	err := o.Run(context.Background(), RunInput{SourcePath: "/repo/main"})
	if err == nil || !strings.Contains(err.Error(), "no git worktrees found") {
		t.Errorf("expected no-worktrees error, got: %v", err)
	}
}

func TestOrchestrator_runnerError(t *testing.T) {
	o := NewOrchestrator(
		fakeRunner{err: errors.New("git not found")},
		&fakeEditor{},
		&fakeSyncer{},
	)
	err := o.Run(context.Background(), RunInput{})
	if err == nil || !strings.Contains(err.Error(), "git not found") {
		t.Errorf("expected runner error, got: %v", err)
	}
}

// --- ResolveSourceDir tests ---

func TestResolveSourceDir_useCurrentFlag(t *testing.T) {
	got, err := ResolveSourceDir(true, "", "/home/user/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/user/repo" {
		t.Errorf("got %q, want /home/user/repo", got)
	}
}

func TestResolveSourceDir_explicitPath(t *testing.T) {
	// Absolute path: filepath.Abs is a no-op, keeping the test hermetic.
	got, err := ResolveSourceDir(false, "/tmp/other-repo", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/other-repo" {
		t.Errorf("got %q, want /tmp/other-repo", got)
	}
}

func TestResolveSourceDir_defaultsToMainWorktree(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo/feat", Branch: "feat", IsMain: false},
		{Path: "/repo/main", Branch: "main", IsMain: true},
	}
	got, err := ResolveSourceDir(false, "", "", wts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/repo/main" {
		t.Errorf("got %q, want /repo/main", got)
	}
}

func TestResolveSourceDir_noMainWorktree(t *testing.T) {
	wts := []worktree.Worktree{
		{Path: "/repo/feat", Branch: "feat", IsMain: false},
	}
	_, err := ResolveSourceDir(false, "", "", wts)
	if err == nil {
		t.Fatal("expected error when no main worktree, got nil")
	}
}
