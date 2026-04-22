package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"treepad/internal/artifact"
	"treepad/internal/slug"
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

func Generate(ctx context.Context, d Deps, in GenerateInput) error {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
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

	repoSlug := slug.Slug(filepath.Base(sourceDir))

	outputDir, err := resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}
	slog.Debug("output directory", "dir", outputDir, "explicit", in.OutputDir != "")

	var targets []syncTarget
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		targets = append(targets, syncTarget{path: wt.Path, branch: wt.Branch})
	}

	if in.Branch != "" {
		var matched []syncTarget
		for _, t := range targets {
			if t.branch == in.Branch {
				matched = append(matched, t)
				break
			}
		}
		if len(matched) == 0 {
			return fmt.Errorf("no worktree found for branch %q", in.Branch)
		}
		targets = matched
	}

	cfg, err := loadAndSync(ctx, d, sourceDir, in.ExtraPatterns, targets, repoSlug, outputDir)
	if err != nil {
		return err
	}

	if !in.SyncOnly {
		d.Log.Step("generating artifact files → %s", outputDir)
		for _, wt := range worktrees {
			if in.Branch != "" && wt.Branch != in.Branch {
				continue
			}
			data := templateData(repoSlug, wt.Branch, wt.Path, outputDir)
			path, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
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
