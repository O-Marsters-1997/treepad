package treepad

import (
	"context"
	"fmt"

	"treepad/internal/hook"
)

func hookData(slug, branch, wtPath, outputDir string) hook.Data {
	return hook.Data{
		Branch:       branch,
		WorktreePath: wtPath,
		Slug:         slug,
		OutputDir:    outputDir,
	}
}

func runHook(ctx context.Context, d Deps, cfg hook.Config, event hook.Event, data hook.Data) error {
	entries := cfg.For(event)
	if len(entries) == 0 {
		return nil
	}
	data.HookType = string(event)
	return d.HookRunner.Run(ctx, entries, data)
}

// runHookSandwich runs pre → do → post, aborting on pre failure and
// warning-only on post failure.
func runHookSandwich(
	ctx context.Context, d Deps, cfg hook.Config, pre, post hook.Event, data hook.Data, do func() error,
) error {
	if err := runHook(ctx, d, cfg, pre, data); err != nil {
		return fmt.Errorf("%s hook: %w", pre, err)
	}
	if err := do(); err != nil {
		return err
	}
	if err := runHook(ctx, d, cfg, post, data); err != nil {
		d.Log.Warn("%s hook failed: %v", post, err)
	}
	return nil
}
