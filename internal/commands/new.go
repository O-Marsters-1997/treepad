package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func newCommand() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "create a new git worktree, sync configs, and generate an artifact file",
		ArgsUsage: "<branch>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "ref to branch the new worktree from",
				Value:   "main",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Usage:   "open the generated artifact file after creation",
			},
			&cli.BoolFlag{
				Name:    "current",
				Aliases: []string{"c"},
				Usage:   "stay in the current directory instead of cd-ing into the new worktree",
			},
		},
		Action: runNew,
	}
}

func runNew(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}
	return treepad.New(ctx, commandDeps(cmd), treepad.NewInput{
		Branch:  branch,
		Base:    cmd.String("base"),
		Open:    cmd.Bool("open"),
		Current: cmd.Bool("current"),
	})
}
