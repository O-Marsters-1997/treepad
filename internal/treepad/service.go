package treepad

import (
	"bufio"
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
	in     io.Reader
}

func NewService(runner worktree.CommandRunner, syncer internalsync.Syncer, opener artifact.Opener, out io.Writer, in io.Reader) *Service {
	return &Service{runner: runner, syncer: syncer, opener: opener, out: out, in: in}
}

type GenerateInput struct {
	UseCurrentDir bool
	SourcePath    string
	SyncOnly      bool
	OutputDir     string
	ExtraPatterns []string
}

type NewInput struct {
	Branch    string
	Base      string
	Open      bool
	Current   bool
	OutputDir string
}

type RemoveInput struct {
	Branch    string
	OutputDir string
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

type PruneInput struct {
	Base      string // branch to check merges against, e.g. "main"
	OutputDir string
	DryRun    bool
	All       bool // force-remove all non-main worktrees regardless of merge status
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

	var targets []syncTarget
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		targets = append(targets, syncTarget{path: wt.Path, branch: wt.Branch})
	}
	cfg, err := s.loadAndSync(sourceDir, in.ExtraPatterns, targets)
	if err != nil {
		return err
	}

	if !in.SyncOnly {
		_, _ = fmt.Fprintf(s.out, "\ngenerating artifact files → %s\n", outputDir)
		for _, wt := range worktrees {
			data := s.templateData(repoSlug, wt.Branch, wt.Path, outputDir)
			path, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
			if err != nil {
				return fmt.Errorf("write artifact for %s: %w", wt.Branch, err)
			}
			if path != "" {
				_, _ = fmt.Fprintf(s.out, "  created %s\n", filepath.Base(path))
			}
		}
	}

	if in.SyncOnly {
		_, _ = fmt.Fprintln(s.out, "\ndone: config sync complete")
	} else {
		_, _ = fmt.Fprintln(s.out, "\ndone: artifact files generated and configs synced")
	}
	return nil
}

func (s *Service) New(ctx context.Context, in NewInput) error {
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

	cfg, err := s.loadAndSync(mainWT.Path, nil, []syncTarget{{path: worktreePath, branch: in.Branch}})
	if err != nil {
		return err
	}

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	data := s.templateData(repoSlug, in.Branch, worktreePath, outputDir)
	artifactPath, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	slog.Debug("wrote artifact", "outputDir", outputDir, "branch", in.Branch)

	if in.Open {
		openPath := worktreePath
		if artifactPath != "" {
			openPath = artifactPath
		}
		_, _ = fmt.Fprintln(s.out, "opening...")
		openSpec := artifact.OpenSpec{Command: cfg.Open.Command}
		openData := artifact.OpenData{
			ArtifactPath: openPath,
			Worktree:     artifact.ToWorktree(in.Branch, worktreePath, outputDir),
		}
		if err := s.opener.Open(ctx, openSpec, openData); err != nil {
			return fmt.Errorf("open: %w", err)
		}
	}
	if !in.Current {
		_, _ = fmt.Fprintf(s.out, "__TREEPAD_CD__\t%s\n", worktreePath)
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

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	return s.removeWorktree(ctx, *target, mainWT, outputDir)
}

func (s *Service) Prune(ctx context.Context, in PruneInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
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
		return s.pruneAll(ctx, worktrees, mainWT, outputDir, cwd, in.DryRun)
	}

	merged, err := worktree.MergedBranches(ctx, s.runner, in.Base)
	if err != nil {
		return err
	}

	mergedSet := make(map[string]bool, len(merged))
	for _, b := range merged {
		mergedSet[b] = true
	}

	var candidates []worktree.Worktree
	for _, wt := range worktrees {
		if wt.IsMain || wt.Branch == mainWT.Branch || wt.Branch == "(detached)" {
			continue
		}
		if !mergedSet[wt.Branch] {
			continue
		}
		if rel, relErr := filepath.Rel(wt.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
			_, _ = fmt.Fprintf(s.out, "skipping %s: currently in this worktree\n", wt.Branch)
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(s.out, "no merged worktrees to remove")
		return nil
	}

	if in.DryRun {
		for _, c := range candidates {
			_, _ = fmt.Fprintf(s.out, "would remove: %s (%s)\n", c.Branch, c.Path)
		}
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := s.removeWorktree(ctx, c, mainWT, outputDir); err != nil {
			_, _ = fmt.Fprintf(s.out, "error removing %s: %v\n", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(failed, ", "))
	}
	return nil
}

func (s *Service) pruneAll(ctx context.Context, worktrees []worktree.Worktree, mainWT worktree.Worktree, outputDir, cwd string, dryRun bool) error {
	// Must be invoked from the main worktree.
	if rel, relErr := filepath.Rel(mainWT.Path, cwd); relErr != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("--all must be run from the main worktree (%s)", mainWT.Path)
	}

	var candidates []worktree.Worktree
	for _, wt := range worktrees {
		if wt.IsMain || wt.Branch == mainWT.Branch || wt.Branch == "(detached)" {
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(s.out, "no worktrees to remove")
		return nil
	}

	if dryRun {
		for _, c := range candidates {
			_, _ = fmt.Fprintf(s.out, "would remove: %s (%s)\n", c.Branch, c.Path)
		}
		return nil
	}

	_, _ = fmt.Fprintln(s.out, "the following worktrees will be force-removed:")
	for _, c := range candidates {
		_, _ = fmt.Fprintf(s.out, "  %s  %s\n", c.Branch, c.Path)
	}
	_, _ = fmt.Fprint(s.out, "continue? [y/N]: ")

	line, _ := bufio.NewReader(s.in).ReadString('\n')
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		_, _ = fmt.Fprintln(s.out, "aborted")
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := s.forceRemoveWorktree(ctx, c, mainWT, outputDir); err != nil {
			_, _ = fmt.Fprintf(s.out, "error removing %s: %v\n", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(failed, ", "))
	}
	return nil
}

func (s *Service) removeWorktree(ctx context.Context, target worktree.Worktree, mainWT worktree.Worktree, outputDir string) error {
	if _, err := s.runner.Run(ctx, "git", "worktree", "remove", target.Path); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "removed worktree: %s\n", target.Path)

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	data := s.templateData(repoSlug, target.Branch, target.Path, outputDir)
	artifactPath, ok, err := artifact.Path(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact: %w", err)
		}
		_, _ = fmt.Fprintf(s.out, "removed artifact: %s\n", artifactPath)
	}

	if _, err := s.runner.Run(ctx, "git", "branch", "-d", target.Branch); err != nil {
		return fmt.Errorf("git branch -d: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "deleted branch: %s\n", target.Branch)

	return nil
}

func (s *Service) forceRemoveWorktree(ctx context.Context, target worktree.Worktree, mainWT worktree.Worktree, outputDir string) error {
	if _, err := s.runner.Run(ctx, "git", "worktree", "remove", "--force", target.Path); err != nil {
		return fmt.Errorf("git worktree remove --force: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "removed worktree: %s\n", target.Path)

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	data := s.templateData(repoSlug, target.Branch, target.Path, outputDir)
	artifactPath, ok, err := artifact.Path(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact: %w", err)
		}
		_, _ = fmt.Fprintf(s.out, "removed artifact: %s\n", artifactPath)
	}

	if _, err := s.runner.Run(ctx, "git", "branch", "-D", target.Branch); err != nil {
		return fmt.Errorf("git branch -D: %w", err)
	}
	_, _ = fmt.Fprintf(s.out, "deleted branch: %s\n", target.Branch)

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

// loadAndSync loads config from sourceDir, syncs it to targets, and returns the config.
// Pass a nil targets slice to skip syncing and only load the config.
func (s *Service) loadAndSync(sourceDir string, extraPatterns []string, targets []syncTarget) (config.Config, error) {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if len(targets) == 0 {
		return cfg, nil
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
			return config.Config{}, fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
	}
	return cfg, nil
}

func artifactSpec(c config.ArtifactConfig) artifact.Spec {
	return artifact.Spec{
		FilenameTemplate: c.FilenameTemplate,
		ContentTemplate:  c.ContentTemplate,
	}
}

func (s *Service) templateData(repoSlug, branch, worktreePath, outputDir string) artifact.TemplateData {
	wt := artifact.ToWorktree(branch, worktreePath, outputDir)
	return artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    wt.Name,
		Worktrees: []artifact.Worktree{wt},
		OutputDir: outputDir,
	}
}
