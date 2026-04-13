package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"treepad/internal/config"
	"treepad/internal/worktree"
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
				Name:  "global",
				Usage: "write to the global config path instead of .treepad.json in the main worktree",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("global") {
				path, err := config.WriteDefault("", true)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.Root().Writer, "Wrote config to %s\n", path)
				return err
			}

			wts, err := worktree.List(ctx, worktree.ExecRunner{})
			if err != nil {
				return fmt.Errorf("list worktrees: %w", err)
			}
			main, err := worktree.MainWorktree(wts)
			if err != nil {
				return err
			}
			path, err := config.WriteDefault(main.Path, false)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.Root().Writer, "Wrote config to %s\n", path)
			return err
		},
	}
}

func configShowCommand() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "print the resolved config and which sources contributed",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			wts, err := worktree.List(ctx, worktree.ExecRunner{})
			if err != nil {
				return fmt.Errorf("list worktrees: %w", err)
			}
			main, err := worktree.MainWorktree(wts)
			if err != nil {
				return err
			}
			output, err := config.Show(main.Path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.Root().Writer, output)
			return err
		},
	}
}
