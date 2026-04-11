package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"treepad/internal/workspace"
)

func main() {
	cmd := &cli.Command{
		Name:  "treepad",
		Usage: "CLI for managing git worktrees",
		Commands: []*cli.Command{
			workspace.Command(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
