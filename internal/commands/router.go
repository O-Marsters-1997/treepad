package commands

import "github.com/urfave/cli/v3"

func Router() []*cli.Command {
	return []*cli.Command{
		workspaceCommand(),
		configCommand(),
		newCommand(),
		shellInitCommand(),
		removeCommand(),
		pruneCommand(),
		statusCommand(),
		switchCommand(),
	}
}
