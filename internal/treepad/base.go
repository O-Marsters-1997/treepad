package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"treepad/internal/worktree"
)

type BaseInput struct {
	// Cwd overrides os.Getwd for testing.
	Cwd string
}

func Base(ctx context.Context, d Deps, in BaseInput) error {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return err
	}

	main, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}

	if filepath.Clean(cwd) == filepath.Clean(main.Path) {
		return errors.New("already on the default worktree")
	}

	emitCD(d, main.Path)
	return nil
}
