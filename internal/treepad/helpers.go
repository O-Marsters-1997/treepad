package treepad

import (
	"fmt"
	"log/slog"
	"slices"

	"treepad/internal/artifact"
	"treepad/internal/config"
	internalsync "treepad/internal/sync"
)

type syncTarget struct {
	path   string
	branch string
}

// loadAndSync loads config from sourceDir, syncs it to targets, and returns the config.
// Pass a nil targets slice to skip syncing and only load the config.
func loadAndSync(d Deps, sourceDir string, extraPatterns []string, targets []syncTarget) (config.Config, error) {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if len(targets) == 0 {
		return cfg, nil
	}

	patterns := slices.Concat(cfg.Sync.Files, extraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	d.Log.Step("syncing configs to worktrees...")
	for _, t := range targets {
		d.Log.Info("→ %s (%s)", t.branch, t.path)
		if err := d.Syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: t.path,
		}); err != nil {
			return config.Config{}, fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
	}
	return cfg, nil
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
