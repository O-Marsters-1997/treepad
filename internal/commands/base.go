package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func commandDeps(cmd *cli.Command) treepad.Deps {
	return treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
}

func requireBranch(cmd *cli.Command) (string, error) {
	branch := cmd.Args().First()
	if branch == "" {
		return "", fmt.Errorf("branch name is required")
	}
	return branch, nil
}

func baseCommand() *cli.Command {
	return &cli.Command{
		Name:   "base",
		Usage:  "return to the default worktree",
		Action: runBase,
	}
}

func runBase(ctx context.Context, cmd *cli.Command) error {
	return treepad.Base(ctx, commandDeps(cmd), treepad.BaseInput{})
}
