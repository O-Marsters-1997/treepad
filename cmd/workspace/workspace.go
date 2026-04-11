package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"treepad/internal/config"
	"treepad/internal/editor"
	"treepad/internal/git"
	"treepad/internal/slug"
	"treepad/internal/sync"
	_ "treepad/internal/vscode" // registers the "vscode" adapter
)

func Command() *cli.Command {
	return &cli.Command{
		Name:      "workspace",
		Usage:     "sync editor configs and generate workspace files across git worktrees",
		ArgsUsage: "[source-path]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "use-current",
				Aliases: []string{"c"},
				Usage:   "use current directory as config source instead of the main worktree",
			},
			&cli.BoolFlag{
				Name:  "sync-only",
				Usage: "sync configs to all worktrees; skip workspace file generation",
			},
			&cli.StringFlag{
				Name:    "output-dir",
				Aliases: []string{"o"},
				Usage:   "directory for generated workspace files (default: ~/<repo-slug>-workspaces/)",
			},
			&cli.StringFlag{
				Name:  "editor",
				Value: "vscode",
				Usage: "editor adapter to use",
			},
			&cli.StringSliceFlag{
				Name:  "include",
				Usage: "additional file patterns to sync (appended to sync.files in .treepad.json)",
			},
		},
		Action: run,
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	useCurrentFlag := cmd.Bool("use-current")
	sourcePath := cmd.Args().First()

	if useCurrentFlag && sourcePath != "" {
		return fmt.Errorf("--use-current and a source path argument are mutually exclusive")
	}

	worktrees, err := git.List(ctx, git.ExecRunner{})
	if err != nil {
		return err
	}
	if len(worktrees) == 0 {
		return fmt.Errorf("no git worktrees found")
	}

	var sourceDir string
	switch {
	case useCurrentFlag:
		sourceDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
		fmt.Printf("using current worktree as config source: %s\n", sourceDir)
	case sourcePath != "":
		abs, err := filepath.Abs(sourcePath)
		if err != nil {
			return fmt.Errorf("resolve source path: %w", err)
		}
		sourceDir = abs
		fmt.Printf("using specified path as config source: %s\n", sourceDir)
	default:
		main, err := git.MainWorktree(worktrees)
		if err != nil {
			return err
		}
		sourceDir = main.Path
		fmt.Printf("using main worktree as config source: %s\n", sourceDir)
	}

	repoSlug := slug.Slug(filepath.Base(sourceDir))

	outputDir := cmd.String("output-dir")
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		outputDir = filepath.Join(home, repoSlug+"-workspaces")
	}

	ad, err := editor.New(cmd.String("editor"))
	if err != nil {
		return err
	}

	if err := ad.Configure(worktrees, editor.Options{
		SourceDir: sourceDir,
		OutputDir: outputDir,
		Slug:      repoSlug,
		SyncOnly:  cmd.Bool("sync-only"),
	}); err != nil {
		return err
	}

	treePadCfg, err := config.Load(sourceDir)
	if err != nil {
		return err
	}
	patterns := append(treePadCfg.Sync.Files, cmd.StringSlice("include")...)

	fmt.Println("\nsyncing tool configs to worktrees...")
	syncer := sync.FileSyncer{}
	for _, wt := range worktrees {
		if wt.Path == sourceDir {
			continue
		}
		fmt.Printf("  → %s (%s)\n", wt.Branch, wt.Path)
		if err := syncer.Sync(patterns, sync.Config{
			SourceDir: sourceDir,
			TargetDir: wt.Path,
		}); err != nil {
			return fmt.Errorf("sync tool configs to %s: %w", wt.Branch, err)
		}
	}

	if cmd.Bool("sync-only") {
		fmt.Println("\ndone: config sync complete")
	} else {
		fmt.Println("\ndone: workspace files generated and configs synced")
	}
	return nil
}
