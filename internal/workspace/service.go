package workspace

import (
	"context"
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
	Force     bool
	OutputDir string
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
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

func (s *Service) Remove(ctx context.Context, in RemoveInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))

	if in.Branch == mainWT.Branch {
		return fmt.Errorf("cannot remove the main worktree")
	}

	var target *worktree.Worktree
	for i := range worktrees {
		if worktrees[i].Branch == in.Branch {
			target = &worktrees[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree found for branch %q", in.Branch)
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}
	if rel, relErr := filepath.Rel(target.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("cannot remove the worktree you are currently in; cd elsewhere first")
	}

	wtRemoveArgs := []string{"worktree", "remove", target.Path}
	if in.Force {
		wtRemoveArgs = []string{"worktree", "remove", "--force", target.Path}
	}
	if _, err := s.runner.Run(ctx, "git", wtRemoveArgs...); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "removed worktree: %s\n", target.Path)

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}
	wsPath := filepath.Join(outputDir, codeworkspace.Filename(repoSlug, in.Branch))
	if err := os.Remove(wsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove workspace file: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "removed workspace file: %s\n", wsPath)

	branchFlag := "-d"
	if in.Force {
		branchFlag = "-D"
	}
	if _, err := s.runner.Run(ctx, "git", "branch", branchFlag, in.Branch); err != nil {
		if !in.Force && strings.Contains(err.Error(), "not fully merged") {
			_, _ = fmt.Fprintf(s.out, "branch %s not merged; kept. Re-run with --force to delete, or run: git branch -d %s\n", in.Branch, in.Branch)
			return nil
		}
		return fmt.Errorf("git branch %s: %w", branchFlag, err)
	}
	_, _ = fmt.Fprintf(s.out, "deleted branch: %s\n", in.Branch)

	return nil
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
