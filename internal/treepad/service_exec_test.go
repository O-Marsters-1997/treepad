package treepad

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePassthroughRunner records calls and returns a canned exit code.
type fakePassthroughRunner struct {
	calls    []ptCall
	exitCode int
	err      error
}

type ptCall struct {
	dir  string
	name string
	args []string
}

func (f *fakePassthroughRunner) Run(_ context.Context, dir, name string, args ...string) (int, error) {
	f.calls = append(f.calls, ptCall{dir: dir, name: name, args: args})
	return f.exitCode, f.err
}

// worktreePorcelainWithPath builds porcelain output with a controllable path.
func worktreePorcelainWithPath(branch, path string) []byte {
	return fmt.Appendf(nil, "worktree %s\nHEAD abc123\nbranch refs/heads/%s\n\n", path, branch)
}

func TestExec_unknownBranch(t *testing.T) {
	svc := NewService(fakeRunner{output: twoWorktreePorcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &bytes.Buffer{}, strings.NewReader(""))
	_, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "nonexistent",
		Command: "build",
		Cwd:     "/some/other",
		Runner:  &fakePassthroughRunner{},
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
			wantOutput:   []string{"warning"},
		},
		{
			name:    "no command lists detected runner and scripts",
			files:   map[string]string{"justfile": "build:\n  go build\ntest:\n  go test\n"},
			command: "",
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

			pt := &fakePassthroughRunner{exitCode: tt.fakeExitCode}
			var out bytes.Buffer
			porcelain := worktreePorcelainWithPath("feat", dir)
			svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &out, strings.NewReader(""))

			exitCode, err := svc.Exec(context.Background(), ExecInput{
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
				if len(pt.calls) != 0 {
					t.Errorf("expected no exec calls, got %d", len(pt.calls))
				}
			} else {
				if len(pt.calls) == 0 {
					t.Fatalf("expected an exec call, got none")
				}
				call := pt.calls[0]
				if call.name != tt.wantCallName {
					t.Errorf("call name = %q, want %q", call.name, tt.wantCallName)
				}
				if tt.wantCallArgs != nil && !equalStringSlice(call.args, tt.wantCallArgs) {
					t.Errorf("call args = %v, want %v", call.args, tt.wantCallArgs)
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
