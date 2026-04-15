package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
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
	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	return treepad.Status(ctx, d, treepad.StatusInput{JSON: cmd.Bool("json")})
}
