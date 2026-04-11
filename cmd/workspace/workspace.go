package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"treepad/internal/editor"
	"treepad/internal/git"
	"treepad/internal/slug"
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

	if cmd.Bool("sync-only") {
		fmt.Println("\ndone: config sync complete")
	} else {
		fmt.Println("\ndone: workspace files generated and configs synced")
	}
	return nil
}
