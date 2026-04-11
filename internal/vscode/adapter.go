package vscode

import (
	"fmt"

	"treepad/internal/editor"
	"treepad/internal/sync"
	"treepad/internal/worktree"
)

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

	fmt.Println("\nsyncing configs to worktrees...")
	syncer := sync.FileSyncer{}
	return sync.SyncAll(syncer, opts.SourceDir, worktrees)
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
