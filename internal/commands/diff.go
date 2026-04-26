package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func diffCommand() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "diff a worktree against a base branch",
		ArgsUsage: "<branch> [-- <git-diff-args>...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "base branch to diff against (default: origin/main, or [diff] base in .treepad.toml)",
			},
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "write diff to `file` instead of terminal"},
		},
		Action: runDiff,
	}
}

func runDiff(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}
	extra := cmd.Args().Slice()[1:]
	return treepad.Diff(ctx, commandDeps(cmd), treepad.DiffInput{
		Branch:     branch,
		Base:       cmd.String("base"),
		OutputFile: cmd.String("output"),
		ExtraArgs:  extra,
	})
}
