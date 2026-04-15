package worktree

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}

func List(ctx context.Context, runner CommandRunner) ([]Worktree, error) {
	slog.Debug("listing git worktrees")
	out, err := runner.Run(ctx, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	wts, err := parsePorcelain(out)
	if err != nil {
		return nil, err
	}
	for i := range wts {
		wts[i].IsMain = isMainWorktree(wts[i].Path)
	}
	return wts, nil
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

// MergedBranches returns local branches already merged into base, excluding base itself.
func MergedBranches(ctx context.Context, runner CommandRunner, base string) ([]string, error) {
	out, err := runner.Run(ctx, "git", "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("git branch --merged: %w", err)
	}
	var branches []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == base {
			continue
		}
		branches = append(branches, line)
	}
	return branches, nil
}

func isMainWorktree(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
