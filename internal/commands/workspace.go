package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad"
	"treepad/internal/worktree"
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

	runner := worktree.ExecRunner{}
	svc := treepad.NewService(runner, internalsync.FileSyncer{}, nil, hook.ExecRunner{Runner: runner}, os.Stdout, os.Stdin)
	return svc.Generate(ctx, treepad.GenerateInput{
		UseCurrentDir: useCurrentFlag,
		SourcePath:    sourcePath,
		SyncOnly:      cmd.Bool("sync-only"),
		OutputDir:     cmd.String("output-dir"),
		ExtraPatterns: cmd.StringSlice("include"),
	})
}
