package treepad

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"treepad/internal/config"
	tpexec "treepad/internal/exec"
	"treepad/internal/passthrough"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

// PassthroughRunner is an alias for passthrough.Runner kept for existing callers.
type PassthroughRunner = passthrough.Runner

// ExecInput parameterises a tp exec invocation.
type ExecInput struct {
	Branch  string
	Command string
	Args    []string
	// Cwd overrides os.Getwd for the same-worktree warning. Empty means use os.Getwd.
	Cwd string
	// Runner overrides the default passthrough.OSRunner for testing.
	Runner PassthroughRunner
}

// Exec runs a command in the named worktree, routing through the detected task
// runner when the command matches a known script. Returns the child process exit
// code (non-zero does not produce an error).
func Exec(ctx context.Context, d deps.Deps, in ExecInput) (int, error) {
	worktrees, err := repo.ListWorktrees(ctx, d.Runner)
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
		pt = passthrough.OSRunner{}
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
