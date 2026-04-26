package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
)

type createWorktreeResult struct {
	RC           RepoContext
	Cfg          config.Config
	WorktreePath string
	ArtifactPath string
}

func createWorktreeWithSync(ctx context.Context, d Deps, branch, base, outputDir string) (createWorktreeResult, error) {
	rc, err := loadRepoContext(ctx, d, outputDir)
	if err != nil {
		return createWorktreeResult{}, err
	}

	worktreePath := filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return createWorktreeResult{}, fmt.Errorf("load config: %w", err)
	}
	hData := hookData(rc.Slug, branch, worktreePath, rc.OutputDir)

	var artifactPath string
	if err := runHookSandwich(ctx, d, cfg.Hooks, hook.PreNew, hook.PostNew, hData, func() error {
		if _, err := d.Runner.Run(ctx, "git", "worktree", "add", "-b", branch, worktreePath, base); err != nil {
			return fmt.Errorf("git worktree add: %w", err)
		}
		d.Log.OK("created worktree at %s", worktreePath)

		var syncErr error
		cfg, syncErr = loadAndSync(ctx, d, rc.Main.Path, nil, []syncTarget{{path: worktreePath, branch: branch}}, rc.Slug, rc.OutputDir)
		if syncErr != nil {
			return syncErr
		}

		artData := templateData(rc.Slug, branch, worktreePath, rc.OutputDir)
		var artErr error
		artifactPath, artErr = artifact.Write(artifactSpec(cfg.Artifact), rc.OutputDir, artData)
		if artErr != nil {
			return fmt.Errorf("write artifact: %w", artErr)
		}
		slog.Debug("wrote artifact", "outputDir", rc.OutputDir, "branch", branch)
		return nil
	}); err != nil {
		return createWorktreeResult{}, err
	}

	return createWorktreeResult{
		RC:           rc,
		Cfg:          cfg,
		WorktreePath: worktreePath,
		ArtifactPath: artifactPath,
	}, nil
}

type syncTarget struct {
	path   string
	branch string
}

func openWorktree(ctx context.Context, d Deps, openCmd []string, branch, wtPath, artifactPath, outputDir string) error {
	openPath := wtPath
	if artifactPath != "" {
		openPath = artifactPath
	}
	spec := artifact.OpenSpec{Command: openCmd}
	data := artifact.OpenData{
		ArtifactPath: openPath,
		Worktree:     artifact.ToWorktree(branch, wtPath, outputDir),
	}
	return d.Opener.Open(ctx, spec, data)
}

func loadAndSync(
	ctx context.Context, d Deps, sourceDir string,
	extraPatterns []string, targets []syncTarget,
	repoSlug, outputDir string,
) (config.Config, error) {
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
		if err := runHookSandwich(ctx, d, cfg.Hooks, hook.PreSync, hook.PostSync, hData, func() error {
			if err := d.Syncer.Sync(patterns, internalsync.Config{
				SourceDir: sourceDir,
				TargetDir: t.path,
			}); err != nil {
				return fmt.Errorf("sync configs to %s: %w", t.branch, err)
			}
			slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
			return nil
		}); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}
