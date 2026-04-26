package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
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
	return treepad.Remove(ctx, commandDeps(cmd), treepad.RemoveInput{Branch: branch})
}
