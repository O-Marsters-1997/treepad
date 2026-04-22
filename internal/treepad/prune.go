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

func Prune(ctx context.Context, d Deps, in PruneInput) error {
	v, err := d.NewRepoView(ctx, in.OutputDir)
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

	if in.All {
		return pruneAll(ctx, d, v.Worktrees(), v.Main(), v.OutputDir(), cwd, in.DryRun)
	}

	merged, err := v.MergedInto(ctx, in.Base)
	if err != nil {
		return err
	}

	snaps, err := v.Snapshots(ctx, Probe{Dirty: true, AheadBehind: true})
	if err != nil {
		return err
	}

	var candidates []worktree.Worktree
	for _, s := range snaps {
		if s.IsMain || s.Branch == v.Main().Branch || s.Branch == "(detached)" || s.Prunable {
			continue
		}
		if !merged[s.Branch] {
			continue
		}
		if rel, relErr := filepath.Rel(s.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
			d.Log.Warn("skipping %s: currently in this worktree", s.Branch)
			continue
		}
		if s.Dirty {
			d.Log.Warn("skipping %s: has uncommitted changes or untracked files", s.Branch)
			continue
		}
		if s.HasUpstream && s.Ahead > 0 {
			d.Log.Warn("skipping %s: %d unpushed commit(s)", s.Branch, s.Ahead)
			continue
		}
		candidates = append(candidates, s.Worktree)
	}

	if len(candidates) == 0 {
		d.Log.Info("no merged worktrees to remove")
		if !in.DryRun {
			pruneGitWorktreeMetadata(ctx, d)
		}
		return nil
	}

	if in.DryRun {
		for _, c := range candidates {
			d.Log.Info("would remove: %s (%s)", c.Branch, c.Path)
		}
		d.Log.Info("would run: git worktree prune")
		return nil
	}

	if !in.Yes {
		d.Log.Step("the following worktrees will be removed:")
		for _, c := range candidates {
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
	for _, c := range candidates {
		if err := removeWorktreeAndArtifact(ctx, d, c, v.Main(), v.OutputDir(), false); err != nil {
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

func pruneAll(ctx context.Context, d Deps, worktrees []worktree.Worktree, main worktree.Worktree, outputDir, cwd string, dryRun bool) error {
	// Must be invoked from the main worktree.
	if rel, relErr := filepath.Rel(main.Path, cwd); relErr != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("--all must be run from the main worktree (%s)", main.Path)
	}

	var candidates []worktree.Worktree
	for _, wt := range worktrees {
		if wt.IsMain || wt.Branch == main.Branch || wt.Branch == "(detached)" || wt.Prunable {
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		d.Log.Info("no worktrees to remove")
		return nil
	}

	if dryRun {
		for _, c := range candidates {
			d.Log.Info("would remove: %s (%s)", c.Branch, c.Path)
		}
		d.Log.Info("would run: git worktree prune")
		return nil
	}

	d.Log.Step("the following worktrees will be force-removed:")
	for _, c := range candidates {
		d.Log.Info("%s  %s", c.Branch, c.Path)
	}
	d.Log.Prompt("continue? [y/N]: ")

	line, _ := bufio.NewReader(d.In).ReadString('\n')
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		d.Log.Warn("aborted")
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := removeWorktreeAndArtifact(ctx, d, c, main, outputDir, true); err != nil {
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
