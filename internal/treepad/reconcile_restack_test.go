package treepad

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/slug"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/ui"
)

// restackWrapperDeps mirrors reconcileDeps but for a *restackWrapperRunner,
// including the Log field restack's warning path needs.
func restackWrapperDeps(runner *restackWrapperRunner) deps.Deps {
	return deps.Deps{
		Runner: runner,
		Syncer: &treepadtest.FakeSyncer{},
		Opener: &treepadtest.FakeOpener{},
		Log:    ui.New(io.Discard),
	}
}

// restackWrapperRunner layers configurable restack (`-C`) responses on top
// of reconcileFakeRunner's materialise/link/gh handling, so Reconcile's
// restack step can be exercised end-to-end without a real worktree.
type restackWrapperRunner struct {
	*reconcileFakeRunner
	dirty       map[string]bool   // worktree path -> dirty
	aheadBehind map[string][2]int // worktree path -> [ahead, behind]; default [0,0] (in sync)
	cherryPlus  map[string]bool   // worktree path -> cherry reports a "+" line
	noUpstream  map[string]bool   // worktree path -> no upstream configured
}

func (r *restackWrapperRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "git" && len(args) >= 3 && args[0] == "-C" {
		path := args[1]
		switch args[2] {
		case "fetch", "merge", "reset":
			return nil, nil
		case "status":
			if r.dirty[path] {
				return []byte("M file.go\n"), nil
			}
			return []byte(""), nil
		case "rev-parse":
			if r.noUpstream[path] {
				return nil, fmt.Errorf("no upstream")
			}
			return []byte("origin/x\n"), nil
		case "rev-list":
			ab := r.aheadBehind[path]
			return fmt.Appendf(nil, "%d\t%d\n", ab[0], ab[1]), nil
		case "cherry":
			if r.cherryPlus[path] {
				return []byte("+ abc1234 msg\n"), nil
			}
			return []byte("- abc1234 msg\n"), nil
		}
	}
	return r.reconcileFakeRunner.Run(ctx, name, args...)
}

// worktreePathForBranch mirrors worktreePathFor's derivation for a test that
// needs to pre-create the directory restack's os.Stat gate requires.
func worktreePathForBranch(mainPath, branch string) string {
	repoSlug := slug.Slug(filepath.Base(mainPath))
	return filepath.Join(filepath.Dir(mainPath), repoSlug+"-"+slug.Slug(branch))
}

func TestReconcileRestack(t *testing.T) {
	oneChainManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12"]
`

	t.Run("marks a diverged, non-patch-equivalent member stack-stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		base := newReconcileFakeRunner(mainPath, commonDir)
		base.ghListOut = []byte(oneOpenPRJSON)

		wtPath := worktreePathForBranch(mainPath, "feat/eng-12")
		if err := os.MkdirAll(wtPath, 0o755); err != nil {
			t.Fatal(err)
		}

		runner := &restackWrapperRunner{
			reconcileFakeRunner: base,
			aheadBehind:         map[string][2]int{wtPath: {1, 1}},
			cherryPlus:          map[string]bool{wtPath: true},
		}
		d := restackWrapperDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var got ReportEntry
		found := false
		for _, e := range report.Members {
			if e.Branch == "feat/eng-12" {
				got, found = e, true
			}
		}
		if !found {
			t.Fatal("feat/eng-12 not in report")
		}
		if !got.StackStale {
			t.Errorf("expected feat/eng-12 to be stack-stale, got %+v", got)
		}
	})

	t.Run("clean plain-behind member fast-forwards, no stack-stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		base := newReconcileFakeRunner(mainPath, commonDir)
		base.ghListOut = []byte(oneOpenPRJSON)

		wtPath := worktreePathForBranch(mainPath, "feat/eng-12")
		if err := os.MkdirAll(wtPath, 0o755); err != nil {
			t.Fatal(err)
		}

		runner := &restackWrapperRunner{
			reconcileFakeRunner: base,
			aheadBehind:         map[string][2]int{wtPath: {0, 4}},
		}
		d := restackWrapperDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, e := range report.Members {
			if e.Branch == "feat/eng-12" && e.StackStale {
				t.Errorf("expected feat/eng-12 not to be stack-stale (plain fast-forward), got %+v", e)
			}
		}
	})

	t.Run("a member with no worktree on disk yet is skipped", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		// No PR yet: position 0 still materialises, but its path was never
		// created on disk by this fake runner, so restack must not touch it.
		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Members[0].StackStale {
			t.Errorf("member without a real worktree on disk must not be marked stack-stale: %+v", report.Members[0])
		}
	})
}

func TestReconcileRetire(t *testing.T) {
	oneChainManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12"]
`

	t.Run("marks a member with a MERGED pull request removable", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListOut = []byte(`[
			{"number":42,"headRefName":"feat/eng-12","baseRefName":"main","state":"MERGED","url":"https://x/42"}
		]`)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Members) != 1 || !report.Members[0].Removable {
			t.Errorf("expected feat/eng-12 to be removable, got %+v", report.Members)
		}
	})

	t.Run("an open pull request does not mark a member removable", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListOut = []byte(oneOpenPRJSON)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Members[0].Removable {
			t.Errorf("an open pull request must not be reported removable: %+v", report.Members[0])
		}
	})

	t.Run("treepad never merges: no gh pr merge call", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListOut = []byte(`[
			{"number":42,"headRefName":"feat/eng-12","baseRefName":"main","state":"MERGED","url":"https://x/42"}
		]`)
		d := reconcileDeps(runner)

		if _, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, call := range runner.ghCalls {
			if len(call) >= 2 && call[0] == "pr" && call[1] == "merge" {
				t.Errorf("Reconcile issued a gh pr merge call: %v", call)
			}
		}
	})
}
