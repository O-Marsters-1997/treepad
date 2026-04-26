package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func execCommand() *cli.Command {
	return &cli.Command{
		Name:          "exec",
		Usage:         "run a command in a specific worktree",
		ArgsUsage:     "<branch> [command] [args...]",
		ShellComplete: completeExecBranch,
		Action:        runExec,
	}
}

func runExec(ctx context.Context, cmd *cli.Command) error {
	branch, err := requireBranch(cmd)
	if err != nil {
		return err
	}

	args := cmd.Args().Slice()
	var command string
	var cmdArgs []string
	if len(args) > 1 {
		command = args[1]
		cmdArgs = args[2:]
	}

	d := commandDeps(cmd)
	exitCode, err := treepad.Exec(ctx, d, treepad.ExecInput{
		Branch:  branch,
		Command: command,
		Args:    cmdArgs,
	})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return cli.Exit("", exitCode)
	}
	return nil
}
