package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/artifact"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad"
	"treepad/internal/worktree"
)

func statusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "list all worktrees with branch, dirty state, ahead/behind, and last-touched",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "emit JSON instead of a table"},
		},
		Action: runStatus,
	}
}

func runStatus(ctx context.Context, cmd *cli.Command) error {
	runner := worktree.ExecRunner{}
	svc := treepad.NewService(
		runner,
		internalsync.FileSyncer{},
		artifact.ExecOpener{Runner: runner},
		os.Stdout,
	)
	return svc.Status(ctx, treepad.StatusInput{JSON: cmd.Bool("json")})
}
