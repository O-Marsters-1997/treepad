package app

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/commands"
	"treepad/internal/ui"
)

// Version, Commit, and Date are set at build time via linker flags:
//
//	-X treepad/internal/app.Version=v1.2.3
//	-X treepad/internal/app.Commit=abc1234
//	-X treepad/internal/app.Date=2024-01-01
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Run builds and executes the CLI, returning an exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	cmd := &cli.Command{
		Name:                  "tp",
		Usage:                 "CLI for managing git worktrees",
		Version:               Version + " (commit: " + Commit + ", built: " + Date + ")",
		EnableShellCompletion: true,
		Writer:                stdout,
		ErrWriter:             stderr,
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

// Main is a convenience wrapper so that cmd/tp/main.go remains a one-liner.
func Main() {
	os.Exit(Run(os.Args, os.Stdout, os.Stderr))
}
