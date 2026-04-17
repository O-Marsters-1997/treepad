package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func diffCommand() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "diff a worktree against a base branch",
		ArgsUsage: "<branch> [-- <git-diff-args>...]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base", Aliases: []string{"b"}, Value: "main", Usage: "base branch to diff against"},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "write diff to `file` instead of terminal"},
		},
		Action: runDiff,
	}
}

func runDiff(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("branch name is required")
	}
	branch := args[0]
	extra := args[1:]

	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	return treepad.Diff(ctx, d, treepad.DiffInput{
		Branch:     branch,
		Base:       cmd.String("base"),
		OutputFile: cmd.String("output"),
		ExtraArgs:  extra,
	})
}
