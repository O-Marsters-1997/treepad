package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "manage treepad configuration",
		Commands: []*cli.Command{
			configInitCommand(),
			configShowCommand(),
		},
	}
}

func configInitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "write a config file with default values",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "global",
				Aliases: []string{"g"},
				Usage:   "write to the global config path instead of .treepad.toml in the main worktree",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return treepad.ConfigInit(ctx, commandDeps(cmd), treepad.ConfigInitInput{
				Global: cmd.Bool("global"),
			})
		},
	}
}

func configShowCommand() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "print the resolved config and which sources contributed",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return treepad.ConfigShow(ctx, commandDeps(cmd), treepad.ConfigShowInput{})
		},
	}
}
