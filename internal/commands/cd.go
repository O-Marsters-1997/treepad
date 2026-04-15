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

	runner := worktree.ExecRunner{}
	svc := treepad.NewService(
		runner,
		internalsync.FileSyncer{},
		artifact.ExecOpener{Runner: runner},
		os.Stdout,
		os.Stdin,
	)
	return svc.CD(ctx, treepad.CDInput{Branch: branch})
}
