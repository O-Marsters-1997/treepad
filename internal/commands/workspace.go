package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	internalsync "treepad/internal/sync"
	"treepad/internal/workspace"
	"treepad/internal/worktree"
)

func workspaceCommand() *cli.Command {
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
			&cli.StringSliceFlag{
				Name:  "include",
				Usage: "additional file patterns to sync (appended to sync.files in .treepad.json)",
			},
		},
		Action: runWorkspace,
	}
}

func runWorkspace(ctx context.Context, cmd *cli.Command) error {
	useCurrentFlag := cmd.Bool("use-current")
	sourcePath := cmd.Args().First()

	if useCurrentFlag && sourcePath != "" {
		return fmt.Errorf("--use-current and a source path argument are mutually exclusive")
	}

	o := workspace.NewOrchestrator(worktree.ExecRunner{}, internalsync.FileSyncer{}, os.Stdout)
	return o.Run(ctx, workspace.RunInput{
		UseCurrentDir: useCurrentFlag,
		SourcePath:    sourcePath,
		SyncOnly:      cmd.Bool("sync-only"),
		OutputDir:     cmd.String("output-dir"),
		ExtraPatterns: cmd.StringSlice("include"),
	})
}
