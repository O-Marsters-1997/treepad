package treepad

import (
	"context"
	"fmt"

	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

type ConfigInitInput struct {
	Global    bool
	Inherit   bool
	HooksOnly bool
	Force     bool
}

func ConfigInit(ctx context.Context, d deps.Deps, in ConfigInitInput) error {
	opts := config.InitOptions{
		Global:    in.Global,
		Inherit:   in.Inherit,
		HooksOnly: in.HooksOnly,
		Force:     in.Force,
	}

	if in.Global {
		path, err := config.WriteDefault("", opts)
		if err != nil {
			return err
		}
		d.Log.OK("wrote config to %s", path)
		return nil
	}

	rc, err := repo.Load(ctx, d.Runner, "")
	if err != nil {
		return err
	}
	path, err := config.WriteDefault(rc.Main.Path, opts)
	if err != nil {
		return err
	}
	d.Log.OK("wrote config to %s", path)

	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return err
	}
	data := hook.Data{
		Branch:       rc.Main.Branch,
		WorktreePath: rc.Main.Path,
		Slug:         rc.Slug,
		OutputDir:    rc.OutputDir,
	}
	if err := hook.Run(ctx, d.HookRunner, cfg.Hooks, hook.PostConfigInit, data); err != nil {
		d.Log.Warn("%s", &hook.PostErr{Event: hook.PostConfigInit, Err: err})
	}
	return nil
}

type ConfigShowInput struct{}

func ConfigShow(ctx context.Context, d deps.Deps, _ ConfigShowInput) error {
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
