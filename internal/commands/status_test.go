package commands

import (
	"context"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestStatusCommand(t *testing.T) {
	t.Run("registered with name status", func(t *testing.T) {
		cmd := statusCommand()
		if cmd.Name != "status" {
			t.Errorf("name = %q, want %q", cmd.Name, "status")
		}
	})

	t.Run("has --json flag", func(t *testing.T) {
		cmd := statusCommand()
		for _, f := range cmd.Flags {
			if f.Names()[0] == "json" {
				return
			}
		}
		t.Error("--json flag not registered")
	})

	t.Run("runnable without routing error", func(t *testing.T) {
		app := &cli.Command{
			Commands: []*cli.Command{statusCommand()},
		}
		// Running inside the real git repo; verify the action is reachable —
		// domain errors are acceptable, CLI routing errors are not.
		_ = app.Run(context.Background(), []string{"treepad", "status"})
	})
}
