package commands

import (
	"context"
	"errors"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func uiCommand() *cli.Command {
	return &cli.Command{
		Name:   "ui",
		Usage:  "open a live fleet view (requires a TTY)",
		Action: runUI,
	}
}

func runUI(ctx context.Context, cmd *cli.Command) error {
	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	if err := treepad.UI(ctx, d, treepad.StatusInput{}); err != nil {
		if errors.Is(err, treepad.ErrNotTTY) {
			return cli.Exit(err.Error(), 2)
		}
		return err
	}
	return nil
}
