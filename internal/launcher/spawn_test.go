package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestProcessLauncher_SpawnSurvivesRelease proves the detached child keeps
// running after this test — standing in for "tp batch sync" — returns from
// Launch's Start()+release, without ever waiting on the child. A fixture
// shell command sleeps briefly, then writes a sentinel file; the test
// returns immediately and polls for the sentinel to appear.
func TestProcessLauncher_SpawnSurvivesRelease(t *testing.T) {
	dir := t.TempDir()
	activityFile := filepath.Join(dir, "activity.log")
	sentinel := filepath.Join(dir, "sentinel")

	argv := []string{"sh", "-c", "sleep 0.3; echo done > " + sentinel}

	start := time.Now()
	if err := (ProcessLauncher{}).Launch(argv, dir, activityFile); err != nil {
		t.Fatalf("Launch() error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("Launch() took %v; it must return immediately rather than waiting on the child", elapsed)
	}

	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("sentinel already exists right after Launch() returned; the fixture isn't actually sleeping")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sentinel); err == nil {
			return // survived: the child wrote its sentinel after the parent returned
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sentinel never appeared; the child did not survive Launch() returning")
}

// TestProcessLauncher_StdoutAndStderr asserts both streams land in the
// Activity file.
func TestProcessLauncher_StdoutAndStderr(t *testing.T) {
	dir := t.TempDir()
	activityFile := filepath.Join(dir, "activity.log")

	argv := []string{"sh", "-c", "echo to-stdout; echo to-stderr 1>&2"}
	if err := (ProcessLauncher{}).Launch(argv, dir, activityFile); err != nil {
		t.Fatalf("Launch() error: %v", err)
	}

	var contents []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(activityFile)
		if err == nil && len(b) > 0 {
			contents = b
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := string(contents)
	if !strings.Contains(got, "to-stdout") {
		t.Errorf("Activity file missing stdout content; got %q", got)
	}
	if !strings.Contains(got, "to-stderr") {
		t.Errorf("Activity file missing stderr content; got %q", got)
	}
}

// TestProcessLauncher_CmdDir asserts the child runs with dir as its working
// directory.
func TestProcessLauncher_CmdDir(t *testing.T) {
	dir := t.TempDir()
	activityFile := filepath.Join(dir, "activity.log")

	argv := []string{"sh", "-c", "pwd"}
	if err := (ProcessLauncher{}).Launch(argv, dir, activityFile); err != nil {
		t.Fatalf("Launch() error: %v", err)
	}

	var contents []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(activityFile)
		if err == nil && len(b) > 0 {
			contents = b
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := string(contents)
	if !strings.Contains(got, dir) {
		t.Errorf("Activity file = %q, want it to contain pwd %q", got, dir)
	}
}
