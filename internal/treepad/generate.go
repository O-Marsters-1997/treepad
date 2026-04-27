package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/slug"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/lifecycle"
	"treepad/internal/treepad/repo"
)

type GenerateInput struct {
	UseCurrentDir bool
	SourcePath    string
	SyncOnly      bool
	OutputDir     string
	ExtraPatterns []string
	// Branch restricts the sync and artifact generation to a single worktree.
	// Empty means fleet-wide (existing behaviour).
	Branch string
}

func Generate(ctx context.Context, d deps.Deps, in GenerateInput) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	worktrees, err := repo.ListWorktrees(ctx, d.Runner)
	if err != nil {
		return err
	}

	sourceDir, err := ResolveSourceDir(in.UseCurrentDir, in.SourcePath, cwd, worktrees)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	slog.Debug("resolved source directory",
		"sourceDir", sourceDir,
		"useCurrentDir", in.UseCurrentDir,
		"sourcePath", in.SourcePath,
	)
	d.Log.Info("using config source: %s", sourceDir)

	// Generate uses sourceDir (not main.Path) as the slug base because --current
	// may point to a non-main worktree.
	repoSlug := slug.Slug(filepath.Base(sourceDir))
	outputDir, err := repo.ResolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}
	slog.Debug("output directory", "dir", outputDir, "explicit", in.OutputDir != "")

	var targets []lifecycle.SyncTarget
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		targets = append(targets, lifecycle.SyncTarget{Path: wt.Path, Branch: wt.Branch})
	}

	if in.Branch != "" {
		var matched []lifecycle.SyncTarget
		for _, t := range targets {
			if t.Branch == in.Branch {
				matched = append(matched, t)
				break
			}
		}
		if len(matched) == 0 {
			return fmt.Errorf("no worktree found for branch %q", in.Branch)
		}
		targets = matched
	}

	cfg, err := lifecycle.LoadAndSync(
		ctx,
		deps.Deps{
			Runner:     d.Runner,
			Syncer:     d.Syncer,
			Opener:     d.Opener,
			HookRunner: d.HookRunner,
			Profiler:   d.Profiler,
			Log:        d.Log,
			In:         d.In,
		}, sourceDir, nil, in.ExtraPatterns, targets, repoSlug, outputDir)
	if err != nil {
		return err
	}

	if !in.SyncOnly {
		d.Log.Step("generating artifact files → %s", outputDir)
		for _, wt := range worktrees {
			if in.Branch != "" && wt.Branch != in.Branch {
				continue
			}
			data := config.MakeTemplateData(repoSlug, wt.Branch, wt.Path, outputDir)
			path, err := artifact.Write(cfg.Artifact.Spec(), outputDir, data)
			if err != nil {
				return fmt.Errorf("write artifact for %s: %w", wt.Branch, err)
			}
			if path != "" {
				d.Log.OK("created %s", filepath.Base(path))
			}
		}
	}

	if in.SyncOnly {
		d.Log.OK("done: config sync complete")
	} else {
		d.Log.OK("done: artifact files generated and configs synced")
	}
	return nil
}
