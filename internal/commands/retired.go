package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// fromSpecBulkCommand is retired: a Batch of single-Ticket Chains replaces
// it (see docs/commands.md#batch-orchestration). It stays registered so the
// verb fails with an explanation instead of disappearing silently.
func fromSpecBulkCommand() *cli.Command {
	return &cli.Command{
		Name:  "from-spec-bulk",
		Usage: "retired — replaced by a Batch Manifest with one Chain per Ticket",
		Action: func(context.Context, *cli.Command) error {
			return fmt.Errorf("tp from-spec-bulk is retired: a Batch of single-Ticket Chains replaces it — " +
				"write a Manifest and run `tp batch sync` instead")
		},
	}
}
