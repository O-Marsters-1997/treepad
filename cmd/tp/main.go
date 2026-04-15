package main

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/commands"
	"treepad/internal/ui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() { os.Exit(Run(os.Args, os.Stdout, os.Stderr)) }

// Run is the testscript entry point: it builds and executes the CLI, returning
// an exit code. main() is a thin wrapper so tests can invoke Run directly.
func Run(args []string, stdout, stderr io.Writer) int {
	cmd := &cli.Command{
		Name:      "tp",
		Usage:     "CLI for managing git worktrees",
		Version:   version + " (commit: " + commit + ", built: " + date + ")",
		Writer:    stdout,
		ErrWriter: stderr,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "enable debug logging to stderr",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := slog.LevelWarn
			if cmd.Bool("verbose") {
				level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(
				slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}),
			))
			return ctx, nil
		},
		Commands: commands.Router(),
	}

	if err := cmd.Run(context.Background(), args); err != nil {
		ui.New(stderr).Err(err.Error())
		return 1
	}
	return 0
}
