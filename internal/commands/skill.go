package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func skillCommand() *cli.Command {
	return &cli.Command{
		Name:  "skill",
		Usage: "manage treepad's agent skills",
		Commands: []*cli.Command{
			skillInstallCommand(),
			skillListCommand(),
		},
	}
}

func skillInstallCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "install treepad's agent skills into ~/.agents/skills, linking any detected agent harness that needs its own copy",
		ArgsUsage: "[name...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Usage:   "install into the repo instead of the user's home directory",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "overwrite a skill directory that already exists",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return treepad.SkillInstall(ctx, commandDeps(cmd), treepad.SkillInstallInput{
				Names: cmd.Args().Slice(),
				Local: cmd.Bool("local"),
				Force: cmd.Bool("force"),
			})
		},
	}
}

func skillListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list the agent skills treepad ships",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return treepad.SkillList(ctx, commandDeps(cmd), treepad.SkillListInput{})
		},
	}
}
