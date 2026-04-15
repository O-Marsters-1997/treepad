package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"

	"treepad/internal/config"
	tpexec "treepad/internal/exec"
	"treepad/internal/worktree"
)

// PassthroughRunner executes a command in dir with stdio inherited from the
// calling process. Returns the child's exit code (non-zero does not produce an
// error; a non-nil error indicates a launch failure such as binary not found).
type PassthroughRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (exitCode int, err error)
}

// OSPassthroughRunner is the real PassthroughRunner that uses os/exec.
type OSPassthroughRunner struct{}

func (OSPassthroughRunner) Run(ctx context.Context, dir, name string, args ...string) (int, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// ExecInput parameterises a tp exec invocation.
type ExecInput struct {
	Branch  string
	Command string
	Args    []string
	// Cwd overrides os.Getwd for the same-worktree warning. Empty means use os.Getwd.
	Cwd string
	// Runner overrides OSPassthroughRunner for testing.
	Runner PassthroughRunner
}

// ExecResult carries the child process exit code.
type ExecResult struct {
	ExitCode int
}

// Exec runs a command in the named worktree, routing through the detected task
// runner when the command matches a known script.
func (s *Service) Exec(ctx context.Context, in ExecInput) (ExecResult, error) {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return ExecResult{}, err
	}

	wt, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return ExecResult{}, fmt.Errorf("no worktree found for branch %q; run `tp workspace` to list worktrees", in.Branch)
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return ExecResult{}, fmt.Errorf("get current directory: %w", err)
		}
	}
	if filepath.Clean(wt.Path) == filepath.Clean(cwd) {
		_, _ = fmt.Fprintln(s.out, "warning: already in this worktree; consider invoking the runner directly")
	}

	cfg, err := config.Load(wt.Path)
	if err != nil {
		return ExecResult{}, fmt.Errorf("load config: %w", err)
	}

	runner, err := tpexec.Detect(wt.Path, cfg.Exec.Runner)
	if err != nil {
		return ExecResult{}, err
	}

	if in.Command == "" {
		s.printScripts(runner)
		return ExecResult{}, nil
	}

	name, args := s.buildCommand(runner, in.Command, in.Args)

	pt := in.Runner
	if pt == nil {
		pt = OSPassthroughRunner{}
	}
	exitCode, err := pt.Run(ctx, wt.Path, name, args...)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{ExitCode: exitCode}, nil
}

func (s *Service) printScripts(runner tpexec.Runner) {
	_, _ = fmt.Fprintf(s.out, "Runner: %s\n", runner.Name)
	if len(runner.Scripts) == 0 {
		_, _ = fmt.Fprintln(s.out, "Scripts: (none)")
		return
	}
	_, _ = fmt.Fprintln(s.out, "Scripts:")
	for _, sc := range runner.Scripts {
		_, _ = fmt.Fprintf(s.out, "  %s\n", sc)
	}
}

// buildCommand returns the executable name and arguments for the given command,
// routing through the runner when the command matches an enumerated script.
func (s *Service) buildCommand(runner tpexec.Runner, command string, extraArgs []string) (string, []string) {
	scriptSet := make(map[string]bool, len(runner.Scripts))
	for _, sc := range runner.Scripts {
		scriptSet[sc] = true
	}

	if !scriptSet[command] {
		return command, extraArgs
	}

	// Build: <ScriptCmd...> <command> [-- for npm] <extraArgs...>
	full := append(append([]string{}, runner.ScriptCmd...), command)
	if runner.Name == "npm" && len(extraArgs) > 0 {
		full = append(full, "--")
	}
	full = append(full, extraArgs...)

	return full[0], full[1:]
}
