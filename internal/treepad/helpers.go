package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
)

type syncTarget struct {
	path   string
	branch string
}

// loadAndSync loads config from sourceDir, syncs it to targets, and returns the config.
// Pass a nil targets slice to skip syncing and only load the config.
func loadAndSync(ctx context.Context, d Deps, sourceDir string, extraPatterns []string, targets []syncTarget, repoSlug, outputDir string) (config.Config, error) {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if len(targets) == 0 {
		return cfg, nil
	}

	patterns := slices.Concat(cfg.Sync.Include, extraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	d.Log.Step("syncing configs to worktrees...")
	for _, t := range targets {
		d.Log.Info("→ %s (%s)", t.branch, t.path)
		hData := hookData(repoSlug, t.branch, t.path, outputDir)
		if err := runHook(ctx, d, cfg.Hooks, hook.PreSync, hData); err != nil {
			return config.Config{}, fmt.Errorf("pre_sync hook: %w", err)
		}
		if err := d.Syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: t.path,
		}); err != nil {
			return config.Config{}, fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
		if err := runHook(ctx, d, cfg.Hooks, hook.PostSync, hData); err != nil {
			d.Log.Warn("post_sync hook failed: %v", err)
		}
	}
	return cfg, nil
}

// hookData constructs a hook.Data value from operation context.
// The HookType field is set by runHook when the event is known.
func hookData(slug, branch, wtPath, outputDir string) hook.Data {
	return hook.Data{
		Branch:       branch,
		WorktreePath: wtPath,
		Slug:         slug,
		OutputDir:    outputDir,
	}
}

// runHook fires the hooks for the given event. It is a no-op when no hooks are
// configured for the event. Callers control the failure semantics: pre-hooks
// should return the error to abort; post-hooks should log and continue.
func runHook(ctx context.Context, d Deps, cfg hook.Config, event hook.Event, data hook.Data) error {
	entries := cfg.For(event)
	if len(entries) == 0 {
		return nil
	}
	data.HookType = string(event)
	return d.HookRunner.Run(ctx, entries, data)
}

func artifactSpec(c config.ArtifactConfig) artifact.Spec {
	return artifact.Spec{
		FilenameTemplate: c.FilenameTemplate,
		ContentTemplate:  c.ContentTemplate,
	}
}

func templateData(repoSlug, branch, worktreePath, outputDir string) artifact.TemplateData {
	wt := artifact.ToWorktree(branch, worktreePath, outputDir)
	return artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    wt.Name,
		Worktrees: []artifact.Worktree{wt},
		OutputDir: outputDir,
	}
}

func emitCD(d Deps, path string) {
	_, _ = fmt.Fprintf(d.Out, "__TREEPAD_CD__\t%s\n", path)
}
