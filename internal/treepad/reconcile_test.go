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

	// gh responses. Zero-value ghAuthErr and ghListErr mean gh is available
	// and returns ghListOut (defaulted to an empty PR list) — a test opts
	// into "gh absent/failing" by setting one of these.
	ghAuthErr error
	ghListOut []byte
	ghListErr error

	mu      sync.Mutex
	adds    []string   // branches actually created, in call order
	ghCalls [][]string // gh invocations, in call order
}

func newReconcileFakeRunner(mainPath, commonDir string) *reconcileFakeRunner {
	return &reconcileFakeRunner{
		mainPath:  mainPath,
		commonDir: commonDir,
		branches:  map[string]bool{"main": true},
		failAdd:   map[string]error{},
		ghListOut: []byte("[]"),
	}
}

func (r *reconcileFakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "gh" {
		r.mu.Lock()
		r.ghCalls = append(r.ghCalls, append([]string{}, args...))
		r.mu.Unlock()
		switch {
		case len(args) >= 2 && args[0] == "auth" && args[1] == "status":
			return nil, r.ghAuthErr
		case len(args) >= 2 && args[0] == "pr" && args[1] == "list":
			return r.ghListOut, r.ghListErr
		default:
			return nil, fmt.Errorf("unexpected gh call: %v", args)
		}
	}
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
		runner.ghListOut = []byte(oneOpenPRJSON)
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
		runner.ghListOut = []byte(oneOpenPRJSON)
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
		runner.ghListOut = []byte(oneOpenPRJSON)
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
		runner.ghListOut = []byte(oneOpenPRJSON)
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

const oneOpenPRJSON = `[
	{"number":42,"headRefName":"feat/eng-12","baseRefName":"main","state":"OPEN","url":"https://x/42"}
]`

// TestReconcileReadyGate covers ticket #138: materialisation past position 0
// is gated on the parent's pull request, not its branch, and gh being
// unavailable degrades to Chain heads only rather than a hard error.
func TestReconcileReadyGate(t *testing.T) {
	threeChainManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12", "ENG-13", "ENG-15"]
`

	t.Run("fresh Manifest creates only Chain heads", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := []string{"feat/eng-12"}; len(runner.adds) != len(got) || runner.adds[0] != got[0] {
			t.Errorf("adds = %v, want %v", runner.adds, got)
		}

		byBranch := map[string]ReportEntry{}
		for _, e := range report.Members {
			byBranch[e.Branch] = e
		}
		if got := byBranch["feat/eng-12"].Action; got != ActionCreated {
			t.Errorf("feat/eng-12 action = %q, want %q", got, ActionCreated)
		}
		for _, b := range []string{"feat/eng-13", "feat/eng-15"} {
			if got := byBranch[b].Action; got != ActionBlocked {
				t.Errorf("%s action = %q, want %q", b, got, ActionBlocked)
			}
		}
	})

	t.Run("opening a pull request on the head unblocks position 1 and no further", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		if _, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()}); err != nil {
			t.Fatalf("first run: unexpected error: %v", err)
		}

		runner.ghListOut = []byte(oneOpenPRJSON)
		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("second run: unexpected error: %v", err)
		}
		wantAdds := []string{"feat/eng-12", "feat/eng-13"}
		if len(runner.adds) != len(wantAdds) {
			t.Fatalf("adds = %v, want %v", runner.adds, wantAdds)
		}
		for i, b := range wantAdds {
			if runner.adds[i] != b {
				t.Errorf("adds[%d] = %q, want %q", i, runner.adds[i], b)
			}
		}

		byBranch := map[string]ReportEntry{}
		for _, e := range report.Members {
			byBranch[e.Branch] = e
		}
		if got := byBranch["feat/eng-13"].Action; got != ActionCreated {
			t.Errorf("feat/eng-13 action = %q, want %q", got, ActionCreated)
		}
		if got := byBranch["feat/eng-15"].Action; got != ActionBlocked {
			t.Errorf("feat/eng-15 action = %q, want %q", got, ActionBlocked)
		}
	})

	t.Run("gh absent: Chain heads still materialise, deeper members report gh-required, exit 0", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghAuthErr = errors.New("gh: command not found")
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, e := range report.Members {
			if e.Action == ActionError {
				t.Errorf("member %s: action = %q, want no error (exit 0)", e.Branch, e.Action)
			}
		}
		if got := []string{"feat/eng-12"}; len(runner.adds) != len(got) || runner.adds[0] != got[0] {
			t.Errorf("adds = %v, want %v: no worktree should be created for a deeper member without gh", runner.adds, got)
		}
	})

	t.Run("gh absent marks every deeper member gh-required", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghAuthErr = errors.New("gh: command not found")
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
		for _, b := range []string{"feat/eng-13", "feat/eng-15"} {
			if got := byBranch[b].Action; got != ActionGHRequired {
				t.Errorf("%s action = %q, want %q", b, got, ActionGHRequired)
			}
		}
	})

	t.Run("--offline behaves identically to gh being absent", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", threeChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Offline: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.ghCalls) != 0 {
			t.Errorf("gh invoked %d times under --offline, want 0: %v", len(runner.ghCalls), runner.ghCalls)
		}
		if got := []string{"feat/eng-12"}; len(runner.adds) != len(got) || runner.adds[0] != got[0] {
			t.Errorf("adds = %v, want %v", runner.adds, got)
		}
		byBranch := map[string]ReportEntry{}
		for _, e := range report.Members {
			byBranch[e.Branch] = e
		}
		for _, b := range []string{"feat/eng-13", "feat/eng-15"} {
			if got := byBranch[b].Action; got != ActionGHRequired {
				t.Errorf("%s action = %q, want %q", b, got, ActionGHRequired)
			}
		}
	})
}

func TestReconcilePRState(t *testing.T) {
	oneChainManifest := `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12"]
`

	t.Run("wires gh pr list into the Report", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListOut = []byte(oneOpenPRJSON)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Members) != 1 {
			t.Fatalf("len(members) = %d, want 1", len(report.Members))
		}
		e := report.Members[0]
		if e.PRNumber != 42 || e.PRState != "OPEN" {
			t.Errorf("PRNumber/PRState = %d/%q, want 42/OPEN", e.PRNumber, e.PRState)
		}
		if e.PRStale {
			t.Error("PRStale = true, want false when gh succeeded")
		}
	})

	t.Run("--offline issues no gh call and marks entries stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Offline: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(runner.ghCalls) != 0 {
			t.Errorf("gh invoked %d times under --offline, want 0: %v", len(runner.ghCalls), runner.ghCalls)
		}
		if !report.Members[0].PRStale {
			t.Error("PRStale = false, want true under --offline")
		}
	})

	t.Run("gh unavailable does not fail the sync and marks entries stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghAuthErr = errors.New("gh: command not found")
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !report.Members[0].PRStale {
			t.Error("PRStale = false, want true when gh is unavailable")
		}
	})

	t.Run("gh pr list failing does not fail the sync and marks entries stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListErr = errors.New("network error")
		d := reconcileDeps(runner)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !report.Members[0].PRStale {
			t.Error("PRStale = false, want true when gh pr list fails")
		}
	})

	t.Run("a failing tick reports the last successful tick's PR state, marked stale", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		runner.ghListOut = []byte(oneOpenPRJSON)
		d := reconcileDeps(runner)

		if _, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()}); err != nil {
			t.Fatalf("first run: unexpected error: %v", err)
		}

		runner.ghListErr = errors.New("network error")
		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("second run: unexpected error: %v", err)
		}
		e := report.Members[0]
		if e.PRNumber != 42 || e.PRState != "OPEN" {
			t.Errorf("PRNumber/PRState = %d/%q, want last-known 42/OPEN", e.PRNumber, e.PRState)
		}
		if !e.PRStale {
			t.Error("PRStale = false, want true once gh starts failing")
		}
	})
}
