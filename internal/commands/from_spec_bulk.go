package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/O-Marsters-1997/treepad/internal/treepad/fromspec"
)

func fromSpecBulkCommand() *cli.Command {
	return &cli.Command{
		Name:  "from-spec-bulk",
		Usage: "create worktrees from multiple tickets; writes PROMPT.md into each and prints a summary",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "tickets",
				Aliases:  []string{"t"},
				Usage:    "comma-separated ticket URLs or refs, e.g. \"ENG-12,ENG-14\"",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "branch-prefix",
				Usage: "prefix prepended to the slugified ref for each branch name",
			},
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "ref to branch every new worktree from",
				Value:   "main",
			},
			&cli.StringFlag{
				Name:    "prompt",
				Aliases: []string{"p"},
				Usage:   "instructions appended to each prompt body (default body ends with \"Implement the ticket.\")",
			},
		},
		Action: runFromSpecBulk,
	}
}

func runFromSpecBulk(ctx context.Context, cmd *cli.Command) error {
	tickets, err := parseTickets(cmd.String("tickets"))
	if err != nil {
		return err
	}

	d := commandDeps(cmd)
	_, failed, err := fromspec.FromSpecBulk(ctx, d, fromspec.FromSpecBulkInput{
		Tickets:      tickets,
		BranchPrefix: cmd.String("branch-prefix"),
		Base:         cmd.String("base"),
		Prompt:       cmd.String("prompt"),
	})
	if err != nil {
		return err
	}
	if failed > 0 {
		return cli.Exit("", 1)
	}
	return nil
}

func parseTickets(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	tickets := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		tickets = append(tickets, p)
	}
	if len(tickets) == 0 {
		return nil, fmt.Errorf("--tickets requires at least one ticket")
	}
	return tickets, nil
}
