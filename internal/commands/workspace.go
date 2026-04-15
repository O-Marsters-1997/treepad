package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func workspaceCommand() *cli.Command {
	return &cli.Command{
		Name:      "workspace",
		Usage:     "sync configs and generate artifact files across git worktrees",
		ArgsUsage: "[source-path]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "use-current",
				Aliases: []string{"c"},
				Usage:   "use current directory as config source instead of the main worktree",
			},
			&cli.BoolFlag{
				Name:  "sync-only",
				Usage: "sync configs to all worktrees; skip artifact file generation",
			},
			&cli.StringFlag{
				Name:    "output-dir",
				Aliases: []string{"o"},
				Usage:   "directory for generated artifact files (default: ~/<repo-slug>-workspaces/)",
			},
			&cli.StringSliceFlag{
				Name:  "include",
				Usage: "additional file patterns to sync (appended to sync.files in .treepad.toml)",
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

	d := treepad.DefaultDeps(cmd.Root().Writer, os.Stdin)
	return treepad.Generate(ctx, d, treepad.GenerateInput{
		UseCurrentDir: useCurrentFlag,
		SourcePath:    sourcePath,
		SyncOnly:      cmd.Bool("sync-only"),
		OutputDir:     cmd.String("output-dir"),
		ExtraPatterns: cmd.StringSlice("include"),
	})
}
