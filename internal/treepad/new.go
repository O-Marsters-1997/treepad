package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/slug"
)

type NewInput struct {
	Branch    string
	Base      string
	Open      bool
	Current   bool
	OutputDir string
}

func New(ctx context.Context, d Deps, in NewInput) error {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	worktreePath := filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(in.Branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	hookCfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	hData := hookData(rc.Slug, in.Branch, worktreePath, rc.OutputDir)
	if err := runHook(ctx, d, hookCfg.Hooks, hook.PreNew, hData); err != nil {
		return fmt.Errorf("pre_new hook: %w", err)
	}

	if _, err := d.Runner.Run(ctx, "git", "worktree", "add", "-b", in.Branch, worktreePath, in.Base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	d.Log.OK("created worktree at %s", worktreePath)

	cfg, err := loadAndSync(ctx, d, rc.Main.Path, nil, []syncTarget{{path: worktreePath, branch: in.Branch}}, rc.Slug, rc.OutputDir)
	if err != nil {
		return err
	}

	artData := templateData(rc.Slug, in.Branch, worktreePath, rc.OutputDir)
	artifactPath, err := artifact.Write(artifactSpec(cfg.Artifact), rc.OutputDir, artData)
	if err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	slog.Debug("wrote artifact", "outputDir", rc.OutputDir, "branch", in.Branch)

	if err := runHook(ctx, d, cfg.Hooks, hook.PostNew, hData); err != nil {
		d.Log.Warn("post_new hook failed: %v", err)
	}

	if in.Open {
		openPath := worktreePath
		if artifactPath != "" {
			openPath = artifactPath
		}
		d.Log.Step("opening...")
		openSpec := artifact.OpenSpec{Command: cfg.Open.Command}
		openData := artifact.OpenData{
			ArtifactPath: openPath,
			Worktree:     artifact.ToWorktree(in.Branch, worktreePath, rc.OutputDir),
		}
		if err := d.Opener.Open(ctx, openSpec, openData); err != nil {
			return fmt.Errorf("open: %w", err)
		}
	}
	if !in.Current {
		emitCD(d, worktreePath)
	}
	return nil
}
