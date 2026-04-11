package workspace

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"treepad/internal/editor"
	"treepad/internal/git"
	internalsync "treepad/internal/sync"
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

	ad, err := editor.New(cmd.String("editor"))
	if err != nil {
		return err
	}

	o := NewOrchestrator(git.ExecRunner{}, ad, internalsync.FileSyncer{})
	return o.Run(ctx, RunInput{
		UseCurrentDir: useCurrentFlag,
		SourcePath:    sourcePath,
		SyncOnly:      cmd.Bool("sync-only"),
		OutputDir:     cmd.String("output-dir"),
		ExtraPatterns: cmd.StringSlice("include"),
	})
}
