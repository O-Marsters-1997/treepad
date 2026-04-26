package treepad

import (
	"context"
	"fmt"

	"treepad/internal/config"
	"treepad/internal/worktree"
)

type ConfigInitInput struct {
	Global bool
}

func ConfigInit(ctx context.Context, d Deps, in ConfigInitInput) error {
	if in.Global {
		path, err := config.WriteDefault("", true)
		if err != nil {
			return err
		}
		d.Log.OK("wrote config to %s", path)
		return nil
	}

	wts, err := worktree.List(ctx, d.Runner)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}
	main, err := worktree.MainWorktree(wts)
	if err != nil {
		return err
	}
	path, err := config.WriteDefault(main.Path, false)
	if err != nil {
		return err
	}
	d.Log.OK("wrote config to %s", path)
	return nil
}

type ConfigShowInput struct{}

func ConfigShow(ctx context.Context, d Deps, _ ConfigShowInput) error {
	wts, err := worktree.List(ctx, d.Runner)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}
	main, err := worktree.MainWorktree(wts)
	if err != nil {
		return err
	}
	output, err := config.Show(main.Path)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(d.Out, output)
	return err
}
