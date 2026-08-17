package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

// reconcileFakeRunner answers the git calls Reconcile's materialise step
// makes: worktree listing, common-dir resolution, branch-exists checks, and
// `git worktree add`. It tracks branch existence statefully so a materialised
// member's branch is visible to the next member's parent-exists check,
// without needing to hand-order a response queue.
type reconcileFakeRunner struct {
	mainPath, commonDir string
	branches            map[string]bool
	failAdd             map[string]error // branch -> error to return from `worktree add`

	mu   sync.Mutex
	adds []string // branches actually created, in call order
}

func newReconcileFakeRunner(mainPath, commonDir string) *reconcileFakeRunner {
	return &reconcileFakeRunner{
		mainPath:  mainPath,
		commonDir: commonDir,
		branches:  map[string]bool{"main": true},
		failAdd:   map[string]error{},
	}
}

func (r *reconcileFakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name != "git" {
		return nil, fmt.Errorf("unexpected command %s", name)
	}
	switch {
	case len(args) >= 2 && args[0] == "worktree" && args[1] == "list":
		return treepadtest.MainWorktreePorcelain(r.mainPath), nil
	case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--git-common-dir":
		return []byte(r.commonDir + "\n"), nil
	case len(args) >= 3 && args[0] == "branch" && args[1] == "--list":
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.branches[args[2]] {
			return []byte(args[2] + "\n"), nil
		}
		return nil, nil
	case len(args) >= 5 && args[0] == "worktree" && args[1] == "add" && args[3] == "-b":
		branch := args[4]
		if err, ok := r.failAdd[branch]; ok {
			return nil, err
		}
		r.mu.Lock()
		r.branches[branch] = true
		r.adds = append(r.adds, branch)
		r.mu.Unlock()
		return nil, nil
	case len(args) >= 1 && args[0] == "-C":
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected git call: %v", args)
	}
}

const reconcileTOML = `
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
`

func reconcileDeps(runner *reconcileFakeRunner) deps.Deps {
	return deps.Deps{
		Runner: runner,
		Syncer: &treepadtest.FakeSyncer{},
		Opener: &treepadtest.FakeOpener{},
	}
}

func writeReconcileManifest(t *testing.T, commonDir, name, content string) {
	t.Helper()
	batchesDir := filepath.Join(commonDir, "treepad", "batches")
	if err := os.MkdirAll(batchesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batchesDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupReconcileRepo(t *testing.T) (mainPath, commonDir string) {
	t.Helper()
	mainPath = makeMainWorktree(t)
	commonDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(reconcileTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, commonDir
}

func TestReconcile(t *testing.T) {
	twoChainManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12", "ENG-13"]
[[chain]]
tickets = ["ENG-14"]
`

	t.Run("materialises a stacked Chain in order", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", twoChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Members) != 3 {
			t.Fatalf("len(members) = %d, want 3", len(report.Members))
		}
		for _, e := range report.Members {
			if e.Action != ActionCreated {
				t.Errorf("member %s: action = %q, want %q", e.Branch, e.Action, ActionCreated)
			}
			if e.WorktreePath == "" {
				t.Errorf("member %s: WorktreePath is empty", e.Branch)
			}
		}
		wantOrder := []string{"feat/eng-12", "feat/eng-13", "feat/eng-14"}
		if len(runner.adds) != len(wantOrder) {
			t.Fatalf("worktree add called %d times, want %d: %v", len(runner.adds), len(wantOrder), runner.adds)
		}
		for i, b := range wantOrder {
			if runner.adds[i] != b {
				t.Errorf("adds[%d] = %q, want %q", i, runner.adds[i], b)
			}
		}
	})

	t.Run("re-running is idempotent: creates nothing", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", twoChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		if _, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()}); err != nil {
			t.Fatalf("first run: unexpected error: %v", err)
		}
		firstAdds := len(runner.adds)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("second run: unexpected error: %v", err)
		}
		if len(runner.adds) != firstAdds {
			t.Errorf("second run created %d more worktrees, want 0", len(runner.adds)-firstAdds)
		}
		for _, e := range report.Members {
			if e.Action != ActionSkipped {
				t.Errorf("member %s: action = %q, want %q", e.Branch, e.Action, ActionSkipped)
			}
		}
	})

	t.Run("a failing member stops its own Chain only", func(t *testing.T) {
		threeManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12", "ENG-13", "ENG-15"]
[[chain]]
tickets = ["ENG-14"]
`
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.failAdd["feat/eng-13"] = errors.New("branch already exists")
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		byBranch := map[string]ReportEntry{}
		for _, e := range report.Members {
			byBranch[e.Branch] = e
		}

		if got := byBranch["feat/eng-12"].Action; got != ActionCreated {
			t.Errorf("feat/eng-12 action = %q, want %q", got, ActionCreated)
		}
		if got := byBranch["feat/eng-13"].Action; got != ActionError {
			t.Errorf("feat/eng-13 action = %q, want %q", got, ActionError)
		}
		if _, ok := byBranch["feat/eng-15"]; ok {
			t.Error("feat/eng-15 should not appear: its Chain stopped at the prior failure")
		}
		if got := byBranch["feat/eng-14"].Action; got != ActionCreated {
			t.Errorf("other Chain's feat/eng-14 action = %q, want %q (must not be affected)", got, ActionCreated)
		}
	})

	t.Run("--dry-run reports the whole stacked Chain and creates nothing", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", twoChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), DryRun: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.adds) != 0 {
			t.Errorf("dry-run called worktree add %d times, want 0", len(runner.adds))
		}
		if len(report.Members) != 3 {
			t.Fatalf("len(members) = %d, want 3", len(report.Members))
		}
		for _, e := range report.Members {
			if e.Action != ActionWouldCreate {
				t.Errorf("member %s: action = %q, want %q", e.Branch, e.Action, ActionWouldCreate)
			}
		}
	})

	t.Run("--batch narrows to one Manifest", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", twoChainManifest)
		writeReconcileManifest(t, commonDir, "other.toml", `
name = "other"
[[chain]]
tickets = ["ENG-99"]
`)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Batch: "other"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Members) != 1 {
			t.Fatalf("len(members) = %d, want 1", len(report.Members))
		}
		if report.Members[0].Ticket != "ENG-99" {
			t.Errorf("ticket = %q, want ENG-99", report.Members[0].Ticket)
		}
	})
}
