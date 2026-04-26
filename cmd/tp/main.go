package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"treepad/internal/commands"
	"treepad/internal/profile"
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
			&cli.BoolFlag{
				Name:  "profile",
				Usage: "collect and display per-stage timing after the command finishes",
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
			if cmd.Bool("profile") {
				if cmd.Metadata == nil {
					cmd.Metadata = make(map[string]any)
				}
				cmd.Metadata["profiler"] = profile.NewRecorder()
			}
			return ctx, nil
		},
		After: func(_ context.Context, cmd *cli.Command) error {
			if rec, ok := cmd.Metadata["profiler"].(*profile.Recorder); ok {
				rec.Summary(cmd.Root().ErrWriter, cmd.Name+" "+cmd.Args().First())
			}
			return nil
		},
		Commands: commands.Router(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.Run(ctx, os.Args); err != nil {
		ui.New(os.Stderr).Err(err.Error())
		os.Exit(1)
	}
}
