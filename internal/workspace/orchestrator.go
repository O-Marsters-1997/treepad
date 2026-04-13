package workspace

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"treepad/internal/codeworkspace"
	"treepad/internal/config"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/worktree"
)

// Orchestrator holds injected dependencies and owns all command logic.
// run() is reduced to wiring: it builds an Orchestrator and calls Run.
type Orchestrator struct {
	runner worktree.CommandRunner
	syncer internalsync.Syncer
	out    io.Writer
}

func NewOrchestrator(runner worktree.CommandRunner, syncer internalsync.Syncer, out io.Writer) *Orchestrator {
	return &Orchestrator{runner: runner, syncer: syncer, out: out}
}

// RunInput carries all CLI-layer decisions. Using a struct means adding a flag
// later is a non-breaking change to callers and tests.
type RunInput struct {
	UseCurrentDir bool
	SourcePath    string // raw CLI arg; empty when not provided
	SyncOnly      bool
	OutputDir     string   // empty triggers the default ~/<slug>-workspaces/ path
	ExtraPatterns []string // appended to patterns from .treepad.json
}

func (o *Orchestrator) Run(ctx context.Context, in RunInput) error {
	worktrees, err := worktree.List(ctx, o.runner)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}
	if len(worktrees) == 0 {
		return fmt.Errorf("no git worktrees found")
	}
	slog.Debug("discovered worktrees", "count", len(worktrees))

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	sourceDir, err := ResolveSourceDir(in.UseCurrentDir, in.SourcePath, cwd, worktrees)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	slog.Debug("resolved source directory", "sourceDir", sourceDir, "useCurrentDir", in.UseCurrentDir, "sourcePath", in.SourcePath)
	_, _ = fmt.Fprintf(o.out, "using config source: %s\n", sourceDir)

	repoSlug := slug.Slug(filepath.Base(sourceDir))

	outputDir := in.OutputDir
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		outputDir = filepath.Join(home, repoSlug+"-workspaces")
	}
	slog.Debug("output directory", "dir", outputDir, "explicit", in.OutputDir != "")

	if !in.SyncOnly {
		extensions, err := codeworkspace.ResolveExtensions(sourceDir)
		if err != nil {
			return fmt.Errorf("resolve extensions: %w", err)
		}
		slog.Debug("resolved extensions", "count", len(extensions))
		_, _ = fmt.Fprintf(o.out, "\ngenerating workspace files → %s\n", outputDir)
		if err := codeworkspace.Generate(worktrees, extensions, repoSlug, outputDir, o.out); err != nil {
			return err
		}
	}

	treePadCfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	patterns := slices.Concat(treePadCfg.Sync.Files, in.ExtraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	_, _ = fmt.Fprintln(o.out, "\nsyncing configs to worktrees...")
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		_, _ = fmt.Fprintf(o.out, "  → %s (%s)\n", wt.Branch, wt.Path)
		if err := o.syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: wt.Path,
		}); err != nil {
			return fmt.Errorf("sync configs to %s: %w", wt.Branch, err)
		}
		slog.Debug("synced worktree", "branch", wt.Branch, "target", wt.Path)
	}

	if in.SyncOnly {
		_, _ = fmt.Fprintln(o.out, "\ndone: config sync complete")
	} else {
		_, _ = fmt.Fprintln(o.out, "\ndone: workspace files generated and configs synced")
	}
	return nil
}
