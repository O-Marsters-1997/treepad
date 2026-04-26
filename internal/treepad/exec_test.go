package treepad

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/passthrough"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
	"treepad/internal/ui"
)

func TestExec_unknownBranch(t *testing.T) {
	d := deps.Deps{
		Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.TwoWorktreePorcelain}}},
		Syncer: &treepadtest.FakeSyncer{},
		Out:    &bytes.Buffer{},
		In:     strings.NewReader(""),
	}
	_, err := Exec(context.Background(), d, ExecInput{
		Branch:  "nonexistent",
		Command: "build",
		Cwd:     "/some/other",
		Runner:  &treepadtest.FakePassthroughRunner{},
	})
	if err == nil {
		t.Fatal("expected error for unknown branch")
	}
}

func TestExec_dispatch(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string // filename → content written to worktree dir
		command      string
		args         []string
		cwdSame      bool // true: set Cwd = worktree dir (triggers same-worktree warning)
		fakeExitCode int
		wantExitCode int
		wantCallName string   // "" means no exec call expected
		wantCallArgs []string // checked only when wantCallName is set
		wantOutput   []string // substrings expected in stdout
	}{
		{
			name:         "script routed through just",
			files:        map[string]string{"justfile": "build:\n  go build ./...\n"},
			command:      "build",
			args:         []string{"--verbose"},
			wantCallName: "just",
			wantCallArgs: []string{"build", "--verbose"},
		},
		{
			name:         "unknown command falls back to raw exec",
			files:        map[string]string{"justfile": "build:\n  go build ./...\n"},
			command:      "ls",
			args:         []string{"-la"},
			wantCallName: "ls",
			wantCallArgs: []string{"-la"},
		},
		{
			name:         "exit code propagated from child process",
			files:        map[string]string{"justfile": "fail:\n  exit 1\n"},
			command:      "ls",
			fakeExitCode: 42,
			wantExitCode: 42,
			wantCallName: "ls",
		},
		{
			name: "config override selects runner when multiple present",
			files: map[string]string{
				"package.json":  `{"scripts":{"start":"node"}}`,
				"justfile":      "build:\n  go build\n",
				".treepad.toml": "[exec]\nrunner = \"just\"\n",
			},
			command:      "build",
			wantCallName: "just",
			wantCallArgs: []string{"build"},
		},
		{
			name:         "same worktree emits warning but still runs",
			files:        map[string]string{"justfile": "build:\n  go build\n"},
			command:      "build",
			cwdSame:      true,
			wantCallName: "just",
			wantCallArgs: []string{"build"},
			wantOutput:   []string{"already in this worktree"},
		},
		{
			name:       "no command lists detected runner and scripts",
			files:      map[string]string{"justfile": "build:\n  go build\ntest:\n  go test\n"},
			command:    "",
			wantOutput: []string{"Runner: just", "build", "test"},
		},
		{
			name:         "npm: double dash injected before extra args",
			files:        map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "package-lock.json": "{}"},
			command:      "test",
			args:         []string{"--watch"},
			wantCallName: "npm",
			wantCallArgs: []string{"run", "test", "--", "--watch"},
		},
		{
			name:         "npm: no double dash when no extra args",
			files:        map[string]string{"package.json": `{"scripts":{"build":"tsc"}}`, "package-lock.json": "{}"},
			command:      "build",
			wantCallName: "npm",
			wantCallArgs: []string{"run", "build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			cwd := "/some/other"
			if tt.cwdSame {
				cwd = dir
			}

			pt := &treepadtest.FakePassthroughRunner{ExitCode: tt.fakeExitCode}
			var out bytes.Buffer
			porcelain := treepadtest.WorktreePorcelainWithPath("feat", dir)
			d := deps.Deps{
				Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: porcelain}}},
				Syncer: &treepadtest.FakeSyncer{},
				Out:    &out,
				Log:    ui.New(&out),
				In:     strings.NewReader(""),
			}

			exitCode, err := Exec(context.Background(), d, ExecInput{
				Branch:  "feat",
				Command: tt.command,
				Args:    tt.args,
				Cwd:     cwd,
				Runner:  pt,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exitCode != tt.wantExitCode {
				t.Errorf("exit code = %d, want %d", exitCode, tt.wantExitCode)
			}

			if tt.wantCallName == "" {
				if len(pt.Calls) != 0 {
					t.Errorf("expected no exec calls, got %d", len(pt.Calls))
				}
			} else {
				if len(pt.Calls) == 0 {
					t.Fatalf("expected an exec call, got none")
				}
			}

			outStr := out.String()
			for _, want := range tt.wantOutput {
				if !strings.Contains(outStr, want) {
					t.Errorf("output missing %q: got %q", want, outStr)
				}
			}
		})
	}
}

func TestOsPassthroughRunner_NoTTY_Fallback(t *testing.T) {
	orig := passthrough.OpenTTY
	defer func() { passthrough.OpenTTY = orig }()
	passthrough.OpenTTY = func() *os.File { return nil }

	code, err := passthrough.OSRunner{}.Run(context.Background(), t.TempDir(), "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestOsPassthroughRunner_TTY_ChildInherits(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origTTY := passthrough.OpenTTY
	origIsTTY := passthrough.StdioIsTTY
	defer func() { passthrough.OpenTTY = origTTY; passthrough.StdioIsTTY = origIsTTY }()
	passthrough.StdioIsTTY = func() bool { return false } // force /dev/tty path regardless of test environment
	passthrough.OpenTTY = func() *os.File { return pw }

	code, runErr := passthrough.OSRunner{}.Run(context.Background(), t.TempDir(), "echo", "hello")
	// pw is closed inside Run via defer tty.Close(); ReadAll gets EOF.
	got, _ := io.ReadAll(pr)
	_ = pr.Close()
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(string(got), "hello") {
		t.Errorf("expected child stdout on tty fd; got %q", got)
	}
}

func TestOsPassthroughRunner_PrefersInheritedWhenStdioIsTTY(t *testing.T) {
	ttyOpened := false
	origTTY := passthrough.OpenTTY
	origIsTTY := passthrough.StdioIsTTY
	defer func() { passthrough.OpenTTY = origTTY; passthrough.StdioIsTTY = origIsTTY }()
	passthrough.StdioIsTTY = func() bool { return true }
	passthrough.OpenTTY = func() *os.File { ttyOpened = true; return nil }

	code, err := passthrough.OSRunner{}.Run(context.Background(), t.TempDir(), "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if ttyOpened {
		t.Error("openTTY was called; expected inherited stdio path to be taken")
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
