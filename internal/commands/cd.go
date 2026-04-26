package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func cdCommand() *cli.Command {
	return &cli.Command{
		Name:          "cd",
		Usage:         "cd into an existing worktree by branch name",
		ArgsUsage:     "<branch>",
		ShellComplete: completeWorktreeBranch,
		Action:        runCD,
	}
}

func runCD(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}
	return treepad.CD(ctx, commandDeps(cmd), treepad.CDInput{Branch: branch})
}
