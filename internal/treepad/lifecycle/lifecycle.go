// Package lifecycle owns the worktree creation, removal, and pruning verbs.
package lifecycle

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/profile"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

// CreateResult holds the output of a successful worktree creation.
type CreateResult struct {
	RC           repo.Context
	Cfg          config.Config
	WorktreePath string
	ArtifactPath string
}

// CreateWorktreeWithSync creates a worktree, syncs configs, and writes the artifact.
func CreateWorktreeWithSync(ctx context.Context, d deps.Deps, branch, base, outputDir string) (CreateResult, error) {
	p := profile.OrDisabled(d.Profiler)

	repoLoadDone := p.Stage("repo.load")
	rc, err := repo.Load(ctx, d.Runner, outputDir)
	repoLoadDone()
	if err != nil {
		return CreateResult{}, err
	}

	worktreePath := filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	configLoadDone := p.Stage("config.load")
	cfg, err := config.Load(rc.Main.Path)
	configLoadDone()
	if err != nil {
		return CreateResult{}, fmt.Errorf("load config: %w", err)
	}
	hData := hook.Data{
		Branch:       branch,
		WorktreePath: worktreePath,
		Slug:         rc.Slug,
		OutputDir:    rc.OutputDir,
	}

	var artifactPath string
	postErr, err := hook.RunSandwich(ctx, p, d.HookRunner, cfg.Hooks, hook.PreNew, hook.PostNew, hData, func() error {
		addDone := p.Stage("git.worktree_add")
		_, addErr := d.Runner.Run(ctx, "git", "worktree", "add", "-b", branch, worktreePath, base)
		addDone()
		if addErr != nil {
			return fmt.Errorf("git worktree add: %w", addErr)
		}
		d.Log.OK("created worktree at %s", worktreePath)

		var syncErr error
		cfg, syncErr = LoadAndSync(ctx, d, rc.Main.Path, nil,
			[]SyncTarget{{Path: worktreePath, Branch: branch}}, rc.Slug, rc.OutputDir)
		if syncErr != nil {
			return syncErr
		}

		artData := config.MakeTemplateData(rc.Slug, branch, worktreePath, rc.OutputDir)
		artDone := p.Stage("artifact.write")
		var artErr error
		artifactPath, artErr = artifact.Write(cfg.Artifact.Spec(), rc.OutputDir, artData)
		artDone()
		if artErr != nil {
			return fmt.Errorf("write artifact: %w", artErr)
		}
		slog.Debug("wrote artifact", "outputDir", rc.OutputDir, "branch", branch)
		return nil
	})
	if postErr != nil {
		d.Log.Warn("%s", postErr)
	}
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		RC:           rc,
		Cfg:          cfg,
		WorktreePath: worktreePath,
		ArtifactPath: artifactPath,
	}, nil
}

// SyncTarget is a worktree to sync configs into.
type SyncTarget struct {
	Path   string
	Branch string
}

// OpenWorktree opens the artifact (or worktree path when no artifact) via the configured command.
func OpenWorktree(
	ctx context.Context, d deps.Deps, openCmd []string,
	branch, wtPath, artifactPath, outputDir string,
) error {
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

// LoadAndSync loads config from sourceDir and syncs it to all targets.
func LoadAndSync(
	ctx context.Context, d deps.Deps, sourceDir string,
	extraPatterns []string, targets []SyncTarget,
	repoSlug, outputDir string,
) (config.Config, error) {
	p := profile.OrDisabled(d.Profiler)

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
		d.Log.Info("→ %s (%s)", t.Branch, t.Path)
		hData := hook.Data{
			Branch:       t.Branch,
			WorktreePath: t.Path,
			Slug:         repoSlug,
			OutputDir:    outputDir,
		}
		postErr, err := hook.RunSandwich(ctx, p, d.HookRunner, cfg.Hooks, hook.PreSync, hook.PostSync, hData, func() error {
			fileSyncDone := p.Stage("file_sync")
			syncErr := d.Syncer.Sync(patterns, internalsync.Config{
				SourceDir: sourceDir,
				TargetDir: t.Path,
			})
			fileSyncDone()
			if syncErr != nil {
				return fmt.Errorf("sync configs to %s: %w", t.Branch, syncErr)
			}
			slog.Debug("synced worktree", "branch", t.Branch, "target", t.Path)
			return nil
		})
		if postErr != nil {
			d.Log.Warn("%s", postErr)
		}
		if err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

// RemoveWorktreeAndArtifact removes a git worktree, its artifact, and its branch.
func RemoveWorktreeAndArtifact(
	ctx context.Context, d deps.Deps,
	target, main worktree.Worktree,
	outputDir string, force bool,
) error {
	p := profile.OrDisabled(d.Profiler)

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

	configLoadDone := p.Stage("config.load")
	cfg, err := config.Load(main.Path)
	configLoadDone()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(main.Path))
	hData := hook.Data{
		Branch:       target.Branch,
		WorktreePath: target.Path,
		Slug:         repoSlug,
		OutputDir:    outputDir,
	}

	postErr, err := hook.RunSandwich(ctx, p, d.HookRunner, cfg.Hooks, hook.PreRemove, hook.PostRemove, hData, func() error {
		wtRemoveDone := p.Stage("git.worktree_remove")
		_, wtErr := d.Runner.Run(ctx, "git", removeArgs...)
		wtRemoveDone()
		if wtErr != nil {
			return fmt.Errorf("%s: %w", removeVerb, wtErr)
		}
		d.Log.OK("removed worktree: %s", target.Path)

		artRemoveDone := p.Stage("artifact.remove")
		artErr := func() error {
			defer artRemoveDone()
			artifactPath, ok, err := config.ResolveArtifactPath(
				cfg.Artifact, repoSlug, target.Branch, target.Path, outputDir)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			if rmErr := os.Remove(artifactPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return fmt.Errorf("remove artifact: %w", rmErr)
			}
			d.Log.OK("removed artifact: %s", artifactPath)
			return nil
		}()
		if artErr != nil {
			return artErr
		}

		branchDeleteDone := p.Stage("git.branch_delete")
		_, branchErr := d.Runner.Run(ctx, "git", "branch", branchFlag, target.Branch)
		branchDeleteDone()
		if branchErr != nil {
			return fmt.Errorf("%s: %w", branchVerb, branchErr)
		}
		d.Log.OK("deleted branch: %s", target.Branch)
		return nil
	})
	if postErr != nil {
		d.Log.Warn("%s", postErr)
	}
	return err
}

func gatherMerged(ctx context.Context, d deps.Deps, rc repo.Context, cwd, base string) (pruneSelection, error) {
	merged, err := worktree.MergedBranches(ctx, d.Runner, base)
	if err != nil {
		return pruneSelection{}, err
	}

	d.Log.Debug("base=%s merged=%v", base, merged)

	mergedSet := make(map[string]bool, len(merged))
	for _, b := range merged {
		mergedSet[b] = true
	}

	var candidates []worktree.Worktree
	for _, wt := range rc.Worktrees {
		d.Log.Debug("worktree branch=%q isMain=%v prunable=%v", wt.Branch, wt.IsMain, wt.Prunable)
		if wt.IsMain || wt.Branch == rc.Main.Branch || wt.Branch == "(detached)" || wt.Prunable {
			d.Log.Debug("  skip: main/detached/prunable")
			continue
		}
		if !mergedSet[wt.Branch] {
			d.Log.Debug("  skip: not merged into %s", base)
			continue
		}
		if repo.CwdInside(cwd, wt.Path) {
			d.Log.Warn("skipping %s: currently in this worktree", wt.Branch)
			continue
		}

		dirty, dirtyErr := worktree.Dirty(ctx, d.Runner, wt.Path)
		if dirtyErr != nil {
			return pruneSelection{}, dirtyErr
		}
		if dirty {
			d.Log.Warn("skipping %s: has uncommitted changes or untracked files", wt.Branch)
			continue
		}

		ahead, _, hasUpstream, aheadErr := worktree.AheadBehind(ctx, d.Runner, wt.Path)
		if aheadErr != nil {
			return pruneSelection{}, aheadErr
		}
		if hasUpstream && ahead > 0 {
			d.Log.Warn("skipping %s: %d unpushed commit(s)", wt.Branch, ahead)
			continue
		}

		d.Log.Debug("  candidate: %s", wt.Branch)
		candidates = append(candidates, wt)
	}

	d.Log.Debug("candidates=%v", func() []string {
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = c.Branch
		}
		return names
	}())

	return pruneSelection{
		candidates: candidates,
		force:      false,
		verb:       "removed",
		emptyMsg:   "no merged worktrees to remove",
	}, nil
}

func gatherAll(rc repo.Context, cwd string) (pruneSelection, error) {
	if err := repo.RequireCwdInside(cwd, rc.Main.Path,
		fmt.Sprintf("--all must be run from the main worktree (%s)", rc.Main.Path)); err != nil {
		return pruneSelection{}, err
	}

	var candidates []worktree.Worktree
	for _, wt := range rc.Worktrees {
		if wt.IsMain || wt.Branch == rc.Main.Branch || wt.Branch == "(detached)" || wt.Prunable {
			continue
		}
		candidates = append(candidates, wt)
	}

	return pruneSelection{
		candidates: candidates,
		force:      true,
		verb:       "force-removed",
		emptyMsg:   "no worktrees to remove",
	}, nil
}

func executePrune(ctx context.Context, d deps.Deps, rc repo.Context, sel pruneSelection, dryRun, yes bool) error {
	if len(sel.candidates) == 0 {
		d.Log.Info(sel.emptyMsg)
		if !dryRun {
			pruneGitWorktreeMetadata(ctx, d)
		}
		return nil
	}

	if dryRun {
		for _, c := range sel.candidates {
			d.Log.Info("would remove: %s (%s)", c.Branch, c.Path)
		}
		d.Log.Info("would run: git worktree prune")
		return nil
	}

	if !yes {
		d.Log.Step("the following worktrees will be %s:", sel.verb)
		for _, c := range sel.candidates {
			d.Log.Info("%s  %s", c.Branch, c.Path)
		}
		d.Log.Prompt("continue? [y/N]: ")
		line, _ := bufio.NewReader(d.In).ReadString('\n')
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			d.Log.Warn("aborted")
			return nil
		}
	}

	var failed []string
	for _, c := range sel.candidates {
		if err := RemoveWorktreeAndArtifact(ctx, d, c, rc.Main, rc.OutputDir, sel.force); err != nil {
			d.Log.Err("error removing %s: %v", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

	pruneGitWorktreeMetadata(ctx, d)

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(failed, ", "))
	}
	return nil
}

func pruneGitWorktreeMetadata(ctx context.Context, d deps.Deps) {
	done := profile.OrDisabled(d.Profiler).Stage("git.worktree_prune")
	_, err := d.Runner.Run(ctx, "git", "worktree", "prune")
	done()
	if err != nil {
		d.Log.Warn("git worktree prune: %v", err)
		return
	}
	d.Log.OK("pruned stale worktree metadata")
}
