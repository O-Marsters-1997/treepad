package commands

import (
	"context"
	"treepad/internal/treepad/lifecycle"

	"github.com/urfave/cli/v3"
)

func removeCommand() *cli.Command {
	return &cli.Command{
		Name:          "remove",
		Usage:         "remove a git worktree and its associated files",
		ArgsUsage:     "<branch>",
		ShellComplete: completeRemoveBranch,
		Action:        runRemove,
	}
}

func runRemove(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}
	return lifecycle.Remove(ctx, commandDeps(cmd), lifecycle.RemoveInput{Branch: branch})
}
