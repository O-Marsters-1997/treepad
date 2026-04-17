package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func baseCommand() *cli.Command {
	return &cli.Command{
		Name:   "base",
		Usage:  "return to the default worktree",
		Action: runBase,
	}
}

func runBase(ctx context.Context, cmd *cli.Command) error {
	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	return treepad.Base(ctx, d, treepad.BaseInput{})
}
