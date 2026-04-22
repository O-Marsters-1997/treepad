package treepad

import (
	"context"
	"fmt"
	"os"

	"treepad/internal/config"
	"treepad/internal/worktree"
)

type DiffInput struct {
	Branch     string
	Base       string // empty defaults to "main"
	OutputFile string // if set, writes uncolored patch here instead of terminal
	ExtraArgs  []string
	Runner     PassthroughRunner // nil uses osPassthroughRunner
}

// Diff diffs the target worktree against base using three-dot merge-base
// semantics (base...HEAD), matching GitHub PR view. Inherits stdio so the
// user's pager and color config (delta, diff-so-fancy) apply automatically.
func Diff(ctx context.Context, d Deps, in DiffInput) error {
	if in.Branch == "" {
		return fmt.Errorf("branch name is required")
	}

	wts, err := listWorktrees(ctx, d)
	if err != nil {
		return err
	}
	target, ok := worktree.FindByBranch(wts, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q; run `tp sync` to list worktrees", in.Branch)
	}
	if target.Prunable {
		return fmt.Errorf("worktree for %q is prunable (%s); run `tp prune`", in.Branch, target.PrunableReason)
	}

	base := in.Base
	if base == "" {
		base = resolveBase(wts)
	}

	refs := base + "...HEAD"

	if in.OutputFile != "" {
		args := append([]string{"-C", target.Path, "diff", "--no-color", refs}, in.ExtraArgs...)
		out, err := d.Runner.Run(ctx, "git", args...)
		if err != nil {
			return fmt.Errorf("git diff: %w", err)
		}
		if err := os.WriteFile(in.OutputFile, out, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", in.OutputFile, err)
		}
		d.Log.OK(fmt.Sprintf("wrote diff to %s", in.OutputFile))
		return nil
	}

	args := append([]string{"diff", refs}, in.ExtraArgs...)
	runner := in.Runner
	if runner == nil {
		runner = osPassthroughRunner{}
	}
	if _, err := runner.Run(ctx, target.Path, "git", args...); err != nil {
		return fmt.Errorf("git diff: %w", err)
	}
	return nil
}

// resolveBase returns the base ref for diffing, loading from config when
// available. Falls back to "origin/main".
func resolveBase(wts []worktree.Worktree) string {
	if main, err := worktree.MainWorktree(wts); err == nil {
		if cfg, err := config.Load(main.Path); err == nil {
			return cfg.Diff.Base
		}
	}
	return "origin/main"
}
