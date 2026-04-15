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
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
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

	if in.All {
		return pruneAll(ctx, d, rc.Worktrees, rc.Main, rc.OutputDir, cwd, in.DryRun)
	}

	merged, err := worktree.MergedBranches(ctx, d.Runner, in.Base)
	if err != nil {
		return err
	}

	mergedSet := make(map[string]bool, len(merged))
	for _, b := range merged {
		mergedSet[b] = true
	}

	var candidates []worktree.Worktree
	for _, wt := range rc.Worktrees {
		if wt.IsMain || wt.Branch == rc.Main.Branch || wt.Branch == "(detached)" {
			continue
		}
		if !mergedSet[wt.Branch] {
			continue
		}
		if rel, relErr := filepath.Rel(wt.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
			_, _ = fmt.Fprintf(d.Out, "skipping %s: currently in this worktree\n", wt.Branch)
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(d.Out, "no merged worktrees to remove")
		return nil
	}

	if in.DryRun {
		for _, c := range candidates {
			_, _ = fmt.Fprintf(d.Out, "would remove: %s (%s)\n", c.Branch, c.Path)
		}
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := removeWorktreeAndArtifact(ctx, d, c, rc.Main, rc.OutputDir, false); err != nil {
			_, _ = fmt.Fprintf(d.Out, "error removing %s: %v\n", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

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
		if wt.IsMain || wt.Branch == main.Branch || wt.Branch == "(detached)" {
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(d.Out, "no worktrees to remove")
		return nil
	}

	if dryRun {
		for _, c := range candidates {
			_, _ = fmt.Fprintf(d.Out, "would remove: %s (%s)\n", c.Branch, c.Path)
		}
		return nil
	}

	_, _ = fmt.Fprintln(d.Out, "the following worktrees will be force-removed:")
	for _, c := range candidates {
		_, _ = fmt.Fprintf(d.Out, "  %s  %s\n", c.Branch, c.Path)
	}
	_, _ = fmt.Fprint(d.Out, "continue? [y/N]: ")

	line, _ := bufio.NewReader(d.In).ReadString('\n')
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		_, _ = fmt.Fprintln(d.Out, "aborted")
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := removeWorktreeAndArtifact(ctx, d, c, main, outputDir, true); err != nil {
			_, _ = fmt.Fprintf(d.Out, "error removing %s: %v\n", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(failed, ", "))
	}
	return nil
}
