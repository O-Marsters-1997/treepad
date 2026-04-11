package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"treepad/internal/git"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
	"treepad/internal/vscode"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:      "workspace",
		Usage:     "sync VS Code configs and generate .code-workspace files across git worktrees",
		ArgsUsage: "[source-path]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "use-current",
				Aliases: []string{"c"},
				Usage:   "use current directory as config source instead of the main worktree",
			},
			&cli.BoolFlag{
				Name:  "sync-only",
				Usage: "sync configs to all worktrees; skip .code-workspace file generation",
			},
			&cli.StringFlag{
				Name:    "output-dir",
				Aliases: []string{"o"},
				Usage:   "directory for generated .code-workspace files (default: ~/<repo-slug>-workspaces/)",
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
	syncOnly := cmd.Bool("sync-only")

	if !syncOnly {
		outputDir := cmd.String("output-dir")
		if outputDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			outputDir = filepath.Join(home, repoSlug+"-workspaces")
		}

		extensions, err := resolveExtensions(sourceDir)
		if err != nil {
			return err
		}

		fmt.Printf("\ngenerating workspace files → %s\n", outputDir)
		if err := vscode.Generate(worktrees, extensions, repoSlug, outputDir); err != nil {
			return err
		}
	}

	fmt.Println("\nsyncing configs to worktrees...")
	if err := internalsync.SyncAll(internalsync.FileSyncer{}, sourceDir, worktrees); err != nil {
		return err
	}

	if syncOnly {
		fmt.Println("\ndone: config sync complete")
	} else {
		fmt.Println("\ndone: workspace files generated and configs synced")
	}
	return nil
}

func resolveExtensions(dir string) ([]string, error) {
	exts, err := vscode.ReadExtensions(dir)
	if err != nil {
		return nil, err
	}
	if exts != nil {
		return exts, nil
	}
	return vscode.DetectExtensions(dir)
}
