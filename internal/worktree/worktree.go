package worktree

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Worktree struct {
	Path   string
	Branch string // stripped of refs/heads/ prefix; "(detached)" when detached
	IsMain bool   // true when .git entry is a directory, not a file
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func List(ctx context.Context, runner CommandRunner) ([]Worktree, error) {
	slog.Debug("listing git worktrees")
	out, err := runner.Run(ctx, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parsePorcelain(out)
}

// MainWorktree returns the worktree whose .git entry is a directory (the main repo).
func MainWorktree(wts []Worktree) (Worktree, error) {
	for _, wt := range wts {
		if wt.IsMain {
			return wt, nil
		}
	}
	return Worktree{}, fmt.Errorf("could not find main worktree (no .git directory found)")
}

// parsePorcelain parses `git worktree list --porcelain` output.
// Entries are blank-line separated; each entry contains key-value pairs.
func parsePorcelain(data []byte) ([]Worktree, error) {
	var worktrees []Worktree
	var current Worktree
	inEntry := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if inEntry {
				worktrees = append(worktrees, current)
				current = Worktree{}
				inEntry = false
			}
			continue
		}

		key, value, _ := strings.Cut(line, " ")
		inEntry = true

		switch key {
		case "worktree":
			current.Path = value
			current.IsMain = isMainWorktree(value)
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Branch = "(detached)"
		}
	}

	// flush final entry if output doesn't end with a blank line
	if inEntry {
		worktrees = append(worktrees, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parsing worktree list: %w", err)
	}

	return worktrees, nil
}

func isMainWorktree(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
