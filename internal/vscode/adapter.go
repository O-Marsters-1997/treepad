package vscode

import (
	"fmt"

	"treepad/internal/editor"
	"treepad/internal/sync"
	"treepad/internal/worktree"
)

// vscodeSyncPatterns are the editor-specific files propagated to every worktree.
var vscodeSyncPatterns = []string{
	".vscode/settings.json",
	".vscode/tasks.json",
	".vscode/launch.json",
	".vscode/extensions.json",
	".vscode/*.code-snippets",
}

func init() {
	editor.Register("vscode", func() editor.Adapter { return &Adapter{} })
}

type Adapter struct{}

func (a *Adapter) Name() string { return "vscode" }

func (a *Adapter) Configure(worktrees []worktree.Worktree, opts editor.Options) error {
	if !opts.SyncOnly {
		extensions, err := resolveExtensions(opts.SourceDir)
		if err != nil {
			return err
		}

		fmt.Printf("\ngenerating workspace files → %s\n", opts.OutputDir)
		if err := Generate(worktrees, extensions, opts.Slug, opts.OutputDir); err != nil {
			return err
		}
	}

	fmt.Println("\nsyncing VS Code configs to worktrees...")
	syncer := sync.FileSyncer{}
	for _, wt := range worktrees {
		if wt.Path == opts.SourceDir {
			continue
		}
		fmt.Printf("  → %s (%s)\n", wt.Branch, wt.Path)
		if err := syncer.Sync(vscodeSyncPatterns, sync.Config{
			SourceDir: opts.SourceDir,
			TargetDir: wt.Path,
		}); err != nil {
			return fmt.Errorf("sync vscode configs to %s: %w", wt.Branch, err)
		}
	}
	return nil
}

func resolveExtensions(dir string) ([]string, error) {
	exts, err := ReadExtensions(dir)
	if err != nil {
		return nil, err
	}
	if exts != nil {
		return exts, nil
	}
	return DetectExtensions(dir)
}
