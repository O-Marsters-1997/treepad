// Package cdshell owns the __TREEPAD_CD__ shell-bridge protocol.
//
// The tp shell wrapper sets TREEPAD_CD_FD=3 and redirects fd 3 into its
// $(...) capture, letting tp's stdout go to the real terminal. When the env
// var is absent (stale wrapper, direct binary invocation, tests) EmitCD falls
// back to writing the "__TREEPAD_CD__\t<path>\n" sentinel to d.Out.
package cdshell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

// Deps are the external dependencies cdshell needs.
type Deps struct {
	Out        io.Writer
	Log        Logger
	IsTerminal func(w io.Writer) bool
	// CDSentinel, when non-nil, returns the writer EmitCD uses instead of
	// the fd-3 probe. Tests set this to a bytes.Buffer.
	CDSentinel func() io.Writer
	Runner     worktree.CommandRunner
}

// Logger is the logging interface cdshell requires from its caller.
type Logger interface {
	Warn(format string, args ...any)
}

// EmitCD writes the __TREEPAD_CD__ sentinel for the path.
// In the fallback path (no fd-3 sentinel), it also writes a human-readable line.
func EmitCD(d Deps, path string) {
	if w := sentinelWriter(d); w != nil {
		_, _ = io.WriteString(w, path+"\n")
		return
	}
	if d.Out == nil {
		return
	}
	_, _ = fmt.Fprintf(d.Out, "__TREEPAD_CD__\t%s\n", path)
	_, _ = fmt.Fprintf(d.Out, "→ cd: %s\n", path)
}

// sentinelWriter returns the writer for the cd sentinel.
func sentinelWriter(d Deps) io.Writer {
	if d.CDSentinel != nil {
		return d.CDSentinel()
	}
	fdStr := os.Getenv("TREEPAD_CD_FD")
	if fdStr == "" {
		return nil
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil || fd < 0 {
		return nil
	}
	return os.NewFile(uintptr(fd), "treepad-cd")
}

// MaybeWarnStaleWrapper prints a one-line stderr hint when an agent_command is
// configured but the new shell wrapper has not been installed.
func MaybeWarnStaleWrapper(d Deps, hasAgentCommand bool) {
	if !hasAgentCommand {
		return
	}
	if os.Getenv("TREEPAD_CD_FD") != "" {
		return
	}
	if d.IsTerminal(d.Out) {
		return
	}
	d.Log.Warn("stale shell wrapper detected — re-run: eval \"$(tp shell-init)\"")
	d.Log.Warn("Your agent will still start interactively via /dev/tty.")
}

// CDInput parameterises a tp cd invocation.
type CDInput struct {
	Branch string
}

// CD emits a cd sentinel for the given branch's worktree path.
func CD(ctx context.Context, d Deps, in CDInput) error {
	worktrees, err := repo.ListWorktrees(ctx, d.Runner)
	if err != nil {
		return err
	}

	wt, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q; create one with: tp new %s", in.Branch, in.Branch)
	}

	EmitCD(d, wt.Path)
	return nil
}

// BaseInput parameterises a tp base invocation.
type BaseInput struct {
	// Cwd overrides os.Getwd for testing.
	Cwd string
}

// Base emits a cd sentinel for the main worktree.
func Base(ctx context.Context, d Deps, in BaseInput) error {
	worktrees, err := repo.ListWorktrees(ctx, d.Runner)
	if err != nil {
		return err
	}

	main, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	cwd := in.Cwd
	if cwd == "" {
		var cwdErr error
		cwd, cwdErr = os.Getwd()
		if cwdErr != nil {
			return fmt.Errorf("get current directory: %w", cwdErr)
		}
	}

	if filepath.Clean(cwd) == filepath.Clean(main.Path) {
		return errors.New("already on the default worktree")
	}

	EmitCD(d, main.Path)
	return nil
}
