package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"treepad/internal/worktree"
)

// completeWorktreeBranch prints all non-detached worktree branches as completion candidates.
func completeWorktreeBranch(ctx context.Context, cmd *cli.Command) {
	printBranches(ctx, cmd, false)
}

// completeRemoveBranch prints non-main, non-detached branches — main cannot be removed.
func completeRemoveBranch(ctx context.Context, cmd *cli.Command) {
	printBranches(ctx, cmd, true)
}

// completeExecBranch completes the first positional arg (branch) of tp exec only.
func completeExecBranch(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() != 0 {
		return
	}
	completeWorktreeBranch(ctx, cmd)
}

func printBranches(ctx context.Context, cmd *cli.Command, skipMain bool) {
	wts, err := worktree.List(ctx, worktree.ExecRunner{})
	if err != nil {
		return
	}
	for _, wt := range wts {
		if wt.Branch == "(detached)" {
			continue
		}
		if skipMain && wt.IsMain {
			continue
		}
		fmt.Fprintln(cmd.Root().Writer, wt.Branch)
	}
}
