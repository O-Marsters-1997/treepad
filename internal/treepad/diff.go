package treepad

import (
	"context"
	"fmt"
	"os"

	"treepad/internal/worktree"
)

// DiffInput parameterises a tp diff invocation.
type DiffInput struct {
	Branch     string
	Base       string // default "main" if empty
	OutputFile string // optional; writes uncolored patch to this path
	ExtraArgs  []string
	// Runner overrides osPassthroughRunner for testing.
	Runner PassthroughRunner
}

// Diff shows the diff of the target worktree against base (default "main")
// using three-dot merge-base semantics (base...HEAD). When OutputFile is set
// the patch is written uncolored to that path; otherwise stdio is inherited so
// git's pager and color config apply.
func Diff(ctx context.Context, d Deps, in DiffInput) error {
	if in.Branch == "" {
		return fmt.Errorf("branch name is required")
	}
	base := in.Base
	if base == "" {
		base = "main"
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
