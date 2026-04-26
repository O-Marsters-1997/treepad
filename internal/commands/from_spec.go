package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func fromSpecCommand() *cli.Command {
	return &cli.Command{
		Name:      "from-spec",
		Usage:     "create a worktree from a GitHub issue, render a prompt, and hand off to an agent",
		ArgsUsage: "<branch>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "issue",
				Aliases: []string{"i"},
				Usage:   "GitHub issue `number` to use as the spec",
			},
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "ref to branch the new worktree from",
				Value:   "main",
			},
			&cli.BoolFlag{
				Name:    "current",
				Aliases: []string{"c"},
				Usage:   "stay in the current directory instead of cd-ing into the new worktree",
			},
			&cli.StringFlag{
				Name:    "prompt",
				Aliases: []string{"p"},
				Usage:   "instructions appended to the prompt body (default body ends with \"Implement the ticket.\")",
			},
		},
		Action: runFromSpec,
	}
}

func runFromSpec(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}

	issue := int(cmd.Int("issue"))
	if issue == 0 {
		return fmt.Errorf("--issue is required")
	}

	d := commandDeps(cmd)
	code, err := treepad.FromSpec(ctx, d, treepad.FromSpecInput{
		Issue:   issue,
		Branch:  branch,
		Base:    cmd.String("base"),
		Current: cmd.Bool("current"),
		Prompt:  cmd.String("prompt"),
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.Exit("", code)
	}
	return nil
}
