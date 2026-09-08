package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
)

func removeCommand() *cli.Command {
	forceFlag := &cli.BoolFlag{
		Name:    "force",
		Aliases: []string{"f"},
		Usage:   "force removal of a dirty worktree and delete the branch even if unmerged",
	}
	mergedFlag := &cli.BoolFlag{
		Name:  "merged",
		Usage: "assert the branch already merged upstream, deleting it even if not an ancestor of the base branch",
	}
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a git worktree and its associated files",
		ArgsUsage: "<branch>",
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{Flags: [][]cli.Flag{{forceFlag}, {mergedFlag}}},
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
	in := lifecycle.RemoveInput{Branch: branch, Force: cmd.Bool("force"), Merged: cmd.Bool("merged")}
	return lifecycle.Remove(ctx, commandDeps(cmd), in)
}
