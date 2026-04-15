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

func TestExec_scriptDispatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("build:\n  go build ./...\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{}
	var out bytes.Buffer
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &out, strings.NewReader(""))

	exitCode, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "feat",
		Command: "build",
		Args:    []string{"--verbose"},
		Cwd:     "/some/other",
		Runner:  pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", exitCode)
	}
	if len(pt.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(pt.calls))
	}
	call := pt.calls[0]
	if call.name != "just" {
		t.Errorf("name = %q, want %q", call.name, "just")
	}
	wantArgs := []string{"build", "--verbose"}
	if !equalStringSlice(call.args, wantArgs) {
		t.Errorf("args = %v, want %v", call.args, wantArgs)
	}
	if call.dir != dir {
		t.Errorf("dir = %q, want %q", call.dir, dir)
	}
}

func TestExec_rawFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("build:\n  go build ./...\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{}
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &bytes.Buffer{}, strings.NewReader(""))

	_, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "feat",
		Command: "ls",
		Args:    []string{"-la"},
		Cwd:     "/some/other",
		Runner:  pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pt.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(pt.calls))
	}
	call := pt.calls[0]
	if call.name != "ls" {
		t.Errorf("name = %q, want %q", call.name, "ls")
	}
	wantArgs := []string{"-la"}
	if !equalStringSlice(call.args, wantArgs) {
		t.Errorf("args = %v, want %v", call.args, wantArgs)
	}
}

func TestExec_npmArgs(t *testing.T) {
	tests := []struct {
		name        string
		packageJSON string
		command     string
		args        []string
		wantArgs    []string
	}{
		{
			name:        "double dash injected when args present",
			packageJSON: `{"scripts":{"test":"jest"}}`,
			command:     "test",
			args:        []string{"--watch"},
			wantArgs:    []string{"run", "test", "--", "--watch"},
		},
		{
			name:        "no double dash when no args",
			packageJSON: `{"scripts":{"build":"tsc"}}`,
			command:     "build",
			args:        nil,
			wantArgs:    []string{"run", "build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.packageJSON), 0o644); err != nil {
				t.Fatalf("write package.json: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644); err != nil {
				t.Fatalf("write package-lock.json: %v", err)
			}

			porcelain := worktreePorcelainWithPath("feat", dir)
			pt := &fakePassthroughRunner{}
			svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &bytes.Buffer{}, strings.NewReader(""))

			_, err := svc.Exec(context.Background(), ExecInput{
				Branch:  "feat",
				Command: tt.command,
				Args:    tt.args,
				Cwd:     "/some/other",
				Runner:  pt,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pt.calls) == 0 {
				t.Fatal("expected a call")
			}
			if !equalStringSlice(pt.calls[0].args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", pt.calls[0].args, tt.wantArgs)
			}
		})
	}
}

func TestExec_zeroArgListsScripts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("build:\n  go build\ntest:\n  go test\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{}
	var out bytes.Buffer
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &out, strings.NewReader(""))

	exitCode, err := svc.Exec(context.Background(), ExecInput{
		Branch: "feat",
		Cwd:    "/some/other",
		Runner: pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", exitCode)
	}
	if len(pt.calls) != 0 {
		t.Errorf("expected no exec calls, got %d", len(pt.calls))
	}
	outStr := out.String()
	if !strings.Contains(outStr, "Runner: just") {
		t.Errorf("output missing runner name: %q", outStr)
	}
	if !strings.Contains(outStr, "build") || !strings.Contains(outStr, "test") {
		t.Errorf("output missing scripts: %q", outStr)
	}
}

func TestExec_sameWorktreeWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("build:\n  go build\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{}
	var out bytes.Buffer
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &out, strings.NewReader(""))

	_, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "feat",
		Command: "build",
		Cwd:     dir, // same as worktree path
		Runner:  pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "warning") {
		t.Errorf("expected same-worktree warning, got: %q", out.String())
	}
}

func TestExec_exitCodePropagated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("fail:\n  exit 1\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{exitCode: 42}
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &bytes.Buffer{}, strings.NewReader(""))

	exitCode, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "feat",
		Command: "ls", // raw exec, exitCode still comes from fake
		Cwd:     "/some/other",
		Runner:  pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", exitCode)
	}
}

func TestExec_configOverrideRunner(t *testing.T) {
	dir := t.TempDir()
	// Has package.json but .treepad.toml says runner=just
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"start":"node"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte("build:\n  go build\n"), 0o644); err != nil {
		t.Fatalf("write justfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".treepad.toml"), []byte("[exec]\nrunner = \"just\"\n"), 0o644); err != nil {
		t.Fatalf("write .treepad.toml: %v", err)
	}

	porcelain := worktreePorcelainWithPath("feat", dir)
	pt := &fakePassthroughRunner{}
	svc := NewService(fakeRunner{output: porcelain}, &fakeSyncer{}, nil, &fakeHookRunner{}, &bytes.Buffer{}, strings.NewReader(""))

	_, err := svc.Exec(context.Background(), ExecInput{
		Branch:  "feat",
		Command: "build",
		Cwd:     "/some/other",
		Runner:  pt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pt.calls) == 0 {
		t.Fatal("expected a call")
	}
	if pt.calls[0].name != "just" {
		t.Errorf("name = %q, want %q", pt.calls[0].name, "just")
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
