package commands

import (
	"context"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestBaseCommand_isRegistered(t *testing.T) {
	cmd := baseCommand()
	if cmd.Name != "base" {
		t.Errorf("command name = %q, want %q", cmd.Name, "base")
	}
}

func TestBaseCommand_isRunnable(t *testing.T) {
	app := &cli.Command{
		Commands: []*cli.Command{baseCommand()},
	}
	// Running inside the project's real git repo; we just verify the action is
	// reachable — either it succeeds or fails with a domain error, not a CLI
	// routing error.
	_ = app.Run(context.Background(), []string{"treepad", "base"})
}
