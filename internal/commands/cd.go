package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func cdCommand() *cli.Command {
	return &cli.Command{
		Name:      "cd",
		Usage:     "cd into an existing worktree by branch name",
		ArgsUsage: "<branch>",
		Action:    runCD,
	}
}

func runCD(ctx context.Context, cmd *cli.Command) error {
	branch := cmd.Args().First()
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}

	d := treepad.DefaultDeps(cmd.Root().Writer, os.Stdin)
	return treepad.CD(ctx, d, treepad.CDInput{Branch: branch})
}
