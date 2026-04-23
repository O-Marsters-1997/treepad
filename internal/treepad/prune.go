package treepad

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"treepad/internal/worktree"
)

type PruneInput struct {
	Base      string // branch to check merges against, e.g. "main"
	OutputDir string
	DryRun    bool
	All       bool // force-remove all non-main worktrees regardless of merge status
	Yes       bool // skip the interactive confirmation prompt (for scripting)
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

type pruneSelection struct {
	candidates []worktree.Worktree
	force      bool
	verb       string // substituted into "the following worktrees will be %s:"
	emptyMsg   string
}

func Prune(ctx context.Context, d Deps, in PruneInput) error {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}

	var sel pruneSelection
	if in.All {
		sel, err = gatherAll(rc, cwd)
	} else {
		sel, err = gatherMerged(ctx, d, rc, cwd, in.Base)
	}
	if err != nil {
		return err
	}

	return executePrune(ctx, d, rc, sel, in.DryRun, in.Yes)
}

func gatherMerged(ctx context.Context, d Deps, rc repoContext, cwd, base string) (pruneSelection, error) {
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
		if rel, relErr := filepath.Rel(wt.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
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

func gatherAll(rc repoContext, cwd string) (pruneSelection, error) {
	if rel, relErr := filepath.Rel(rc.Main.Path, cwd); relErr != nil || strings.HasPrefix(rel, "..") {
		return pruneSelection{}, fmt.Errorf("--all must be run from the main worktree (%s)", rc.Main.Path)
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

func executePrune(ctx context.Context, d Deps, rc repoContext, sel pruneSelection, dryRun, yes bool) error {
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
		if err := removeWorktreeAndArtifact(ctx, d, c, rc.Main, rc.OutputDir, sel.force); err != nil {
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

func pruneGitWorktreeMetadata(ctx context.Context, d Deps) {
	if _, err := d.Runner.Run(ctx, "git", "worktree", "prune"); err != nil {
		d.Log.Warn("git worktree prune: %v", err)
		return
	}
	d.Log.OK("pruned stale worktree metadata")
}
