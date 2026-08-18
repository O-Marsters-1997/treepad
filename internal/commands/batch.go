package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad"
)

func batchCommand() *cli.Command {
	return &cli.Command{
		Name:  "batch",
		Usage: "manage Batch orchestration: Manifests, Chains, and the fleet they materialise",
		Commands: []*cli.Command{
			batchListCommand(),
			batchSyncCommand(),
		},
	}
}

func batchListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "list every Batch, its Chains, and each member's ticket, ref, branch and base",
		Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Aliases: []string{"j"}, Usage: "emit JSON instead of a table"}},
		Action: runBatchList,
	}
}

func runBatchList(ctx context.Context, cmd *cli.Command) error {
	d := commandDeps(cmd)
	return treepad.BatchList(ctx, d, treepad.BatchListInput{JSON: cmd.Bool("json")})
}

func batchSyncCommand() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "reconcile every Batch: materialise each Chain into a stacked worktree per member",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Aliases: []string{"j"}, Usage: "emit the Report as JSON"},
			&cli.BoolFlag{
				Name: "dry-run", Aliases: []string{"n"},
				Usage: "print what would be created without touching anything",
			},
			&cli.StringFlag{Name: "batch", Usage: "narrow to one Manifest by name"},
			&cli.BoolFlag{Name: "offline", Usage: "skip the gh call; report last-known PR state"},
		},
		Action: runBatchSync,
	}
}

func runBatchSync(ctx context.Context, cmd *cli.Command) error {
	failed, err := treepad.BatchSync(ctx, commandDeps(cmd), treepad.BatchSyncInput{
		JSON:    cmd.Bool("json"),
		DryRun:  cmd.Bool("dry-run"),
		Offline: cmd.Bool("offline"),
		Batch:   cmd.String("batch"),
	})
	if err != nil {
		return err
	}
	if failed > 0 {
		return cli.Exit("", 1)
	}
	return nil
}
