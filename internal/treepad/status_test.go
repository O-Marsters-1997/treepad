package treepad

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"treepad/internal/slug"
	"treepad/internal/worktree"
)

func TestStatus(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	featPath := mainPath + "-feat"
	outputDir := t.TempDir()
	repoSlug := slug.Slug(filepath.Base(mainPath))

	mainWT := worktree.Worktree{Branch: "main", Path: mainPath, IsMain: true}
	featWT := worktree.Worktree{Branch: "feat", Path: featPath}

	newFake := func(opts ...func(*fakeRepoView)) func(context.Context, string) (RepoView, error) {
		f := &fakeRepoView{
			main:      mainWT,
			worktrees: []worktree.Worktree{mainWT, featWT},
			slug:      repoSlug,
			outputDir: outputDir,
		}
		for _, o := range opts {
			o(f)
		}
		return func(_ context.Context, _ string) (RepoView, error) { return f, nil }
	}

	t.Run("renders table for two worktrees", func(t *testing.T) {
		featArtifact := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(featArtifact, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup artifact: %v", err)
		}

		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = newFake(func(f *fakeRepoView) {
			f.dirtyByBranch = map[string]bool{"feat": true}
			f.aheadBehindByBranch = map[string]fakeAheadBehind{
				"main": {A: 0, B: 1, HasUpstream: true},
			}
			f.lastCommitByBranch = map[string]worktree.CommitInfo{
				"main": {ShortSHA: "abc1234", Subject: "init"},
				"feat": {ShortSHA: "def5678", Subject: "add x"},
			}
		})

		if err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"BRANCH", "main", "feat", "clean", "dirty", "↑0 ↓1", "—", "abc1234", "def5678"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("json flag emits JSON array", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = newFake(func(f *fakeRepoView) {
			f.lastCommitByBranch = map[string]worktree.CommitInfo{
				"main": {ShortSHA: "abc1234", Subject: "init"},
				"feat": {ShortSHA: "def5678", Subject: "add x"},
			}
		})

		if err := Status(context.Background(), deps, StatusInput{JSON: true, OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, "[") {
			t.Errorf("expected JSON array, got: %s", out)
		}
		var rows []StatusRow
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("got %d rows, want 2", len(rows))
		}
		for _, want := range []string{"main", "feat", `"dirty"`, `"branch"`} {
			if !strings.Contains(out, want) {
				t.Errorf("JSON output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("prunable worktree renders without git calls", func(t *testing.T) {
		prunablePath := mainPath + "-stale"
		prunableWT := worktree.Worktree{
			Branch: "stale-branch", Path: prunablePath,
			Prunable: true, PrunableReason: "gitdir file points to non-existent location",
		}

		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return &fakeRepoView{
				main:      mainWT,
				worktrees: []worktree.Worktree{mainWT, prunableWT},
				slug:      repoSlug,
				outputDir: outputDir,
				aheadBehindByBranch: map[string]fakeAheadBehind{
					"main": {A: 0, B: 0, HasUpstream: true},
				},
				lastCommitByBranch: map[string]worktree.CommitInfo{
					"main": {ShortSHA: "abc1234", Subject: "init"},
				},
			}, nil
		}

		if err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"stale-branch", "prunable", "gitdir file points to non-existent location", "tp prune"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("artifact path and last-touched populated when file exists", func(t *testing.T) {
		artifactFile := filepath.Join(outputDir, repoSlug+"-feat.code-workspace")
		if err := os.WriteFile(artifactFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		var buf strings.Builder
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf
		deps.NewRepoView = newFake(func(f *fakeRepoView) {
			f.lastCommitByBranch = map[string]worktree.CommitInfo{
				"main": {ShortSHA: "abc1234", Subject: "init", Committed: time.Now().Add(-1 * time.Hour)},
				"feat": {ShortSHA: "def5678", Subject: "add x", Committed: time.Now().Add(-2 * time.Hour)},
			}
		})

		rows, err := refreshStatus(context.Background(), deps, StatusInput{OutputDir: outputDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var featRow *StatusRow
		for i := range rows {
			if rows[i].Branch == "feat" {
				featRow = &rows[i]
			}
		}
		if featRow == nil {
			t.Fatal("no feat row returned")
		}
		if featRow.ArtifactPath == "" {
			t.Error("expected ArtifactPath to be set for feat")
		}
		if featRow.LastTouched.IsZero() {
			t.Error("expected LastTouched to be set for feat")
		}
	})

	t.Run("NewRepoView failure propagates", func(t *testing.T) {
		deps := testDeps(fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.NewRepoView = func(_ context.Context, _ string) (RepoView, error) {
			return nil, errors.New("git not found")
		}
		err := Status(context.Background(), deps, StatusInput{OutputDir: outputDir})
		if err == nil || !strings.Contains(err.Error(), "git not found") {
			t.Errorf("got error %v, want error containing %q", err, "git not found")
		}
	})
}
