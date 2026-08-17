package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad"
)

func playbookCommand() *cli.Command {
	return &cli.Command{
		Name:  "playbook",
		Usage: "manage the repo's playbooks",
		Commands: []*cli.Command{
			playbookNewCommand(),
		},
	}
}

func playbookNewCommand() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "write stdin verbatim to .claude/playbooks/<name>.md in the main worktree",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "overwrite a playbook that already exists",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("playbook name is required")
			}
			return treepad.PlaybookNew(ctx, commandDeps(cmd), treepad.PlaybookNewInput{
				Name:  name,
				Force: cmd.Bool("force"),
			})
		},
	}
}
