package commands

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "report cross-worktree health issues (stale, merged-present, remote-gone, config-drift)",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "emit JSON instead of a table"},
			&cli.IntFlag{Name: "stale-days", Value: 30, Usage: "flag worktrees with no commit in this many days"},
			&cli.StringFlag{Name: "base", Value: "main", Usage: "branch to check merges against"},
			&cli.BoolFlag{Name: "offline", Usage: "skip remote branch existence check"},
			&cli.BoolFlag{Name: "strict", Usage: "exit non-zero if any findings are reported"},
		},
		Action: runDoctor,
	}
}

func runDoctor(ctx context.Context, cmd *cli.Command) error {
	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	return treepad.Doctor(ctx, d, treepad.DoctorInput{
		JSON:      cmd.Bool("json"),
		StaleDays: int(cmd.Int("stale-days")),
		Base:      cmd.String("base"),
		Offline:   cmd.Bool("offline"),
		Strict:    cmd.Bool("strict"),
	})
}
