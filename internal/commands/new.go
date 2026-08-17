package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad/fromspec"
	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
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
			&cli.StringFlag{
				Name:    "ticket",
				Aliases: []string{"t"},
				Usage:   "ticket URL, or a bare ref when [from_spec] ticket_url is configured; handed to agent_command",
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
	if ticket := cmd.String("ticket"); ticket != "" {
		if cmd.Bool("open") {
			return fmt.Errorf("--open is not supported with --ticket")
		}
		return runNewTicket(ctx, cmd, branch, ticket)
	}
	_, err = lifecycle.New(ctx, commandDeps(cmd), lifecycle.NewInput{
		Branch:    branch,
		Base:      cmd.String("base"),
		Open:      cmd.Bool("open"),
		Current:   cmd.Bool("current"),
		OutputDir: cmd.String("output-dir"),
	})
	if err != nil {
		return err
	}
	return nil
}

func runNewTicket(ctx context.Context, cmd *cli.Command, branch, ticket string) error {
	code, err := fromspec.FromSpec(ctx, commandDeps(cmd), fromspec.FromSpecInput{
		Ticket:    ticket,
		Branch:    branch,
		Base:      cmd.String("base"),
		Current:   cmd.Bool("current"),
		OutputDir: cmd.String("output-dir"),
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.Exit("", code)
	}
	return nil
}
