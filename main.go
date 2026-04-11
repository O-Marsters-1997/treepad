package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:     "treepad",
		Usage:    "CLI for managing git worktrees",
		Commands: []*cli.Command{
			// subcommands will be added here
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
