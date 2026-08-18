package launcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestState(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		age  time.Duration // time since mtime; negative means no file
		want RunState
	}{
		{"no Activity file", -1, StatePending},
		{"just touched", 0, StateWorking},
		{"89s old, still within the window", 89 * time.Second, StateWorking},
		{"exactly 90s old", 90 * time.Second, StateWorking},
		{"91s old, past the window", 91 * time.Second, StateIdle},
		{"an hour old", time.Hour, StateIdle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			commonDir := t.TempDir()
			branch := "feat/eng-12"
			if tc.age >= 0 {
				path := ActivityPath(commonDir, branch)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				mtime := now.Add(-tc.age)
				if err := os.Chtimes(path, mtime, mtime); err != nil {
					t.Fatal(err)
				}
			}

			got, err := State(commonDir, branch, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("State() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNoStuckState(t *testing.T) {
	for _, s := range []RunState{StatePending, StateWorking, StateIdle} {
		if s == "stuck" {
			t.Fatalf("a %q state exists; the plan forbids it", s)
		}
	}
}

func TestExists(t *testing.T) {
	commonDir := t.TempDir()
	branch := "feat/eng-12"

	if Exists(commonDir, branch) {
		t.Error("Exists() = true before any Activity file is written")
	}

	path := ActivityPath(commonDir, branch)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if !Exists(commonDir, branch) {
		t.Error("Exists() = false after the Activity file was written")
	}
}
