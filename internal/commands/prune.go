package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/artifact"
	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad"
	"treepad/internal/worktree"
)

func pruneCommand() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "remove worktrees whose branches are merged into a base branch",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base", Value: "main", Usage: "base branch to check merges against"},
			&cli.BoolFlag{Name: "dry-run", Usage: "preview removals without executing"},
			&cli.BoolFlag{Name: "all", Usage: "force-remove all non-main worktrees (must be run from main)"},
		},
		Action: runPrune,
	}
}

func runPrune(ctx context.Context, cmd *cli.Command) error {
	runner := worktree.ExecRunner{}
	svc := treepad.NewService(
		runner,
		internalsync.FileSyncer{},
		artifact.ExecOpener{Runner: runner},
		hook.ExecRunner{Runner: runner},
		os.Stdout,
		os.Stdin,
	)
	return svc.Prune(ctx, treepad.PruneInput{
		Base:   cmd.String("base"),
		DryRun: cmd.Bool("dry-run"),
		All:    cmd.Bool("all"),
	})
}
