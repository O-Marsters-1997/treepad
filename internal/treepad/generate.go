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
	slog.Debug("resolved source directory", "sourceDir", sourceDir, "useCurrentDir", in.UseCurrentDir, "sourcePath", in.SourcePath)
	_, _ = fmt.Fprintf(d.Out, "using config source: %s\n", sourceDir)

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
	cfg, err := loadAndSync(d, sourceDir, in.ExtraPatterns, targets)
	if err != nil {
		return err
	}

	if !in.SyncOnly {
		_, _ = fmt.Fprintf(d.Out, "\ngenerating artifact files → %s\n", outputDir)
		for _, wt := range worktrees {
			data := templateData(repoSlug, wt.Branch, wt.Path, outputDir)
			path, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
			if err != nil {
				return fmt.Errorf("write artifact for %s: %w", wt.Branch, err)
			}
			if path != "" {
				_, _ = fmt.Fprintf(d.Out, "  created %s\n", filepath.Base(path))
			}
		}
	}

	if in.SyncOnly {
		_, _ = fmt.Fprintln(d.Out, "\ndone: config sync complete")
	} else {
		_, _ = fmt.Fprintln(d.Out, "\ndone: artifact files generated and configs synced")
	}
	return nil
}
