package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"treepad/internal/worktree"
)

func completeWorktreeBranch(ctx context.Context, cmd *cli.Command) {
	writeBranches(ctx, worktree.ExecRunner{}, cmd.Root().Writer, func(wt worktree.Worktree) bool {
		return wt.Branch != "(detached)"
	})
}

// completeRemoveBranch omits the main worktree, which tp remove refuses to remove.
func completeRemoveBranch(ctx context.Context, cmd *cli.Command) {
	writeBranches(ctx, worktree.ExecRunner{}, cmd.Root().Writer, func(wt worktree.Worktree) bool {
		return !wt.IsMain && wt.Branch != "(detached)"
	})
}

func completeExecBranch(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() != 0 {
		return
	}
	completeWorktreeBranch(ctx, cmd)
}

func writeBranches(ctx context.Context, runner worktree.CommandRunner, w io.Writer, include func(worktree.Worktree) bool) {
	wts, err := worktree.List(ctx, runner)
	if err != nil {
		return
	}
	for _, wt := range wts {
		if !include(wt) {
			continue
		}
		if _, err := fmt.Fprintln(w, wt.Branch); err != nil {
			return
		}
	}
}
