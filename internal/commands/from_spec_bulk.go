package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"treepad/internal/treepad"
)

func fromSpecBulkCommand() *cli.Command {
	return &cli.Command{
		Name:  "from-spec-bulk",
		Usage: "create worktrees from multiple GitHub issues; writes PROMPT.md into each and prints a summary",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "issues",
				Usage:    "comma-separated issue `numbers`, e.g. \"12,14,19\"",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "branch-prefix",
				Usage: "prefix prepended to the slugified issue title for each branch name",
			},
			&cli.StringFlag{
				Name:  "base",
				Usage: "ref to branch every new worktree from",
				Value: "main",
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

	d := treepad.DefaultDeps(cmd.Root().Writer, cmd.Root().ErrWriter, os.Stdin)
	_, failed, err := treepad.FromSpecBulk(ctx, d, treepad.FromSpecBulkInput{
		Issues:       issues,
		BranchPrefix: cmd.String("branch-prefix"),
		Base:         cmd.String("base"),
	})
	if err != nil {
		return err
	}
	if failed > 0 {
		os.Exit(1)
	}
	return nil
}

// parseIssues parses a comma-separated string of issue numbers into []int.
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
