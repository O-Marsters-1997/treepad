package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"

	"treepad/internal/config"
	"treepad/internal/passthrough"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

type DiffInput struct {
	Branch     string
	Base       string // empty defaults to "origin/main"
	OutputFile string // if set, writes uncolored patch here instead of terminal
	ExtraArgs  []string
	Runner     PassthroughRunner // nil uses osPassthroughRunner
}

// Diff diffs the target worktree against base using three-dot merge-base
// semantics (base...HEAD), matching GitHub PR view. Inherits stdio so the
// user's pager and color config (delta, diff-so-fancy) apply automatically.
func Diff(ctx context.Context, d deps.Deps, in DiffInput) error {
	if in.Branch == "" {
		return errors.New("branch name is required")
	}

	wts, err := repo.ListWorktrees(ctx, d.Runner)
	if err != nil {
		return err
	}
	target, err := worktree.FindOrErr(wts, in.Branch)
	if err != nil {
		return err
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
		runner = passthrough.OSRunner{}
	}
	if _, err := runner.Run(ctx, target.Path, "git", args...); err != nil {
		return fmt.Errorf("git diff: %w", err)
	}
	return nil
}

func resolveBase(wts []worktree.Worktree) string {
	if main, err := worktree.MainWorktree(wts); err == nil {
		return resolveDiffBaseFromMainPath(main.Path)
	}
	return "origin/main"
}

func resolveDiffBaseFromMainPath(mainPath string) string {
	if mainPath == "" {
		return "origin/main"
	}
	if cfg, err := config.Load(mainPath); err == nil && cfg.Diff.Base != "" {
		return cfg.Diff.Base
	}
	return "origin/main"
}
