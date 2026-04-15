package commands

import (
	"context"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestCDCommand_missingArg(t *testing.T) {
	app := &cli.Command{
		Commands: []*cli.Command{cdCommand()},
	}

	err := app.Run(context.Background(), []string{"treepad", "cd"})
	if err == nil {
		t.Fatal("expected error for missing branch arg, got nil")
	}
}
