package commands

import "github.com/urfave/cli/v3"

func Router() []*cli.Command {
	return []*cli.Command{
		syncCommand(),
		configCommand(),
		newCommand(),
		shellInitCommand(),
		removeCommand(),
		pruneCommand(),
		statusCommand(),
		cdCommand(),
		baseCommand(),
		doctorCommand(),
		execCommand(),
		diffCommand(),
		fromSpecCommand(),
		fromSpecBulkCommand(),
	}
}
