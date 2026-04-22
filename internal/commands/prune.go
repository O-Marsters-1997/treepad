package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func pruneCommand() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "remove worktrees whose branches are merged into a base branch",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base", Aliases: []string{"b"}, Value: "main", Usage: "base branch to check merges against"},
			&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "preview removals without executing"},
			&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "force-remove all non-main worktrees (must be run from main)"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "skip the confirmation prompt"},
		},
		Action: runPrune,
	}
}

func runPrune(ctx context.Context, cmd *cli.Command) error {
	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	return treepad.Prune(ctx, d, treepad.PruneInput{
		Base:   cmd.String("base"),
		DryRun: cmd.Bool("dry-run"),
		All:    cmd.Bool("all"),
		Yes:    cmd.Bool("yes"),
	})
}
