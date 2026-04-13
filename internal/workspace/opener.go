package workspace

import (
	"context"

	"treepad/internal/worktree"
)

// Opener opens a file path using the OS default application.
type Opener interface {
	Open(ctx context.Context, path string) error
}

// ExecOpener opens files via the macOS `open` command.
type ExecOpener struct {
	Runner worktree.CommandRunner
}

func (e ExecOpener) Open(ctx context.Context, path string) error {
	_, err := e.Runner.Run(ctx, "open", path)
	return err
}
