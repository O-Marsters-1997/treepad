package commands

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
	"treepad/internal/treepad/deps"
)

var uiScriptHandler func(context.Context, deps.Deps, treepad.StatusInput, string) error

// RegisterScriptedUI wires a hidden --script flag and its handler into tp ui.
// Intended to be called from e2e/register before the CLI is constructed.
func RegisterScriptedUI(handler func(context.Context, deps.Deps, treepad.StatusInput, string) error) {
	uiScriptHandler = handler
}

func uiCommand() *cli.Command {
	var flags []cli.Flag
	if uiScriptHandler != nil {
		flags = []cli.Flag{&cli.StringFlag{
			Name:   "script",
			Hidden: true,
			Usage:  "headless key replay for e2e tests (comma-separated tokens)",
		}}
	}
	return &cli.Command{
		Name:   "ui",
		Usage:  "open a live fleet view (requires a TTY)",
		Flags:  flags,
		Action: runUI,
	}
}

func runUI(ctx context.Context, cmd *cli.Command) error {
	d := commandDeps(cmd)
	if uiScriptHandler != nil {
		if keys := cmd.String("script"); keys != "" {
			return uiScriptHandler(ctx, d, treepad.StatusInput{}, keys)
		}
	}
	if err := treepad.UI(ctx, d, treepad.StatusInput{}); err != nil {
		if errors.Is(err, treepad.ErrNotTTY) {
			return cli.Exit(err.Error(), 2)
		}
		return err
	}
	return nil
}
