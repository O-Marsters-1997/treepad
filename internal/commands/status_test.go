package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestStatusCommand(t *testing.T) {
	t.Run("rejects watch and json together", func(t *testing.T) {
		app := &cli.Command{
			Commands: []*cli.Command{statusCommand()},
		}
		err := app.Run(context.Background(), []string{"tp", "status", "--watch", "--json"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("got error %q, want containing \"mutually exclusive\"", err.Error())
		}
	})
}
