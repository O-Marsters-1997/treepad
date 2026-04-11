package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"treepad/internal/config"
	"treepad/internal/editor"
	"treepad/internal/git"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
)

// Orchestrator holds injected dependencies and owns all command logic.
// run() is reduced to wiring: it builds an Orchestrator and calls Run.
type Orchestrator struct {
	runner git.CommandRunner
	editor editor.Adapter
	syncer internalsync.Syncer
}

func NewOrchestrator(runner git.CommandRunner, ed editor.Adapter, syncer internalsync.Syncer) *Orchestrator {
	return &Orchestrator{runner: runner, editor: ed, syncer: syncer}
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
	worktrees, err := git.List(ctx, o.runner)
	if err != nil {
		return err
	}
	if len(worktrees) == 0 {
		return fmt.Errorf("no git worktrees found")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	sourceDir, err := ResolveSourceDir(in.UseCurrentDir, in.SourcePath, cwd, worktrees)
	if err != nil {
		return err
	}
	fmt.Printf("using config source: %s\n", sourceDir)

	repoSlug := slug.Slug(filepath.Base(sourceDir))

	outputDir := in.OutputDir
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		outputDir = filepath.Join(home, repoSlug+"-workspaces")
	}

	if err := o.editor.Configure(worktrees, editor.Options{
		SourceDir: sourceDir,
		OutputDir: outputDir,
		Slug:      repoSlug,
		SyncOnly:  in.SyncOnly,
	}); err != nil {
		return err
	}

	treePadCfg, err := config.Load(sourceDir)
	if err != nil {
		return err
	}
	patterns := append(treePadCfg.Sync.Files, in.ExtraPatterns...)

	fmt.Println("\nsyncing tool configs to worktrees...")
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		fmt.Printf("  → %s (%s)\n", wt.Branch, wt.Path)
		if err := o.syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: wt.Path,
		}); err != nil {
			return fmt.Errorf("sync tool configs to %s: %w", wt.Branch, err)
		}
	}

	if in.SyncOnly {
		fmt.Println("\ndone: config sync complete")
	} else {
		fmt.Println("\ndone: workspace files generated and configs synced")
	}
	return nil
}
