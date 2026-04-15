package commands

import (
	"context"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestSwitchCommand_missingArg(t *testing.T) {
	app := &cli.Command{
		Commands: []*cli.Command{switchCommand()},
	}

	err := app.Run(context.Background(), []string{"treepad", "switch"})
	if err == nil {
		t.Fatal("expected error for missing branch arg, got nil")
	}
}
