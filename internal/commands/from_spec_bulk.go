package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"treepad/internal/treepad/fromspec"

	"github.com/urfave/cli/v3"
)

func fromSpecBulkCommand() *cli.Command {
	return &cli.Command{
		Name:  "from-spec-bulk",
		Usage: "create worktrees from multiple GitHub issues; writes PROMPT.md into each and prints a summary",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "issues",
				Aliases:  []string{"i"},
				Usage:    "comma-separated issue `numbers`, e.g. \"12,14,19\"",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "branch-prefix",
				Usage: "prefix prepended to the slugified issue title for each branch name",
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
	issues, err := parseIssues(cmd.String("issues"))
	if err != nil {
		return err
	}

	d := commandDeps(cmd)
	_, failed, err := fromspec.FromSpecBulk(ctx, d, fromspec.FromSpecBulkInput{
		Issues:       issues,
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

func parseIssues(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	issues := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid issue number %q: must be a positive integer", p)
		}
		issues = append(issues, n)
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("--issues requires at least one issue number")
	}
	return issues, nil
}
