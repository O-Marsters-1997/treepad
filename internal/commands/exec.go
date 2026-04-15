package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/artifact"
	"treepad/internal/hook"
	internalsync "treepad/internal/sync"
	"treepad/internal/treepad"
	"treepad/internal/worktree"
)

func execCommand() *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Usage:     "run a command in a specific worktree",
		ArgsUsage: "<branch> [command] [args...]",
		Action:    runExec,
	}
}

func runExec(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("branch name is required")
	}

	branch := args[0]
	var command string
	var cmdArgs []string
	if len(args) > 1 {
		command = args[1]
		cmdArgs = args[2:]
	}

	runner := worktree.ExecRunner{}
	svc := treepad.NewService(
		runner,
		internalsync.FileSyncer{},
		artifact.ExecOpener{Runner: runner},
		hook.ExecRunner{Runner: runner},
		os.Stdout,
		os.Stdin,
	)
	exitCode, err := svc.Exec(ctx, treepad.ExecInput{
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
