package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
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
				Usage:   "force removal of a dirty worktree and delete the branch even if unmerged",
			},
		},
		ShellComplete: completeRemoveBranch,
		Action:        runRemove,
	}
}

func runRemove(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}
	return lifecycle.Remove(ctx, commandDeps(cmd), lifecycle.RemoveInput{Branch: branch, Force: cmd.Bool("force")})
}
