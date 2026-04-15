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

func removeCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a git worktree and its associated files",
		ArgsUsage: "<branch>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "discard uncommitted changes and force-delete unmerged branch",
			},
		},
		Action: runRemove,
	}
}

func runRemove(ctx context.Context, cmd *cli.Command) error {
	branch := cmd.Args().First()
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}

	runner := worktree.ExecRunner{}
	svc := workspace.NewService(
		runner,
		internalsync.FileSyncer{},
		workspace.ExecOpener{Runner: runner},
		os.Stdout,
	)
	return svc.Remove(ctx, workspace.RemoveInput{
		Branch: branch,
		Force:  cmd.Bool("force"),
	})
}
