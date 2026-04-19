package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/commands"
	"treepad/internal/ui"
)

// Set at build time via linker flags:
//
//	-X main.Version=v1.2.3 -X main.Commit=abc1234 -X main.Date=2024-01-01
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	cmd := &cli.Command{
		Name:                  "tp",
		Usage:                 "CLI for managing git worktrees",
		Version:               Version + " (commit: " + Commit + ", built: " + Date + ")",
		EnableShellCompletion: true,
		Writer:                os.Stdout,
		ErrWriter:             os.Stderr,
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
				slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}),
			))
			return ctx, nil
		},
		Commands: commands.Router(),
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		ui.New(os.Stderr).Err(err.Error())
		os.Exit(1)
	}
}
