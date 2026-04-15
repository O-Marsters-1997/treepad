package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"treepad/internal/codeworkspace"
	"treepad/internal/config"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

type Service struct {
	runner worktree.CommandRunner
	syncer internalsync.Syncer
	opener Opener
	out    io.Writer
}

func NewService(runner worktree.CommandRunner, syncer internalsync.Syncer, opener Opener, out io.Writer) *Service {
	return &Service{runner: runner, syncer: syncer, opener: opener, out: out}
}

type GenerateInput struct {
	UseCurrentDir bool
	SourcePath    string
	SyncOnly      bool
	OutputDir     string
	ExtraPatterns []string
}

type CreateInput struct {
	Branch    string
	Base      string
	Open      bool
	OutputDir string
}

type RemoveInput struct {
	Branch    string
	OutputDir string
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

// RemoveResult captures the per-step outcome of a Remove call.
type RemoveResult struct {
	WorktreeRemoved  bool
	WorktreeErr      error
	WorkspaceRemoved bool
	WorkspaceErr     error
	BranchDeleted    bool
	BranchWarning    string // non-empty when branch is unmerged and kept (not an error)
	BranchErr        error
}

func (r RemoveResult) Err() error {
	return errors.Join(r.WorktreeErr, r.WorkspaceErr, r.BranchErr)
}

func (s *Service) Generate(ctx context.Context, in GenerateInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	sourceDir, err := ResolveSourceDir(in.UseCurrentDir, in.SourcePath, cwd, worktrees)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	slog.Debug("resolved source directory", "sourceDir", sourceDir, "useCurrentDir", in.UseCurrentDir, "sourcePath", in.SourcePath)
	_, _ = fmt.Fprintf(s.out, "using config source: %s\n", sourceDir)

	repoSlug := slug.Slug(filepath.Base(sourceDir))

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}
	slog.Debug("output directory", "dir", outputDir, "explicit", in.OutputDir != "")

	if !in.SyncOnly {
		extensions, err := codeworkspace.ResolveExtensions(sourceDir)
		if err != nil {
			return fmt.Errorf("resolve extensions: %w", err)
		}
		slog.Debug("resolved extensions", "count", len(extensions))
		_, _ = fmt.Fprintf(s.out, "\ngenerating workspace files → %s\n", outputDir)
		if err := codeworkspace.Generate(worktrees, extensions, repoSlug, outputDir, s.out); err != nil {
			return err
		}
	}

	var targets []syncTarget
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		targets = append(targets, syncTarget{path: wt.Path, branch: wt.Branch})
	}
	if err := s.loadAndSync(sourceDir, in.ExtraPatterns, targets); err != nil {
		return err
	}

	if in.SyncOnly {
		_, _ = fmt.Fprintln(s.out, "\ndone: config sync complete")
	} else {
		_, _ = fmt.Fprintln(s.out, "\ndone: workspace files generated and configs synced")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in CreateInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	worktreePath := filepath.Join(filepath.Dir(mainWT.Path), repoSlug+"-"+slug.Slug(in.Branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	if _, err := s.runner.Run(ctx, "git", "worktree", "add", "-b", in.Branch, worktreePath, in.Base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "created worktree at %s\n", worktreePath)

	if err := s.loadAndSync(mainWT.Path, nil, []syncTarget{{path: worktreePath, branch: in.Branch}}); err != nil {
		return err
	}

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	extensions, err := codeworkspace.ResolveExtensions(mainWT.Path)
	if err != nil {
		return fmt.Errorf("resolve extensions: %w", err)
	}

	newWT := worktree.Worktree{Path: worktreePath, Branch: in.Branch}
	if err := codeworkspace.Generate([]worktree.Worktree{newWT}, extensions, repoSlug, outputDir, io.Discard); err != nil {
		return fmt.Errorf("generate workspace file: %w", err)
	}
	slog.Debug("generated workspace file", "outputDir", outputDir, "branch", in.Branch)

	if in.Open {
		wsFile := filepath.Join(outputDir, codeworkspace.Filename(repoSlug, in.Branch))
		_, _ = fmt.Fprintln(s.out, "opening workspace...")
		if err := s.opener.Open(ctx, wsFile); err != nil {
			return fmt.Errorf("open workspace: %w", err)
		}
	}
	return nil
}

func (s *Service) Remove(ctx context.Context, in RemoveInput) (RemoveResult, error) {
	var result RemoveResult

	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return result, err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return result, err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))

	if in.Branch == mainWT.Branch {
		return result, fmt.Errorf("cannot remove the main worktree")
	}

	var target *worktree.Worktree
	for i := range worktrees {
		if worktrees[i].Branch == in.Branch {
			target = &worktrees[i]
			break
		}
	}

	recoveryMode := false
	if target == nil {
		// Check for recovery mode: worktree is already gone but branch still exists.
		out, listErr := s.runner.Run(ctx, "git", "branch", "--list", in.Branch)
		if listErr != nil || strings.TrimSpace(string(out)) == "" {
			return result, fmt.Errorf("no worktree found for branch %q", in.Branch)
		}
		recoveryMode = true
	}

	if !recoveryMode {
		cwd := in.Cwd
		if cwd == "" {
			cwd, err = os.Getwd()
			if err != nil {
				return result, fmt.Errorf("get current directory: %w", err)
			}
		}
		if rel, relErr := filepath.Rel(target.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
			return result, fmt.Errorf("cannot remove the worktree you are currently in; cd elsewhere first")
		}
	}

	// Step 1: Remove worktree (skip in recovery mode).
	if !recoveryMode {
		if _, err := s.runner.Run(ctx, "git", "worktree", "remove", target.Path); err != nil {
			result.WorktreeErr = fmt.Errorf("git worktree remove: %w", err)
			_, _ = fmt.Fprintf(s.out, "failed to remove worktree %s: %v\n", target.Path, err)
		} else {
			result.WorktreeRemoved = true
			_, _ = fmt.Fprintf(s.out, "removed worktree: %s\n", target.Path)
		}
	}

	// Step 2: Remove workspace file.
	outputDir, resolveErr := s.resolveOutputDir(in.OutputDir, repoSlug)
	if resolveErr != nil {
		result.WorkspaceErr = resolveErr
		_, _ = fmt.Fprintf(s.out, "failed to resolve workspace directory: %v\n", resolveErr)
	} else {
		wsPath := filepath.Join(outputDir, codeworkspace.Filename(repoSlug, in.Branch))
		if err := os.Remove(wsPath); err != nil && !os.IsNotExist(err) {
			result.WorkspaceErr = fmt.Errorf("remove workspace file: %w", err)
			_, _ = fmt.Fprintf(s.out, "failed to remove workspace file %s: %v\n", wsPath, err)
		} else {
			result.WorkspaceRemoved = true
			_, _ = fmt.Fprintf(s.out, "removed workspace file: %s\n", wsPath)
		}
	}

	// Step 3: Delete local branch.
	if _, err := s.runner.Run(ctx, "git", "branch", "-d", in.Branch); err != nil {
		if strings.Contains(err.Error(), "not fully merged") {
			result.BranchWarning = fmt.Sprintf("branch %s not merged; kept. Re-run with --force to delete, or run: git branch -d %s", in.Branch, in.Branch)
			_, _ = fmt.Fprintf(s.out, "%s\n", result.BranchWarning)
		} else {
			result.BranchErr = fmt.Errorf("git branch -d: %w", err)
			_, _ = fmt.Fprintf(s.out, "failed to delete branch %s: %v\n", in.Branch, err)
		}
	} else {
		result.BranchDeleted = true
		_, _ = fmt.Fprintf(s.out, "deleted branch: %s\n", in.Branch)
	}

	return result, result.Err()
}

type syncTarget struct {
	path   string
	branch string
}

func (s *Service) listWorktrees(ctx context.Context) ([]worktree.Worktree, error) {
	worktrees, err := worktree.List(ctx, s.runner)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	if len(worktrees) == 0 {
		return nil, fmt.Errorf("no git worktrees found")
	}
	slog.Debug("discovered worktrees", "count", len(worktrees))
	return worktrees, nil
}

func (s *Service) resolveOutputDir(explicit string, repoSlug string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, repoSlug+"-workspaces"), nil
}

func (s *Service) loadAndSync(sourceDir string, extraPatterns []string, targets []syncTarget) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	patterns := slices.Concat(cfg.Sync.Files, extraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	_, _ = fmt.Fprintln(s.out, "\nsyncing configs to worktrees...")
	for _, t := range targets {
		_, _ = fmt.Fprintf(s.out, "  → %s (%s)\n", t.branch, t.path)
		if err := s.syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: t.path,
		}); err != nil {
			return fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
	}
	return nil
}
