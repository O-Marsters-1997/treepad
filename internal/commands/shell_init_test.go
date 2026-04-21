package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestShellInitCommand(t *testing.T) {
	cmd := shellInitCommand()

	var buf bytes.Buffer
	app := &cli.Command{
		Writer:   &buf,
		Commands: []*cli.Command{cmd},
	}

	err := app.Run(context.Background(), []string{"treepad", "shell-init"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"tp()", "TREEPAD_CD_FD=3", "3>&1 1>&4"} {
		if !strings.Contains(out, want) {
			t.Errorf("shell-init output missing %q; got:\n%s", want, out)
		}
	}
	// Regression guard: stdout must not be captured by $(...) — that breaks
	// interactive subprocesses and buffers all output until tp exits.
	if strings.Contains(out, `out=$(command tp`) {
		t.Errorf("shell-init output still uses old stdout-capturing pattern; got:\n%s", out)
	}
}
