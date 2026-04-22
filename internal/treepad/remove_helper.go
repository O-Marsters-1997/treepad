package treepad

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/slug"
	"treepad/internal/worktree"
)

// removeWorktreeAndArtifact removes a git worktree, its generated artifact file,
// and the associated branch. When force is true, passes --force to git worktree remove
// and -D (instead of -d) to git branch.
func removeWorktreeAndArtifact(
	ctx context.Context, d Deps,
	target, main worktree.Worktree,
	outputDir string, force bool,
) error {
	removeArgs := []string{"worktree", "remove", target.Path}
	removeVerb := "git worktree remove"
	branchFlag := "-d"
	branchVerb := "git branch -d"
	if force {
		removeArgs = []string{"worktree", "remove", "--force", target.Path}
		removeVerb = "git worktree remove --force"
		branchFlag = "-D"
		branchVerb = "git branch -D"
	}

	cfg, err := config.Load(main.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(main.Path))
	hData := hookData(repoSlug, target.Branch, target.Path, outputDir)
	if err := runHook(ctx, d, cfg.Hooks, hook.PreRemove, hData); err != nil {
		return fmt.Errorf("pre_remove hook: %w", err)
	}

	if _, err := d.Runner.Run(ctx, "git", removeArgs...); err != nil {
		return fmt.Errorf("%s: %w", removeVerb, err)
	}
	d.Log.OK("removed worktree: %s", target.Path)

	artData := templateData(repoSlug, target.Branch, target.Path, outputDir)
	artifactPath, ok, err := artifact.Path(artifactSpec(cfg.Artifact), outputDir, artData)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact: %w", err)
		}
		d.Log.OK("removed artifact: %s", artifactPath)
	}

	if _, err := d.Runner.Run(ctx, "git", "branch", branchFlag, target.Branch); err != nil {
		return fmt.Errorf("%s: %w", branchVerb, err)
	}
	d.Log.OK("deleted branch: %s", target.Branch)

	if err := runHook(ctx, d, cfg.Hooks, hook.PostRemove, hData); err != nil {
		d.Log.Warn("post_remove hook failed: %v", err)
	}

	return nil
}
