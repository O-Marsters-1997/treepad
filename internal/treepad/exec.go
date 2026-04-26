package treepad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"

	"golang.org/x/term"

	"treepad/internal/config"
	tpexec "treepad/internal/exec"
	"treepad/internal/tty"
	"treepad/internal/worktree"
)

// PassthroughRunner executes a command in dir with stdio inherited from the
// calling process. Returns the child's exit code (non-zero does not produce an
// error; a non-nil error indicates a launch failure such as binary not found).
type PassthroughRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (int, error)
}

// openTTY is the function used to acquire a controlling terminal for
// interactive subprocesses. Overridable in tests.
var openTTY = tty.Open

// stdioIsTTY reports whether all three standard streams are interactive
// terminals. When true, the child inherits them directly — required for
// Bun-compiled agents (e.g. Claude Code) which reject /dev/tty-opened fds
// when constructing node:tty WriteStreams on macOS (kqueue EINVAL), but work
// fine with directly-inherited terminal fds. Overridable in tests.
var stdioIsTTY = func() bool {
	return term.IsTerminal(0) && term.IsTerminal(1) && term.IsTerminal(2)
}

type osPassthroughRunner struct{}

func (osPassthroughRunner) Run(ctx context.Context, dir, name string, args ...string) (int, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	switch {
	case stdioIsTTY():
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	default:
		if tty := openTTY(); tty != nil {
			defer tty.Close() //nolint:errcheck
			cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
		} else {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		}
	}
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

// Exec runs a command in the named worktree, routing through the detected task
// runner when the command matches a known script. Returns the child process exit
// code (non-zero does not produce an error).
func Exec(ctx context.Context, d Deps, in ExecInput) (int, error) {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return 0, err
	}

	wt, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return 0, fmt.Errorf("no worktree found for branch %q; run `tp sync` to list worktrees", in.Branch)
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return 0, fmt.Errorf("get current directory: %w", err)
		}
	}
	if filepath.Clean(wt.Path) == filepath.Clean(cwd) {
		d.Log.Warn("already in this worktree; consider invoking the runner directly")
	}

	cfg, err := config.Load(wt.Path)
	if err != nil {
		return 0, fmt.Errorf("load config: %w", err)
	}

	runner, err := tpexec.Resolve(wt.Path, cfg.Exec.Runner)
	if err != nil {
		return 0, err
	}

	if in.Command == "" {
		printScripts(d.Out, runner)
		return 0, nil
	}

	name, args := buildCommand(runner, in.Command, in.Args)

	pt := in.Runner
	if pt == nil {
		pt = osPassthroughRunner{}
	}
	return pt.Run(ctx, wt.Path, name, args...)
}

func printScripts(out io.Writer, runner tpexec.Runner) {
	_, _ = fmt.Fprintf(out, "Runner: %s\n", runner.Name)
	if len(runner.Scripts) == 0 {
		_, _ = fmt.Fprintln(out, "Scripts: (none)")
		return
	}
	_, _ = fmt.Fprintln(out, "Scripts:")
	for _, sc := range runner.Scripts {
		_, _ = fmt.Fprintf(out, "  %s\n", sc)
	}
}

func buildCommand(runner tpexec.Runner, command string, extraArgs []string) (string, []string) {
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
