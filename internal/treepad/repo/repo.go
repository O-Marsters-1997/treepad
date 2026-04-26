// Package repo resolves the repository context shared by every treepad verb:
// the worktree list, the main worktree, the repo slug, and the artifact output directory.
package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"treepad/internal/slug"
	"treepad/internal/worktree"
)

// Context captures the values derived from listing worktrees at the
// beginning of every treepad operation.
type Context struct {
	Worktrees []worktree.Worktree
	Main      worktree.Worktree
	Slug      string
	OutputDir string
}

// Load runs the prologue shared by every public operation.
// An empty explicitOutputDir means "derive the default".
func Load(ctx context.Context, runner worktree.CommandRunner, explicitOutputDir string) (Context, error) {
	worktrees, err := ListWorktrees(ctx, runner)
	if err != nil {
		return Context{}, err
	}
	main, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return Context{}, err
	}
	repoSlug := slug.Slug(filepath.Base(main.Path))
	outputDir, err := ResolveOutputDir(explicitOutputDir, repoSlug)
	if err != nil {
		return Context{}, err
	}
	return Context{
		Worktrees: worktrees,
		Main:      main,
		Slug:      repoSlug,
		OutputDir: outputDir,
	}, nil
}

// ListWorktrees returns all git worktrees, returning an error when none are found.
func ListWorktrees(ctx context.Context, runner worktree.CommandRunner) ([]worktree.Worktree, error) {
	worktrees, err := worktree.List(ctx, runner)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	if len(worktrees) == 0 {
		return nil, errors.New("no git worktrees found")
	}
	slog.Debug("discovered worktrees", "count", len(worktrees))
	return worktrees, nil
}

// ResolveOutputDir returns explicit when non-empty, otherwise derives the
// default workspace directory from the repo slug and the user's home dir.
func ResolveOutputDir(explicit, repoSlug string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, repoSlug+"-workspaces"), nil
}

// ResolveSourceDir is a pure function — no I/O.
// cwd is pre-fetched by the caller and used only when useCurrentFlag is true.
func ResolveSourceDir(
	useCurrentFlag bool,
	sourcePath string,
	cwd string,
	worktrees []worktree.Worktree,
) (string, error) {
	switch {
	case useCurrentFlag:
		return cwd, nil
	case sourcePath != "":
		return filepath.Abs(sourcePath)
	default:
		main, err := worktree.MainWorktree(worktrees)
		if err != nil {
			return "", err
		}
		return main.Path, nil
	}
}

// CwdInside reports whether cwd is inside wtPath (inclusive).
func CwdInside(cwd, wtPath string) bool {
	rel, err := filepath.Rel(wtPath, cwd)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// RequireCwdInside returns an error with msg when cwd is not inside wtPath.
func RequireCwdInside(cwd, wtPath, msg string) error {
	if !CwdInside(cwd, wtPath) {
		return fmt.Errorf("%s", msg)
	}
	return nil
}
