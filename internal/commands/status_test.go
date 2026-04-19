package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestStatusCommand(t *testing.T) {
	t.Run("--watch flag is removed and produces unknown flag error", func(t *testing.T) {
		app := &cli.Command{
			Commands: []*cli.Command{statusCommand()},
		}
		err := app.Run(context.Background(), []string{"tp", "status", "--watch"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Errorf("got error %q, want unknown flag error", err.Error())
		}
	})
}
