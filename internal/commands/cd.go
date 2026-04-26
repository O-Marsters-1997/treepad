package commands

import (
	"context"
	"treepad/internal/treepad/cd"

	"github.com/urfave/cli/v3"
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
	return cd.CD(ctx, commandDeps(cmd), cd.CDInput{Branch: branch})
}
