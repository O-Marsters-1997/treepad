package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	internalsync "treepad/internal/sync"
	"treepad/internal/workspace"
	"treepad/internal/worktree"
)

func createCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "create a new git worktree, sync configs, and generate a workspace file",
		ArgsUsage: "<branch>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "base",
				Usage: "ref to branch the new worktree from",
				Value: "main",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Usage:   "open the generated workspace file after creation",
			},
		},
		Action: runCreate,
	}
}

func runCreate(ctx context.Context, cmd *cli.Command) error {
	branch := cmd.Args().First()
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}

	runner := worktree.ExecRunner{}
	svc := workspace.NewService(
		runner,
		internalsync.FileSyncer{},
		workspace.ExecOpener{Runner: runner},
		os.Stdout,
	)
	return svc.Create(ctx, workspace.CreateInput{
		Branch: branch,
		Base:   cmd.String("base"),
		Open:   cmd.Bool("open"),
	})
}
