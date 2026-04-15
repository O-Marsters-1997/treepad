package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := &cli.Command{
		Name:    "treepad",
		Usage:   "CLI for managing git worktrees",
		Version: version + " (commit: " + commit + ", built: " + date + ")",
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
		log.Fatal(err)
	}
}
