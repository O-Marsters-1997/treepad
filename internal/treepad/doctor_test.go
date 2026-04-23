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
	internalsync "treepad/internal/sync"
)

// recentCommitOutput returns a git log line for a commit made 1 minute ago.
func recentCommitOutput(sha, subject string) []byte {
	t := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)
	return []byte(sha + "\x00" + subject + "\x00" + t + "\n")
}

// staleCommitOutput returns a git log line for a commit made 60 days ago.
func staleCommitOutput(sha, subject string) []byte {
	t := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	return []byte(sha + "\x00" + subject + "\x00" + t + "\n")
}

func TestDoctor(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := t.TempDir()
	outputDir := t.TempDir()
	porcelain := twoWorktreePorcelainWithMain(mainPath, featPath)

	// offlineInput returns a DoctorInput with Offline=true so tests don't need
	// to stub rev-parse / ls-remote calls unless specifically testing remote-gone.
	offlineInput := func(extra ...func(*DoctorInput)) DoctorInput {
		in := DoctorInput{
			StaleDays: 30,
			Base:      "main",
			Offline:   true,
			OutputDir: outputDir,
		}
		for _, f := range extra {
			f(&in)
		}
		return in
	}

	t.Run("stale finding when last commit exceeds threshold", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},                                // git worktree list
			{output: []byte("aaa111\n")},                       // git rev-parse main^{commit}
			{output: []byte("")},                               // git for-each-ref --merged (nothing)
			{output: recentCommitOutput("abc1234", "init")},    // log: main (recent)
			{output: []byte("")},                               // dirty: main (clean)
			{output: staleCommitOutput("def5678", "old work")}, // log: feat (stale)
			{output: []byte("")},                               // dirty: feat (clean)
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
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
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},                               // dirty: main clean
			{output: staleCommitOutput("def5678", "old work")}, // feat: stale
			{output: []byte("M file.go\n")},                    // dirty: feat dirty
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "dirty-old") {
			t.Errorf("output missing 'dirty-old' finding:\n%s", out)
		}
		// stale should NOT appear separately when dirty-old is reported
		if strings.Contains(out, "stale\t") {
			t.Errorf("stale should not be reported alongside dirty-old:\n%s", out)
		}
	})

	t.Run("merged-present finding when worktree branch is in merged set", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")},                      // git rev-parse main^{commit}
			{output: []byte("feat bbb222\n")},                 // for-each-ref --merged: feat is merged
			{output: recentCommitOutput("abc1234", "init")},   // log: main
			{output: []byte("")},                              // dirty: main
			{output: recentCommitOutput("def5678", "feat x")}, // log: feat
			{output: []byte("")},                              // dirty: feat
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
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
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")},                      // git rev-parse main^{commit}
			{output: []byte("")},                              // for-each-ref --merged: none
			{output: recentCommitOutput("abc1234", "init")},   // log: main
			{output: []byte("")},                              // dirty: main
			{err: errors.New("no upstream")},                  // rev-parse @{upstream}: main (none)
			{output: recentCommitOutput("def5678", "feat x")}, // log: feat
			{output: []byte("")},                              // dirty: feat
			{output: []byte("origin/feat\n")},                 // rev-parse @{upstream}: feat has upstream
			{output: []byte("")},                              // ls-remote: empty → branch gone
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		in := DoctorInput{StaleDays: 30, Base: "main", Offline: false, OutputDir: outputDir}
		err := Doctor(context.Background(), d, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "remote-gone") {
			t.Errorf("output missing 'remote-gone' finding:\n%s", out)
		}
	})

	t.Run("offline flag skips remote-gone check", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		d := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// All 7 calls consumed; any 8th would trip seqRunner's out-of-bounds guard.
		if runner.idx != 7 {
			t.Errorf("runner called %d times, want 7 (rev-parse/ls-remote must be skipped)", runner.idx)
		}
	})

	t.Run("artifact-missing finding when expected file is absent", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		// outputDir has no artifact files on disk → both worktrees flagged missing.
		emptyOutputDir := t.TempDir()
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput(func(in *DoctorInput) {
			in.OutputDir = emptyOutputDir
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "artifact-missing") {
			t.Errorf("output missing 'artifact-missing' finding:\n%s", out)
		}
	})

	t.Run("config-drift finding when worktree has different sync files", func(t *testing.T) {
		// Write a .treepad.toml with different sync files to featPath.
		toml := "[sync]\ninclude = [\"custom-file\"]\n"
		if err := os.WriteFile(filepath.Join(featPath, ".treepad.toml"), []byte(toml), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(featPath, ".treepad.toml")) })

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
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
		// Create artifact files so artifact-missing is not triggered.
		artifactDir := t.TempDir()
		repoSlug := strings.TrimSuffix(filepath.Base(mainPath), filepath.Ext(filepath.Base(mainPath)))
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-main.code-workspace"), []byte("{}"), 0o644)
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-feat.code-workspace"), []byte("{}"), 0o644)

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput(func(in *DoctorInput) {
			in.OutputDir = artifactDir
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "no issues found") {
			t.Errorf("expected 'no issues found'; got:\n%s", buf.String())
		}
	})

	t.Run("json flag emits JSON array", func(t *testing.T) {
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{output: []byte("feat bbb222\n")}, // for-each-ref --merged: feat merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		var buf strings.Builder
		d := Deps{Runner: runner, Syncer: &fakeSyncer{}, Opener: &fakeOpener{}, Out: &buf, In: strings.NewReader("")}

		err := Doctor(context.Background(), d, offlineInput(func(in *DoctorInput) {
			in.JSON = true
		}))
		if err != nil {
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
		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")},      // git rev-parse main^{commit}
			{output: []byte("feat bbb222\n")}, // for-each-ref --merged: feat merged → finding
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		d := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Doctor(context.Background(), d, offlineInput(func(in *DoctorInput) {
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
		repoSlug := strings.TrimSuffix(filepath.Base(mainPath), filepath.Ext(filepath.Base(mainPath)))
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-main.code-workspace"), []byte("{}"), 0o644)
		_ = os.WriteFile(filepath.Join(artifactDir, repoSlug+"-feat.code-workspace"), []byte("{}"), 0o644)

		runner := &seqRunner{responses: []runResponse{
			{output: porcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			{output: recentCommitOutput("abc1234", "init")},
			{output: []byte("")},
			{output: recentCommitOutput("def5678", "feat x")},
			{output: []byte("")},
		}}
		d := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Doctor(context.Background(), d, offlineInput(func(in *DoctorInput) {
			in.Strict = true
			in.OutputDir = artifactDir
		}))
		if err != nil {
			t.Fatalf("strict with no findings should return nil, got: %v", err)
		}
	})

	t.Run("skips detached-head worktrees", func(t *testing.T) {
		detachedPorcelain := []byte("worktree " + mainPath + "\nHEAD abc123\ndetached\n\n")
		runner := &seqRunner{responses: []runResponse{
			{output: detachedPorcelain},
			{output: []byte("aaa111\n")}, // git rev-parse main^{commit}
			{output: []byte("")},         // git for-each-ref --merged
			// no per-worktree calls because detached is skipped
		}}
		d := testDeps(runner, &fakeSyncer{}, &fakeOpener{})

		err := Doctor(context.Background(), d, offlineInput())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.idx != 3 {
			t.Errorf("runner called %d times, want 3 (no per-wt calls for detached)", runner.idx)
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
			name: "git rev-parse base fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{err: errors.New("unknown revision")},
			}},
			wantErr: "unknown revision",
		},
		{
			name: "git for-each-ref --merged fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{output: []byte("aaa111\n")},
				{err: errors.New("for-each-ref failed")},
			}},
			wantErr: "for-each-ref failed",
		},
		{
			name: "last commit probe fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{output: []byte("")},
				{err: errors.New("git log failed")},
			}},
			wantErr: "git log failed",
		},
		{
			name: "dirty probe fails",
			runner: &seqRunner{responses: []runResponse{
				{output: porcelain},
				{output: []byte("")},
				{output: recentCommitOutput("abc1234", "init")},
				{err: errors.New("git status failed")},
			}},
			wantErr: "git status failed",
		},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDeps(tt.runner, &fakeSyncer{}, &fakeOpener{})
			err := Doctor(context.Background(), d, offlineInput())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// Ensure DoctorInput and DoctorFinding are usable in tests without importing
// extra packages — smoke-test the types compile correctly.
var _ = DoctorInput{}
var _ = DoctorFinding{}
var _ internalsync.Config
var _ artifact.Spec
