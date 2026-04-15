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
	"treepad/internal/hook"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/ui"
	"treepad/internal/worktree"
)

type Service struct {
	runner     worktree.CommandRunner
	syncer     internalsync.Syncer
	opener     artifact.Opener
	hookRunner hook.Runner
	out        io.Writer   // stdout: machine-consumed payloads only
	in         io.Reader
	log        *ui.Printer // stderr: structured narrative output
}

func NewService(runner worktree.CommandRunner, syncer internalsync.Syncer, opener artifact.Opener, hookRunner hook.Runner, out io.Writer, in io.Reader, log *ui.Printer) *Service {
	return &Service{runner: runner, syncer: syncer, opener: opener, hookRunner: hookRunner, out: out, in: in, log: log}
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
	s.log.Info("using config source: %s", sourceDir)

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
	cfg, err := s.loadAndSync(ctx, sourceDir, in.ExtraPatterns, targets, repoSlug, outputDir)
	if err != nil {
		return err
	}

	if !in.SyncOnly {
		s.log.Step("generating artifact files → %s", outputDir)
		for _, wt := range worktrees {
			data := s.templateData(repoSlug, wt.Branch, wt.Path, outputDir)
			path, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
			if err != nil {
				return fmt.Errorf("write artifact for %s: %w", wt.Branch, err)
			}
			if path != "" {
				s.log.Info("created %s", filepath.Base(path))
			}
		}
	}

	if in.SyncOnly {
		s.log.OK("config sync complete")
	} else {
		s.log.OK("artifact files generated and configs synced")
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

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	hData := s.hookData(repoSlug, in.Branch, worktreePath, outputDir)
	if err := s.runPreHook(ctx, hook.PreNew, cfg.Hooks.For(hook.PreNew), hData); err != nil {
		return fmt.Errorf("pre_new hook: %w", err)
	}

	s.log.Step("creating worktree %s", in.Branch)
	if _, err := s.runner.Run(ctx, "git", "worktree", "add", "-b", in.Branch, worktreePath, in.Base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	s.log.OK("created worktree at %s", worktreePath)

	if err := s.syncTargets(ctx, cfg, mainWT.Path, nil, []syncTarget{{path: worktreePath, branch: in.Branch}}, repoSlug, outputDir); err != nil {
		return err
	}

	data := s.templateData(repoSlug, in.Branch, worktreePath, outputDir)
	artifactPath, err := artifact.Write(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	slog.Debug("wrote artifact", "outputDir", outputDir, "branch", in.Branch)

	s.runPostHook(ctx, hook.PostNew, cfg.Hooks.For(hook.PostNew), hData)

	if in.Open {
		openPath := worktreePath
		if artifactPath != "" {
			openPath = artifactPath
		}
		s.log.Step("opening...")
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
		s.emitCD(worktreePath)
	}
	return nil
}

func (s *Service) emitCD(path string) {
	_, _ = fmt.Fprintf(s.out, "__TREEPAD_CD__\t%s\n", path)
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

	found, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q", in.Branch)
	}
	target := &found

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
			s.log.Warn("skipping %s: currently in this worktree", wt.Branch)
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		s.log.Info("no merged worktrees to remove")
		return nil
	}

	if in.DryRun {
		for _, c := range candidates {
			s.log.Info("would remove: %s (%s)", c.Branch, c.Path)
		}
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := s.removeWorktree(ctx, c, mainWT, outputDir); err != nil {
			s.log.Err("error removing %s: %v", c.Branch, err)
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
		s.log.Info("no worktrees to remove")
		return nil
	}

	if dryRun {
		for _, c := range candidates {
			s.log.Info("would remove: %s (%s)", c.Branch, c.Path)
		}
		return nil
	}

	s.log.Step("the following worktrees will be force-removed:")
	for _, c := range candidates {
		s.log.Info("  %s  %s", c.Branch, c.Path)
	}
	s.log.Prompt("continue? [y/N]: ")

	line, _ := bufio.NewReader(s.in).ReadString('\n')
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		s.log.Warn("aborted")
		return nil
	}

	var failed []string
	for _, c := range candidates {
		if err := s.forceRemoveWorktree(ctx, c, mainWT, outputDir); err != nil {
			s.log.Err("error removing %s: %v", c.Branch, err)
			failed = append(failed, c.Branch)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(failed, ", "))
	}
	return nil
}

func (s *Service) removeWorktree(ctx context.Context, target worktree.Worktree, mainWT worktree.Worktree, outputDir string) error {
	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	hData := s.hookData(repoSlug, target.Branch, target.Path, outputDir)
	if err := s.runPreHook(ctx, hook.PreRemove, cfg.Hooks.For(hook.PreRemove), hData); err != nil {
		return fmt.Errorf("pre_remove hook: %w", err)
	}

	if _, err := s.runner.Run(ctx, "git", "worktree", "remove", target.Path); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}
	s.log.OK("removed worktree: %s", target.Path)

	data := s.templateData(repoSlug, target.Branch, target.Path, outputDir)
	artifactPath, ok, err := artifact.Path(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact: %w", err)
		}
		s.log.OK("removed artifact: %s", artifactPath)
	}

	if _, err := s.runner.Run(ctx, "git", "branch", "-d", target.Branch); err != nil {
		return fmt.Errorf("git branch -d: %w", err)
	}
	s.log.OK("deleted branch: %s", target.Branch)

	s.runPostHook(ctx, hook.PostRemove, cfg.Hooks.For(hook.PostRemove), hData)

	return nil
}

func (s *Service) forceRemoveWorktree(ctx context.Context, target worktree.Worktree, mainWT worktree.Worktree, outputDir string) error {
	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))
	hData := s.hookData(repoSlug, target.Branch, target.Path, outputDir)
	if err := s.runPreHook(ctx, hook.PreRemove, cfg.Hooks.For(hook.PreRemove), hData); err != nil {
		return fmt.Errorf("pre_remove hook: %w", err)
	}

	if _, err := s.runner.Run(ctx, "git", "worktree", "remove", "--force", target.Path); err != nil {
		return fmt.Errorf("git worktree remove --force: %w", err)
	}
	s.log.OK("removed worktree: %s", target.Path)

	data := s.templateData(repoSlug, target.Branch, target.Path, outputDir)
	artifactPath, ok, err := artifact.Path(artifactSpec(cfg.Artifact), outputDir, data)
	if err != nil {
		return fmt.Errorf("resolve artifact path: %w", err)
	}
	if ok {
		if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove artifact: %w", err)
		}
		s.log.OK("removed artifact: %s", artifactPath)
	}

	if _, err := s.runner.Run(ctx, "git", "branch", "-D", target.Branch); err != nil {
		return fmt.Errorf("git branch -D: %w", err)
	}
	s.log.OK("deleted branch: %s", target.Branch)

	s.runPostHook(ctx, hook.PostRemove, cfg.Hooks.For(hook.PostRemove), hData)

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

// loadAndSync loads config from sourceDir, syncs to targets with pre/post_sync hooks, and returns the config.
func (s *Service) loadAndSync(ctx context.Context, sourceDir string, extraPatterns []string, targets []syncTarget, repoSlug, outputDir string) (config.Config, error) {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	if err := s.syncTargets(ctx, cfg, sourceDir, extraPatterns, targets, repoSlug, outputDir); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// syncTargets syncs files from sourceDir to each target, firing pre_sync/post_sync hooks around each.
func (s *Service) syncTargets(ctx context.Context, cfg config.Config, sourceDir string, extraPatterns []string, targets []syncTarget, repoSlug, outputDir string) error {
	if len(targets) == 0 {
		return nil
	}

	patterns := slices.Concat(cfg.Sync.Files, extraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	s.log.Step("syncing configs to worktrees...")
	for _, t := range targets {
		s.log.Info("  → %s (%s)", t.branch, t.path)
		hData := s.hookData(repoSlug, t.branch, t.path, outputDir)
		if err := s.runPreHook(ctx, hook.PreSync, cfg.Hooks.For(hook.PreSync), hData); err != nil {
			return fmt.Errorf("pre_sync hook for %s: %w", t.branch, err)
		}
		if err := s.syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: t.path,
		}); err != nil {
			return fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		s.runPostHook(ctx, hook.PostSync, cfg.Hooks.For(hook.PostSync), hData)
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
	}
	return nil
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

func (s *Service) hookData(repoSlug, branch, worktreePath, outputDir string) hook.Data {
	return hook.Data{
		Branch:       branch,
		WorktreePath: worktreePath,
		Slug:         repoSlug,
		OutputDir:    outputDir,
	}
}

// runPreHook runs blocking pre-event hooks. A non-zero exit aborts the operation.
func (s *Service) runPreHook(ctx context.Context, event hook.Event, hooks []string, data hook.Data) error {
	if len(hooks) == 0 {
		return nil
	}
	data.HookType = string(event)
	return s.hookRunner.Run(ctx, hooks, data)
}

// runPostHook runs post-event hooks. Failures are logged but never abort the operation.
func (s *Service) runPostHook(ctx context.Context, event hook.Event, hooks []string, data hook.Data) {
	if len(hooks) == 0 {
		return
	}
	data.HookType = string(event)
	if err := s.hookRunner.Run(ctx, hooks, data); err != nil {
		s.log.Warn("post hook %s failed: %v", event, err)
	}
}
