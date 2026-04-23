package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func fromSpecCommand() *cli.Command {
	return &cli.Command{
		Name:      "from-spec",
		Usage:     "create a worktree from a spec (GitHub issue or file), render a prompt, and hand off to an agent",
		ArgsUsage: "<branch>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "issue",
				Aliases: []string{"i"},
				Usage:   "GitHub issue `number` to use as the spec (mutually exclusive with --file)",
			},
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "`path` to a local markdown spec file (mutually exclusive with --issue)",
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
	branch := cmd.Args().First()
	if branch == "" {
		return fmt.Errorf("branch name is required")
	}

	issue := int(cmd.Int("issue"))
	file := cmd.String("file")
	if issue == 0 && file == "" {
		return fmt.Errorf("one of --issue or --file is required")
	}
	if issue != 0 && file != "" {
		return fmt.Errorf("--issue and --file are mutually exclusive")
	}

	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	code, err := treepad.FromSpec(ctx, d, treepad.FromSpecInput{
		Issue:   issue,
		File:    file,
		Branch:  branch,
		Base:    cmd.String("base"),
		Current: cmd.Bool("current"),
		Prompt:  cmd.String("prompt"),
	})
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}
