package treepad

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

type Service struct {
	runner worktree.CommandRunner
	syncer internalsync.Syncer
	opener artifact.Opener
	out    io.Writer
}

func NewService(runner worktree.CommandRunner, syncer internalsync.Syncer, opener artifact.Opener, out io.Writer) *Service {
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
	Branch string
	Base   string
	Open   bool
}

type RemoveInput struct {
	Branch string
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

	sourceDir, err := resolveSourceDir(in.UseCurrentDir, in.SourcePath, cwd, worktrees)
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

	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !in.SyncOnly {
		spec := artifact.Spec{
			FilenameTemplate: cfg.Artifact.FilenameTemplate,
			ContentTemplate:  cfg.Artifact.ContentTemplate,
		}
		_, _ = fmt.Fprintf(s.out, "\ngenerating artifact files → %s\n", outputDir)
		for _, wt := range worktrees {
			data := artifact.TemplateData{
				Slug:      repoSlug,
				Branch:    wt.Branch,
				Worktrees: []artifact.Worktree{artifact.ToWorktree(wt.Branch, wt.Path, outputDir)},
				OutputDir: outputDir,
			}
			written, err := artifact.Write(spec, outputDir, data)
			if err != nil {
				return fmt.Errorf("generate artifact for %s: %w", wt.Branch, err)
			}
			if written != "" {
				_, _ = fmt.Fprintf(s.out, "  created %s\n", filepath.Base(written))
			}
		}
	}

	var targets []syncTarget
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		targets = append(targets, syncTarget{path: wt.Path, branch: wt.Branch})
	}
	if err := s.syncWithConfig(cfg, in.ExtraPatterns, sourceDir, targets); err != nil {
		return err
	}

	if in.SyncOnly {
		_, _ = fmt.Fprintln(s.out, "\ndone: config sync complete")
	} else {
		_, _ = fmt.Fprintln(s.out, "\ndone: artifact files generated and configs synced")
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

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := s.syncWithConfig(cfg, nil, mainWT.Path, []syncTarget{{path: worktreePath, branch: in.Branch}}); err != nil {
		return err
	}

	outputDir, err := s.resolveOutputDir("", repoSlug)
	if err != nil {
		return err
	}

	spec := artifact.Spec{
		FilenameTemplate: cfg.Artifact.FilenameTemplate,
		ContentTemplate:  cfg.Artifact.ContentTemplate,
	}
	newWT := artifact.ToWorktree(in.Branch, worktreePath, outputDir)
	data := artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    in.Branch,
		Worktrees: []artifact.Worktree{newWT},
		OutputDir: outputDir,
	}
	written, err := artifact.Write(spec, outputDir, data)
	if err != nil {
		return fmt.Errorf("generate artifact file: %w", err)
	}
	slog.Debug("generated artifact file", "path", written, "branch", in.Branch)

	if in.Open && written != "" {
		openSpec := artifact.OpenSpec{Command: cfg.Open.Command}
		openData := artifact.OpenData{ArtifactPath: written, Worktree: newWT}
		_, _ = fmt.Fprintln(s.out, "opening artifact...")
		if err := s.opener.Open(ctx, openSpec, openData); err != nil {
			return fmt.Errorf("open artifact: %w", err)
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

	if _, err := s.runner.Run(ctx, "git", "worktree", "remove", target.Path); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "removed worktree: %s\n", target.Path)

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	outputDir, err := s.resolveOutputDir("", repoSlug)
	if err != nil {
		return err
	}

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	spec := artifact.Spec{
		FilenameTemplate: cfg.Artifact.FilenameTemplate,
		ContentTemplate:  cfg.Artifact.ContentTemplate,
	}
	data := artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    in.Branch,
		Worktrees: []artifact.Worktree{artifact.ToWorktree(in.Branch, target.Path, outputDir)},
		OutputDir: outputDir,
	}
	artifactPath, ok, err := artifact.Path(spec, outputDir, data)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact file: %w", err)
		}
		_, _ = fmt.Fprintf(s.out, "removed artifact file: %s\n", artifactPath)
	}

	if _, err := s.runner.Run(ctx, "git", "branch", "-d", in.Branch); err != nil {
		return fmt.Errorf("git branch -d: %w", err)
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

func (s *Service) syncWithConfig(cfg config.Config, extraPatterns []string, sourceDir string, targets []syncTarget) error {
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

// resolveSourceDir is a pure function — no I/O.
// cwd is pre-fetched by the caller and used only when useCurrentFlag is true.
func resolveSourceDir(useCurrentFlag bool, sourcePath string, cwd string, worktrees []worktree.Worktree) (string, error) {
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
