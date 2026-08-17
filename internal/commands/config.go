package commands

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad"
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
			&cli.BoolFlag{
				Name:    "inherit",
				Aliases: []string{"i"},
				Usage:   "seed the config from the global config instead of the built-in defaults",
			},
			&cli.BoolFlag{
				Name:    "hooks-only",
				Aliases: []string{"H"},
				Usage:   "with --inherit, keep the built-in defaults and lift only [hooks] from the global config",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "overwrite the config file if it already exists",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			hooksOnly := cmd.Bool("hooks-only")
			if hooksOnly && !cmd.Bool("inherit") {
				return errors.New("--hooks-only requires --inherit")
			}
			return treepad.ConfigInit(ctx, commandDeps(cmd), treepad.ConfigInitInput{
				Global:    cmd.Bool("global"),
				Inherit:   cmd.Bool("inherit"),
				HooksOnly: hooksOnly,
				Force:     cmd.Bool("force"),
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
