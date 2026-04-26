// Package passthrough executes a command in a target directory with stdio
// inherited from the calling process. Used by exec, diff, and from-spec verbs
// to hand off interactive subprocesses (agents, git-diff pagers, etc.).
package passthrough

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"

	"golang.org/x/term"

	"treepad/internal/tty"
)

// Runner executes a command in dir with stdio inherited from the calling
// process. Returns the child's exit code (non-zero does not produce an error;
// a non-nil error indicates a launch failure such as binary not found).
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (int, error)
}

// OpenTTY is the function used to acquire a controlling terminal for
// interactive subprocesses. Overridable in tests.
var OpenTTY = tty.Open

// StdioIsTTY reports whether all three standard streams are interactive
// terminals. When true, the child inherits them directly — required for
// Bun-compiled agents (e.g. Claude Code) which reject /dev/tty-opened fds
// when constructing node:tty WriteStreams on macOS (kqueue EINVAL), but work
// fine with directly-inherited terminal fds. Overridable in tests.
var StdioIsTTY = func() bool {
	return term.IsTerminal(0) && term.IsTerminal(1) && term.IsTerminal(2)
}

// OSRunner is the production implementation of Runner using os/exec.
type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, dir, name string, args ...string) (int, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	switch {
	case StdioIsTTY():
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	default:
		if tty := OpenTTY(); tty != nil {
			defer tty.Close() //nolint:errcheck
			cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
		} else {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		}
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*osexec.ExitError](err); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
