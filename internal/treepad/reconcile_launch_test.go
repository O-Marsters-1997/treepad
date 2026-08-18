package treepad

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/launcher"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/ui"
)

// fakeLauncher fakes launcher.Launcher: it never spawns a real process. It
// mirrors the real Launcher's effect on the Activity file, since the launch
// step's double-launch guard reads that file from disk.
type fakeLauncher struct {
	mu    sync.Mutex
	calls []fakeLaunchCall
	err   error
}

type fakeLaunchCall struct {
	argv         []string
	dir          string
	activityFile string
}

func (f *fakeLauncher) Launch(argv []string, dir, activityFile string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeLaunchCall{argv: argv, dir: dir, activityFile: activityFile})
	if f.err != nil {
		return f.err
	}
	if err := os.MkdirAll(filepath.Dir(activityFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(activityFile, nil, 0o644)
}

const launchTOML = `
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"

[batch]
launch = ["claude", "--dangerously-skip-permissions", "{{.TicketURL}}"]
`

func setupLaunchRepo(t *testing.T) (mainPath, commonDir string) {
	t.Helper()
	mainPath = makeMainWorktree(t)
	commonDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(launchTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, commonDir
}

func launchDeps(runner *reconcileFakeRunner, l *fakeLauncher) (deps.Deps, *strings.Builder) {
	var logBuf strings.Builder
	d := reconcileDeps(runner)
	d.Launcher = l
	d.Log = ui.New(&logBuf)
	return d, &logBuf
}

const oneChainOneMemberManifest = `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12"]
`

func TestReconcileLaunch(t *testing.T) {
	t.Run("without --launch, nothing is spawned even with [batch] launch configured", func(t *testing.T) {
		mainPath, commonDir := setupLaunchRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainOneMemberManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		l := &fakeLauncher{}
		d, _ := launchDeps(runner, l)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(l.calls) != 0 {
			t.Errorf("Launch called %d times, want 0", len(l.calls))
		}
		if report.Members[0].Action != ActionCreated {
			t.Errorf("action = %q, want %q", report.Members[0].Action, ActionCreated)
		}
	})

	t.Run("empty [batch] launch reports ready to launch and spawns nothing", func(t *testing.T) {
		mainPath, commonDir := setupReconcileRepo(t) // no [batch] section
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainOneMemberManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		l := &fakeLauncher{}
		d, logBuf := launchDeps(runner, l)

		_, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Launch: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(l.calls) != 0 {
			t.Errorf("Launch called %d times, want 0", len(l.calls))
		}
		if !strings.Contains(logBuf.String(), "1 ready to launch") {
			t.Errorf("log = %q, want it to mention %q", logBuf.String(), "1 ready to launch")
		}
	})

	t.Run("--launch with [batch] launch configured spawns each materialised member", func(t *testing.T) {
		mainPath, commonDir := setupLaunchRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainOneMemberManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		l := &fakeLauncher{}
		d, _ := launchDeps(runner, l)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Launch: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(l.calls) != 1 {
			t.Fatalf("Launch called %d times, want 1", len(l.calls))
		}
		call := l.calls[0]
		wantArgv := []string{"claude", "--dangerously-skip-permissions", "https://linear.app/acme/issue/ENG-12"}
		if len(call.argv) != len(wantArgv) {
			t.Fatalf("argv = %v, want %v", call.argv, wantArgv)
		}
		for i, a := range wantArgv {
			if call.argv[i] != a {
				t.Errorf("argv[%d] = %q, want %q", i, call.argv[i], a)
			}
		}
		if call.dir != report.Members[0].WorktreePath {
			t.Errorf("dir = %q, want %q", call.dir, report.Members[0].WorktreePath)
		}
		wantActivity := launcher.ActivityPath(commonDir, "feat/eng-12")
		if call.activityFile != wantActivity {
			t.Errorf("activityFile = %q, want %q", call.activityFile, wantActivity)
		}
		if report.Members[0].Action != ActionLaunched {
			t.Errorf("action = %q, want %q", report.Members[0].Action, ActionLaunched)
		}
	})

	t.Run("a second --launch run does not relaunch a member with an Activity file", func(t *testing.T) {
		mainPath, commonDir := setupLaunchRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainOneMemberManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		l := &fakeLauncher{}
		d, _ := launchDeps(runner, l)

		if _, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Launch: true}); err != nil {
			t.Fatalf("first run: unexpected error: %v", err)
		}
		if len(l.calls) != 1 {
			t.Fatalf("first run: Launch called %d times, want 1", len(l.calls))
		}

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Launch: true})
		if err != nil {
			t.Fatalf("second run: unexpected error: %v", err)
		}
		if len(l.calls) != 1 {
			t.Errorf("second run: Launch called %d times total, want still 1 (no relaunch)", len(l.calls))
		}
		if report.Members[0].Action != ActionSkipped {
			t.Errorf("second run action = %q, want %q (not relaunched)", report.Members[0].Action, ActionSkipped)
		}
	})

	t.Run("a launch failure marks the member an error", func(t *testing.T) {
		mainPath, commonDir := setupLaunchRepo(t)
		writeReconcileManifest(t, commonDir, "silent-refresh.toml", oneChainOneMemberManifest)
		runner := newReconcileFakeRunner(mainPath, commonDir)
		l := &fakeLauncher{err: os.ErrPermission}
		d, _ := launchDeps(runner, l)

		report, err := Reconcile(context.Background(), d, ReconcileInput{OutputDir: t.TempDir(), Launch: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Members[0].Action != ActionError {
			t.Errorf("action = %q, want %q", report.Members[0].Action, ActionError)
		}
	})
}
