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
