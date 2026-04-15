package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/artifact"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad"
	"treepad/internal/worktree"
)

func switchCommand() *cli.Command {
	return &cli.Command{
		Name:      "switch",
		Usage:     "cd into an existing worktree by branch name",
		ArgsUsage: "<branch>",
		Action:    runSwitch,
	}
}

func runSwitch(ctx context.Context, cmd *cli.Command) error {
	branch := cmd.Args().First()
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}

	runner := worktree.ExecRunner{}
	svc := treepad.NewService(
		runner,
		internalsync.FileSyncer{},
		artifact.ExecOpener{Runner: runner},
		os.Stdout,
	)
	return svc.Switch(ctx, treepad.SwitchInput{Branch: branch})
}
