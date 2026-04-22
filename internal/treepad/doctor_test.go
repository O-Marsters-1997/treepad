package treepad

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"treepad/internal/artifact"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

// recentCommit returns a CommitInfo with Committed set 1 minute ago.
func recentCommit(sha, subject string) worktree.CommitInfo {
	return worktree.CommitInfo{ShortSHA: sha, Subject: subject, Committed: time.Now().Add(-1 * time.Minute)}
}

// staleCommit returns a CommitInfo with Committed set 60 days ago.
func staleCommit(sha, subject string) worktree.CommitInfo {
	return worktree.CommitInfo{ShortSHA: sha, Subject: subject, Committed: time.Now().Add(-60 * 24 * time.Hour)}
}

func TestDoctor(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := t.TempDir()
	outputDir := t.TempDir()

	mainWT := worktree.Worktree{Branch: "main", Path: mainPath, IsMain: true}
	featWT := worktree.Worktree{Branch: "feat", Path: featPath}

	offlineInput := func(extra ...func(*DoctorInput)) DoctorInput {
		in := DoctorInput{StaleDays: 30, Base: "main", Offline: true, OutputDir: outputDir}
		for _, f := range extra {
			f(&in)
		}
		return in
	}

	repoSlug := slug.Slug(filepath.Base(mainPath))

	newFake := func(opts ...func(*fakeRepoView)) *fakeRepoView {
		f := &fakeRepoView{
			main:      mainWT,
			worktrees: []worktree.Worktree{mainWT, featWT},
			slug:      repoSlug,
			outputDir: outputDir,
			lastCommitByBranch: map[string]worktree.CommitInfo{
				"main": recentCommit("abc1234", "init"),
				"feat": recentCommit("def5678", "feat x"),
			},
		}
		for _, o := range opts {
			o(f)
		}
		return f
	}
	withFake := func(f *fakeRepoView) func(context.Context, string) (RepoView, error) {
		return func(_ context.Context, _ string) (RepoView, error) { return f, nil }
	}

	t.Run("stale finding when last commit exceeds threshold", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.lastCommitByBranch["feat"] = staleCommit("def5678", "old work")
		}))

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "stale") {
			t.Errorf("output missing 'stale' finding:\n%s", out)
		}
		if !strings.Contains(out, "feat") {
			t.Errorf("output missing 'feat' branch:\n%s", out)
		}
	})

	t.Run("dirty-old finding supersedes stale when worktree is also dirty", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.lastCommitByBranch["feat"] = staleCommit("def5678", "old work")
			f.dirtyByBranch = map[string]bool{"feat": true}
		}))

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "dirty-old") {
			t.Errorf("output missing 'dirty-old' finding:\n%s", out)
		}
		if strings.Contains(out, "stale\t") {
			t.Errorf("stale should not be reported alongside dirty-old:\n%s", out)
		}
	})

	t.Run("merged-present finding when worktree branch is in merged set", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "merged-present") {
			t.Errorf("output missing 'merged-present' finding:\n%s", out)
		}
		if !strings.Contains(out, "feat") {
			t.Errorf("output missing 'feat' branch:\n%s", out)
		}
	})

	t.Run("remote-gone finding when upstream configured but branch absent on remote", func(t *testing.T) {
		// RemoteBranchExists calls: rev-parse (no upstream for main), rev-parse+ls-remote for feat
		runner := &seqRunner{responses: []runResponse{
			{err: errors.New("no upstream")}, // main: no upstream
			{output: []byte("origin/feat\n")}, // feat: has upstream
			{output: []byte("")},              // ls-remote: empty → branch gone
		}}
		var buf strings.Builder
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake())

		in := DoctorInput{StaleDays: 30, Base: "main", Offline: false, OutputDir: outputDir}
		if err := Doctor(context.Background(), deps, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "remote-gone") {
			t.Errorf("output missing 'remote-gone' finding:\n%s", buf.String())
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3 (2 worktrees × remote checks)", runner.idx)
		}
	})

	t.Run("offline flag skips remote-gone check", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{}}
		deps := testDeps(runner, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake())

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 0 {
			t.Errorf("runner called %d times, want 0 (remote checks must be skipped)", runner.idx)
		}
	})

	t.Run("artifact-missing finding when expected file is absent", func(t *testing.T) {
		emptyOutputDir := t.TempDir()
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.outputDir = emptyOutputDir
		}))

		if err := Doctor(context.Background(), deps, offlineInput(func(in *DoctorInput) {
			in.OutputDir = emptyOutputDir
		})); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "artifact-missing") {
			t.Errorf("output missing 'artifact-missing' finding:\n%s", buf.String())
		}
	})

	t.Run("config-drift finding when worktree has different sync files", func(t *testing.T) {
		toml := "[sync]\ninclude = [\"custom-file\"]\n"
		if err := os.WriteFile(filepath.Join(featPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(featPath, ".treepad.toml")) })

		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake())

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "config-drift") {
			t.Errorf("output missing 'config-drift' finding:\n%s", out)
		}
		if !strings.Contains(out, "sync") {
			t.Errorf("drift detail should mention 'sync':\n%s", out)
		}
	})

	t.Run("no issues found message when everything is clean", func(t *testing.T) {
		artifactDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-main.code-workspace"), []byte("{}"), 0o644)
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-feat.code-workspace"), []byte("{}"), 0o644)

		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.outputDir = artifactDir
		}))

		if err := Doctor(context.Background(), deps, offlineInput(func(in *DoctorInput) {
			in.OutputDir = artifactDir
		})); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no issues found") {
			t.Errorf("expected 'no issues found'; got:\n%s", buf.String())
		}
	})

	t.Run("json flag emits JSON array", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		if err := Doctor(context.Background(), deps, offlineInput(func(in *DoctorInput) {
			in.JSON = true
		})); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, "[") {
			t.Errorf("expected JSON array, got: %s", out)
		}
		for _, want := range []string{`"kind"`, `"branch"`, "merged-present", "feat"} {
			if !strings.Contains(out, want) {
				t.Errorf("JSON output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("strict returns error when findings exist", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.merged = map[string][]string{"main": {"feat"}}
		}))

		err := Doctor(context.Background(), deps, offlineInput(func(in *DoctorInput) {
			in.Strict = true
		}))
		if err == nil {
			t.Fatal("expected error with --strict when findings exist, got nil")
		}
		if !strings.Contains(err.Error(), "finding") {
			t.Errorf("error %q should mention findings", err)
		}
	})

	t.Run("strict returns nil when no findings", func(t *testing.T) {
		artifactDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-main.code-workspace"), []byte("{}"), 0o644)
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-feat.code-workspace"), []byte("{}"), 0o644)

		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = withFake(newFake(func(f *fakeRepoView) {
			f.outputDir = artifactDir
		}))

		if err := Doctor(context.Background(), deps, offlineInput(func(in *DoctorInput) {
			in.Strict = true
			in.OutputDir = artifactDir
		})); err != nil {
			t.Fatalf("strict with no findings should return nil, got: %v", err)
		}
	})

	t.Run("skips detached-head worktrees", func(t *testing.T) {
		detachedWT := worktree.Worktree{Branch: "(detached)", Path: mainPath, IsMain: true}
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return &fakeRepoView{
				main:      detachedWT,
				worktrees: []worktree.Worktree{detachedWT},
				outputDir: outputDir,
			}, nil
		}

		if err := Doctor(context.Background(), deps, offlineInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("NewRepoView failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("git not found")
		}
		err := Doctor(context.Background(), deps, offlineInput())
		if err == nil || !strings.Contains(err.Error(), "git not found") {
			t.Errorf("got error %v, want error containing 'git not found'", err)
		}
	})

	t.Run("MergedInto failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("unknown revision")
		}
		err := Doctor(context.Background(), deps, offlineInput())
		if err == nil || !strings.Contains(err.Error(), "unknown revision") {
			t.Errorf("got error %v, want error containing 'unknown revision'", err)
		}
	})
}

// Ensure DoctorInput and DoctorFinding are usable in tests without importing
// extra packages — smoke-test the types compile correctly.
var _ = DoctorInput{}
var _ = DoctorFinding{}
var _ internalsync.Config
var _ artifact.Spec
